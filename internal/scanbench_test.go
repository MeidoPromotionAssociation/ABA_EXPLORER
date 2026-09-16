package internal

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// 这个文件是 runScans 的 worker 数标定实验，不进常规测试
// 设置 ABA_BENCH_ROOT 指向真实目录后运行：
//
//	ABA_BENCH_ROOT="X:\path\to\GameData" go test ./internal -run TestScanWorkerScaling -v -timeout 0
//
// 可选环境变量：ABA_BENCH_WORKERS（默认一串阶梯）、ABA_BENCH_ROUNDS（默认 3）、ABA_BENCH_DEEP=0 只扫浅层、
// ABA_BENCH_COLD=0 跳过冷读阶段
// This file is the worker-count calibration experiment for runScans and stays out of the normal test run
// Point ABA_BENCH_ROOT at a real directory and run the command above
// Optional environment variables: ABA_BENCH_WORKERS (defaults to a ladder), ABA_BENCH_ROUNDS (default 3),
// ABA_BENCH_DEEP=0 to scan the shallow layer only, and ABA_BENCH_COLD=0 to skip the cold-read phase

// windows 的进程级计数器，用来把墙钟时间拆成 CPU 时间和磁盘读取量
// 只看墙钟无法区分瓶颈是 CPU 还是盘，而这两者给出的最优 worker 数正好相反
// Windows process-level counters that split wall time into CPU time and bytes read from the disk
// Wall time alone cannot tell whether the bottleneck is the CPU or the disk, and those two point at opposite
// worker counts
var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGetProcessIoCounters = kernel32.NewProc("GetProcessIoCounters")
	procGetProcessTimes      = kernel32.NewProc("GetProcessTimes")
)

// processIoCounters 对应 Win32 的 IO_COUNTERS / processIoCounters mirrors the Win32 IO_COUNTERS structure
type processIoCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

// processFiletime 对应 Win32 的 FILETIME，单位是 100 纳秒
// processFiletime mirrors the Win32 FILETIME structure in 100-nanosecond units
type processFiletime struct {
	Low  uint32
	High uint32
}

func (value processFiletime) duration() time.Duration {
	ticks := uint64(value.High)<<32 | uint64(value.Low)
	return time.Duration(ticks) * 100 * time.Nanosecond
}

// readIoCounters 读取当前进程的累计读写量 / readIoCounters reads the accumulated read and write volume of this process
func readIoCounters() processIoCounters {
	var counters processIoCounters
	handle, _ := syscall.GetCurrentProcess()
	procGetProcessIoCounters.Call(uintptr(handle), uintptr(unsafe.Pointer(&counters)))
	return counters
}

// readCpuTime 读取当前进程累计的用户加内核 CPU 时间
// readCpuTime reads the accumulated user plus kernel CPU time of this process
func readCpuTime() time.Duration {
	var creation, exit, kernel, user processFiletime
	handle, _ := syscall.GetCurrentProcess()
	procGetProcessTimes.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&creation)),
		uintptr(unsafe.Pointer(&exit)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	return kernel.duration() + user.duration()
}

// benchSample 是一轮测量的结果 / benchSample is the outcome of one measured round
type benchSample struct {
	wall     time.Duration
	cpu      time.Duration
	readMB   float64
	readOps  uint64
	names    int
	files    int
	allocMB  float64
	peakMB   float64
	gcCycles uint32
}

// TestScanWorkerScaling 在真实目录上按 worker 数阶梯各跑若干轮，输出墙钟、CPU 与磁盘读取量
// 分两个阶段。冷读阶段把文件按体积分层切成互不重叠的切片，每个 worker 数只扫自己那一份，
// 谁都没有被别的配置预热过，这样一次就能在所有 worker 数上拿到冷盘的读数；
// 热读阶段再在全量目标上按轮次轮流跑，比较稳定状态下的差距
// 热读阶段之所以轮流跑而不是一个配置跑完再换下一个：磁盘缓存会随着读取变热，
// 集中跑会让靠后的配置白白占便宜
// TestScanWorkerScaling runs the worker-count ladder over a real directory and reports wall time, CPU time,
// and bytes read from disk
// It has two phases. The cold phase splits the files into disjoint slices balanced by size and gives each worker
// count its own slice, so no configuration is warmed up by another and a single pass yields a cold-disk reading
// for every worker count. The warm phase then runs the full target set round by round
// The warm phase interleaves its rounds rather than running a configuration to completion because the file cache
// warms up as the run proceeds and that would hand the later configurations a free advantage
func TestScanWorkerScaling(t *testing.T) {
	root := strings.TrimSpace(os.Getenv("ABA_BENCH_ROOT"))
	if root == "" {
		t.Skip("set ABA_BENCH_ROOT to a directory to run the worker-count calibration")
	}
	deep := os.Getenv("ABA_BENCH_DEEP") != "0"
	rounds := envInt("ABA_BENCH_ROUNDS", 3)
	ladder := parseWorkerLadder(os.Getenv("ABA_BENCH_WORKERS"))
	if len(ladder) == 0 {
		t.Fatalf("ABA_BENCH_WORKERS lists no positive worker count")
	}

	targets, err := scanRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("scan %q: %v", root, err)
	}
	total := len(targets.catalogs) + len(targets.containers)
	t.Logf("root=%s deep=%v cores=%d gomaxprocs=%d", root, deep, runtime.NumCPU(), runtime.GOMAXPROCS(0))
	t.Logf("catalogs=%d containers=%d total=%d rounds=%d ladder=%v",
		len(targets.catalogs), len(targets.containers), total, rounds, ladder)

	service := NewSearchService()
	// ABA_BENCH_GOGC 用来把 GC 关小或关掉再看一遍曲线：热读阶段的墙钟在 6 个 worker 后就压不动了，
	// 而 CPU 总量还在涨，只有改变 GC 才能判断这堵墙是分配开销还是别的
	// ABA_BENCH_GOGC rescans the curve with GC turned down or off: the warm wall time stops moving after six
	// workers while total CPU keeps climbing, and only a change to GC can tell whether that wall is allocation cost
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("ABA_BENCH_GOGC"))); err == nil {
		previous := debug.SetGCPercent(value)
		defer debug.SetGCPercent(previous)
		fmt.Printf("GOGC 设为 %d（默认为 100）/ GOGC set to %d (default 100)\n", value, value)
	}
	if os.Getenv("ABA_BENCH_COLD") != "0" {
		runColdPhase(t, service, targets, deep, ladder)
	}
	runWarmPhase(t, service, targets, deep, total, ladder, rounds, fmt.Sprintf("热读阶段（全量目标，%d 轮，取最快的一轮）", rounds))
	// ABA_BENCH_LATENCY_US 给每次读取叠一个人为延迟，用来回答"盘慢下来以后该开多少 worker"
	// 本机的 NVMe 加系统缓存让整轮只读 178 MB，瓶颈根本不在盘上，于是拐点落在核数的一小半；
	// 把延迟加到 SMB 或 HDD 的量级，拐点会往右走，2 倍甚至 8 倍核数的建议就是为那种场景准备的
	// ABA_BENCH_LATENCY_US adds artificial latency to every read, answering how many workers a slower disk wants
	// This machine's NVMe and its file cache hold a whole sweep to 178 MB, so the disk is nowhere near the
	// bottleneck and the knee sits at a fraction of the core count. Raising the latency to SMB or HDD levels moves
	// the knee right, and that is the case the two-times or eight-times-cores advice was written for
	if usec := envInt("ABA_BENCH_LATENCY_US", 0); usec > 0 {
		restore := injectReadLatency(time.Duration(usec) * time.Microsecond)
		runWarmPhase(t, service, targets, deep, total, ladder, rounds,
			fmt.Sprintf("注入 %d µs/次读取延迟 / %d µs injected per read", usec, usec))
		restore()
	}
	singleFileReport(t, targets, deep)
	postScanReport(t, service, targets, deep, total)
}

// injectedOps 记录注入了延迟的读取次数，用来核对实际读了多少次
// injectedOps counts the reads that carried injected latency, which checks how many reads really happened
var injectedOps int64

// slowReader 在每次读上叠加固定延迟，模拟寻道慢的盘
// 只延迟 Read 与 ReadAt：Seek 不落盘，把它算进去会高估慢盘
// slowReader adds a fixed delay to every read to stand in for a disk with slow seeks
// Only Read and ReadAt are delayed: a Seek never reaches the platter, and counting it would overstate the disk
type slowReader struct {
	file  *os.File
	delay time.Duration
}

func (reader *slowReader) wait() {
	atomic.AddInt64(&injectedOps, 1)
	time.Sleep(reader.delay)
}

func (reader *slowReader) Read(p []byte) (int, error) {
	reader.wait()
	return reader.file.Read(p)
}

func (reader *slowReader) ReadAt(p []byte, off int64) (int, error) {
	reader.wait()
	return reader.file.ReadAt(p, off)
}

func (reader *slowReader) Seek(offset int64, whence int) (int64, error) {
	return reader.file.Seek(offset, whence)
}

// injectReadLatency 换掉解析器的读口，返回还原函数
// injectReadLatency swaps the reader the parsers use and returns the function that puts the original back
func injectReadLatency(delay time.Duration) func() {
	previous := scanReader
	start := time.Now()
	atomic.StoreInt64(&injectedOps, 0)
	scanReader = func(handle *os.File) io.ReadSeeker { return &slowReader{file: handle, delay: delay} }
	return func() {
		scanReader = previous
		ops := atomic.LoadInt64(&injectedOps)
		if ops > 0 {
			fmt.Printf("注入延迟校准：%d 次读取共耗时 %.2fs，平均 %.0f µs/次（目标 %d µs）/ latency calibration\n",
				ops, time.Since(start).Seconds(), float64(time.Since(start).Microseconds())/float64(ops), delay.Microseconds())
		}
	}
}

// postScanReport 把解析之后的单线程阶段单独计时
// 解析能并行而合并与展平不能，若这两段占了大头，那么 worker 数怎么调都是在优化次要项
// postScanReport times the single-threaded phases that follow the scans
// Parsing is parallel while merging and flattening are not, and if those two dominate then every worker count is
// tuning the smaller half of the job
func postScanReport(t *testing.T, service *SearchService, targets scanTargets, deep bool, total int) {
	if envInt("ABA_BENCH_FULL", 0) == 0 {
		return
	}
	catalogs, containers, cancelled := service.runScans(context.Background(), targets, deep, total)
	if cancelled {
		t.Fatal("the scan reported a cancellation")
	}

	start := time.Now()
	scanned, warnings := mergeScans(catalogs, containers)
	merged := time.Since(start)

	start = time.Now()
	sources := combineSources(nil, scanned)
	combined := time.Since(start)

	start = time.Now()
	index := buildIndex(strings.TrimSpace(os.Getenv("ABA_BENCH_ROOT")), deep, sources, warnings)
	flattened := time.Since(start)

	fmt.Printf("\n解析之后的单线程阶段 / single-threaded phases that follow the scans\n")
	fmt.Printf("| phase | wall | notes |\n| --- | ---: | --- |\n")
	fmt.Printf("| mergeScans | %.2fs | |\n", merged.Seconds())
	fmt.Printf("| combineSources | %.2fs | 只沿用了缓存来源时才有活干 / only works when cached sources are reused |\n", combined.Seconds())
	fmt.Printf("| buildIndex | %.2fs | |\n", flattened.Seconds())
	fmt.Printf("| 合计 / total | %.2fs | sources=%d records=%d |\n",
		(merged + combined + flattened).Seconds(), len(sources), index.stats.Names)
	fmt.Printf("\n")
}

// singleFileReport 逐个解析文件并列出最慢的几个
// 池子开得再多也追不上最长的那一个文件：若单个文件就要零点几秒，墙钟的下限就是它，
// 这时加 worker 只是让更多协程陪着它一起等
// singleFileReport parses the files one at a time and lists the slowest
// No pool outruns its longest single file: when one file alone takes several tenths of a second that time is the
// floor for the whole run, and extra workers only wait alongside it
func singleFileReport(t *testing.T, targets scanTargets, deep bool) {
	count := envInt("ABA_BENCH_SLOWEST", 0)
	if count == 0 {
		return
	}
	type fileTime struct {
		path    string
		bytes   int64
		entries int
		took    time.Duration
	}
	timings := make([]fileTime, 0, len(targets.containers))
	for _, path := range targets.containers {
		start := time.Now()
		result := scanContainerFile(path, deep)
		info, _ := os.Stat(path)
		var size int64
		if info != nil {
			size = info.Size()
		}
		timings = append(timings, fileTime{path: filepath.Base(path), bytes: size, entries: len(result.entries), took: time.Since(start)})
	}
	for _, path := range targets.catalogs {
		start := time.Now()
		result := scanCatalogFile(path)
		info, _ := os.Stat(path)
		var size int64
		if info != nil {
			size = info.Size()
		}
		timings = append(timings, fileTime{path: filepath.Base(path), bytes: size, entries: len(result.entries), took: time.Since(start)})
	}
	sort.SliceStable(timings, func(left, right int) bool { return timings[left].took > timings[right].took })

	var total time.Duration
	for _, timing := range timings {
		total += timing.took
	}
	fmt.Printf("\n单文件耗时（逐个解析，共 %d 个文件，合计 %.2fs）/ per-file cost (parsed one at a time)\n", len(timings), total.Seconds())
	fmt.Printf("| file | MB | entries | took |\n| --- | ---: | ---: | ---: |\n")
	for index, timing := range timings {
		if index >= count {
			break
		}
		fmt.Printf("| %s | %.1f | %d | %.3fs |\n", timing.path, float64(timing.bytes)/(1<<20), timing.entries, timing.took.Seconds())
	}
	fmt.Printf("\n")
	for _, share := range []float64{0.5, 0.9, 0.99} {
		var running time.Duration
		for index, timing := range timings {
			running += timing.took
			if running >= time.Duration(float64(total)*share) {
				fmt.Printf("前 %d/%d 个文件占 %.0f%% 的串行总耗时\n", index+1, len(timings), share*100)
				break
			}
		}
	}
}

// runColdPhase 让每个 worker 数扫一份谁都没碰过的切片，取冷盘读数
// runColdPhase gives every worker count a slice nobody has touched yet, yielding cold-disk readings
func runColdPhase(t *testing.T, service *SearchService, targets scanTargets, deep bool, ladder []int) {
	containers := splitBySize(targets.containers, len(ladder))
	catalogs := splitBySize(targets.catalogs, len(ladder))

	fmt.Printf("\n冷读阶段（每个 worker 数一份互不重叠的切片）/ cold phase (a disjoint slice per worker count)\n")
	fmt.Printf("| workers | files | bytes | wall | cpu | cpu/wall | read MB | read ops | avg read | names |\n")
	fmt.Printf("| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for index, workers := range ladder {
		part := scanTargets{catalogs: catalogs[index], containers: containers[index]}
		bytes := totalSize(part.containers)
		sample := runOnce(service, part, deep, len(part.catalogs)+len(part.containers), workers)
		fmt.Printf("| %d | %d | %.2f GB | %.2fs | %.2fs | %.2f | %.1f | %d | %.1f KB | %d |\n",
			workers, sample.files, float64(bytes)/(1<<30), sample.wall.Seconds(), sample.cpu.Seconds(),
			sample.cpu.Seconds()/sample.wall.Seconds(), sample.readMB, sample.readOps,
			sample.readMB*1024/float64(max(sample.readOps, 1)), sample.names)
	}
}

// runWarmPhase 在全量目标上按轮次轮流跑出一份热盘对比表
// runWarmPhase interleaves rounds over the full target set and prints the warm-cache comparison
func runWarmPhase(t *testing.T, service *SearchService, targets scanTargets, deep bool, total int, ladder []int, rounds int, title string) {
	samples := make(map[int][]benchSample, len(ladder))
	for round := 0; round < rounds; round++ {
		for _, workers := range ladder {
			sample := runOnce(service, targets, deep, total, workers)
			samples[workers] = append(samples[workers], sample)
			t.Logf("round=%d workers=%2d wall=%6.2fs cpu=%6.2fs read=%7.1fMB ops=%d names=%d",
				round, workers, sample.wall.Seconds(), sample.cpu.Seconds(), sample.readMB, sample.readOps, sample.names)
		}
	}

	fmt.Printf("\n%s / warm phase\n", title)
	fmt.Printf("| workers | best | median | mean | speedup | cpu/wall | alloc MB | peak heap MB | GC | read MB | names |\n")
	fmt.Printf("| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	var base time.Duration
	for _, workers := range ladder {
		median, mean, best := summarize(samples[workers])
		if base == 0 {
			base = best.wall
		}
		fmt.Printf("| %d | %.2fs | %.2fs | %.2fs | %.2fx | %.2f | %.0f | %.0f | %.1f | %.1f | %d |\n",
			workers, best.wall.Seconds(), median.wall.Seconds(), mean.wall.Seconds(),
			float64(base)/float64(best.wall), best.cpu.Seconds()/best.wall.Seconds(), best.allocMB,
			best.peakMB, float64(best.gcCycles), best.readMB, best.names)
	}
	fmt.Printf("\n")
}

// startHeapSampler 在后台每隔一小段读一次堆占用，返回峰值读取函数
// 用 runtime/metrics 而不是 ReadMemStats：后者会停世界，采样本身就会污染被测的墙钟
// startHeapSampler reads the heap size in the background and returns a function giving the peak
// It uses runtime/metrics rather than ReadMemStats because the latter stops the world and the sampling itself
// would pollute the wall time being measured
func startHeapSampler() func() float64 {
	samples := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	stop := make(chan struct{})
	var peak atomic.Uint64
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				metrics.Read(samples)
				if value := samples[0].Value.Uint64(); value > peak.Load() {
					peak.Store(value)
				}
			}
		}
	}()
	return func() float64 {
		close(stop)
		<-done
		return float64(peak.Load()) / (1 << 20)
	}
}

// runOnce 用给定 worker 数跑一遍扫描并测量耗时、CPU 与磁盘读取量
// runOnce scans once with the given worker count and measures duration, CPU time, and bytes read
func runOnce(service *SearchService, targets scanTargets, deep bool, total int, workers int) benchSample {
	scanWorkers = workers
	defer func() { scanWorkers = clampScanWorkers(runtime.NumCPU()) }()
	defer startProfile(workers)()

	peakHeap := startHeapSampler()
	ioBefore := readIoCounters()
	cpuBefore := readCpuTime()
	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	start := time.Now()
	catalogs, containers, cancelled := service.runScans(context.Background(), targets, deep, total)
	wall := time.Since(start)
	ioAfter := readIoCounters()
	runtime.ReadMemStats(&memAfter)
	sample := benchSample{
		wall:     wall,
		cpu:      readCpuTime() - cpuBefore,
		readMB:   float64(ioAfter.ReadTransferCount-ioBefore.ReadTransferCount) / (1 << 20),
		readOps:  ioAfter.ReadOperationCount - ioBefore.ReadOperationCount,
		files:    len(targets.catalogs) + len(targets.containers),
		allocMB:  float64(memAfter.TotalAlloc-memBefore.TotalAlloc) / (1 << 20),
		gcCycles: memAfter.NumGC - memBefore.NumGC,
		peakMB:   peakHeap(),
	}
	if cancelled {
		panic(fmt.Sprintf("workers=%d: the run reported a cancellation", workers))
	}
	for i := range catalogs {
		sample.names += len(catalogs[i].entries)
	}
	for i := range containers {
		sample.names += len(containers[i].entries)
	}
	return sample
}

// startProfile 在 ABA_BENCH_PROFILE 指向的目录里为这次运行起一份 profile，返回停止函数
// 墙钟平台期出现在核数远未用满的地方，只有 profile 能说清是 GC、系统调用还是某把锁在串行化
// startProfile begins a profile for this run in the directory named by ABA_BENCH_PROFILE and returns its stop function
// The warm curve flattens well before the cores run out, and only a profile can tell whether GC, syscalls,
// or one particular lock is doing the serializing
func startProfile(workers int) func() {
	dir := strings.TrimSpace(os.Getenv("ABA_BENCH_PROFILE"))
	if dir == "" {
		return func() {}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return func() {}
	}
	runtime.SetBlockProfileRate(1)
	runtime.SetMutexProfileFraction(1)

	cpu, err := os.Create(filepath.Join(dir, fmt.Sprintf("workers-%03d.cpu.pprof", workers)))
	if err != nil {
		return func() {}
	}
	pprof.StartCPUProfile(cpu)
	return func() {
		pprof.StopCPUProfile()
		cpu.Close()
		for _, name := range []string{"block", "mutex", "goroutine"} {
			profile := pprof.Lookup(name)
			if profile == nil {
				continue
			}
			file, err := os.Create(filepath.Join(dir, fmt.Sprintf("workers-%03d.%s.pprof", workers, name)))
			if err != nil {
				continue
			}
			profile.WriteTo(file, 0)
			file.Close()
		}
	}
}

// splitBySize 把文件按体积从大到小轮流分给 count 份，使各份的总字节数尽量接近
// 直接按数量均分会让某一份撞上几个特大的容器，冷读阶段各配置的读取量就对不上了
// splitBySize deals the files largest first into count parts so their total bytes end up close
// Splitting by count alone would put a few oversized containers in one part and leave the cold phase comparing
// configurations that read very different volumes
func splitBySize(paths []string, count int) [][]string {
	ordered := append([]string(nil), paths...)
	sizes := make(map[string]int64, len(paths))
	for _, path := range ordered {
		if info, err := os.Stat(path); err == nil {
			sizes[path] = info.Size()
		}
	}
	sort.SliceStable(ordered, func(left, right int) bool { return sizes[ordered[left]] > sizes[ordered[right]] })

	parts := make([][]string, count)
	for index, path := range ordered {
		slot := index % count
		parts[slot] = append(parts[slot], path)
	}
	return parts
}

// totalSize 求一组文件的总体积 / totalSize returns the combined size of a set of files
func totalSize(paths []string) int64 {
	var sum int64
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil {
			sum += info.Size()
		}
	}
	return sum
}

// summarize 把同一个 worker 数的多轮结果归成中位数、均值与最快的一轮
// summarize folds the rounds of one worker count into its median, mean, and fastest round
func summarize(rounds []benchSample) (median benchSample, mean benchSample, best benchSample) {
	if len(rounds) == 0 {
		return benchSample{}, benchSample{}, benchSample{}
	}
	ordered := append([]benchSample(nil), rounds...)
	sort.SliceStable(ordered, func(left, right int) bool { return ordered[left].wall < ordered[right].wall })
	median = ordered[len(ordered)/2]
	best = ordered[0]
	var wall, cpu, read float64
	var names int
	for _, round := range rounds {
		wall += round.wall.Seconds()
		cpu += round.cpu.Seconds()
		read += round.readMB
		names += round.names
	}
	count := float64(len(rounds))
	mean = benchSample{
		wall:   time.Duration(wall / count * float64(time.Second)),
		cpu:    time.Duration(cpu / count * float64(time.Second)),
		readMB: read / count,
		names:  names / len(rounds),
	}
	return median, mean, best
}

// parseWorkerLadder 解析逗号分隔的 worker 数阶梯，空串给出一份默认阶梯
// parseWorkerLadder parses the comma-separated worker ladder and falls back to a default one for an empty string
func parseWorkerLadder(value string) []int {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "1,2,4,8,12,16,20,28,40,64,128"
	}
	var ladder []int
	for _, field := range strings.Split(value, ",") {
		if count, err := strconv.Atoi(strings.TrimSpace(field)); err == nil && count > 0 {
			ladder = append(ladder, count)
		}
	}
	return ladder
}

// envInt 读一个整数环境变量，缺失或非法时用默认值
// envInt reads an integer environment variable and falls back to the default when it is missing or malformed
func envInt(name string, fallback int) int {
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name))); err == nil && value > 0 {
		return value
	}
	return fallback
}
