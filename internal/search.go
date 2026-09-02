package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	KCES "github.com/MeidoPromotionAssociation/MeidoSerialization/v2/serialization/KCES"
	"github.com/MeidoPromotionAssociation/MeidoSerialization/v2/serialization/KCES/aba"
	"github.com/MeidoPromotionAssociation/MeidoSerialization/v2/serialization/KCES/ct"
)

// IndexProgressEvent 是索引进度事件名，main.go 用它注册类型化事件
// IndexProgressEvent is the index-progress event name registered as a typed event in main.go
const IndexProgressEvent = "explorer:index-progress"

// 名字的来源层，决定一条记录是怎么找出来的，也决定前端显示的标签
// Origin layers record how a name was discovered and drive the label shown by the frontend
const (
	// OriginCatalog 来自 .ct 的 catalog 条目，游戏按名字加载资源时查的就是这张表
	// OriginCatalog comes from a .ct catalog item, the table the game consults when loading a resource by name
	OriginCatalog = "catalog"
	// OriginContainer 来自容器内 AssetBundle 对象的 m_Container 加载名
	// OriginContainer comes from the m_Container load names of the AssetBundle object inside a container
	OriginContainer = "container"
	// OriginMenu 来自 .menuassets 内的 Menu 条目，即游戏里的 .menu
	// OriginMenu comes from a Menu entry inside .menuassets, the in-game .menu
	OriginMenu = "menu"
	// OriginMaterial 来自 .materialassets 内的 Material 条目，即 .mate
	// OriginMaterial comes from a Material entry inside .materialassets, the .mate
	OriginMaterial = "material"
	// OriginPmat 来自 .pmatassets 内的 PriorityMaterial 条目，即 .pmat
	// OriginPmat comes from a PriorityMaterial entry inside .pmatassets, the .pmat
	OriginPmat = "pmat"
)

// containerSuffixes 是能被拆出内部名的容器条目扩展名，值为该容器产出的条目来源层
// containerSuffixes maps the extensions of container entries whose inner names can be extracted to the origin of the entries they yield
var containerSuffixes = map[string]string{
	".menuassets":     OriginMenu,
	".materialassets": OriginMaterial,
	".pmatassets":     OriginPmat,
}

// unityLoadSuffixes 是 Unity 追加在 KCES 名字之后构成 m_Container 加载名的资产扩展名
// 取值来自实测：.tex 存成 Texture2D/Sprite 得到 .png，TextAsset 得到 .bytes，Mesh 得到 .asset 或 .crmesh，
// SpriteAtlas 得到 .spriteatlas。像 .anim/.prefab/.wav/.csv 这种本身就是完整资产名的不在此列
// unityLoadSuffixes lists the Unity asset extensions appended after a KCES name to form an m_Container load name
// The values come from measurement: a .tex stored as Texture2D or Sprite yields .png, a TextAsset yields .bytes,
// a Mesh yields .asset or .crmesh, and a SpriteAtlas yields .spriteatlas
// Names that are already complete Unity asset names such as .anim, .prefab, .wav, and .csv are deliberately absent
var unityLoadSuffixes = map[string]bool{
	".png":         true,
	".bytes":       true,
	".asset":       true,
	".crmesh":      true,
	".spriteatlas": true,
}

// containerExtensions 是要打开并索引的 UnityFS 容器扩展名
// containerExtensions lists the UnityFS container extensions that get opened and indexed
var containerExtensions = map[string]bool{
	".aba":         true,
	".asset_bg":    true,
	".asset_scene": true,
}

// maxSearchLimit 是一次查询能返回的最大命中数，避免把整份索引推过前端桥
// maxSearchLimit caps the hits one query returns so a whole index is never pushed across the frontend bridge
const maxSearchLimit = 5000

// defaultSearchLimit 是前端不指定时的返回条数
// defaultSearchLimit is the hit count returned when the frontend does not specify one
const defaultSearchLimit = 500

// IndexOptions 是一次索引构建的输入 / IndexOptions is the input of one index build
type IndexOptions struct {
	Root string `json:"root"` // 要扫描的根目录 / Root directory to scan
	// Deep 打开后会解析 .menuassets/.materialassets/.pmatassets，取出 .menu/.mate/.pmat 这些
	// 只存在于容器内部、catalog 不会列出的名字
	// Deep parses .menuassets, .materialassets, and .pmatassets to recover the .menu, .mate, and .pmat names
	// that exist only inside those containers and are never listed by a catalog
	Deep bool `json:"deep"`
	// Refresh 打开后忽略磁盘缓存，从头重扫每个文件
	// 指纹只看大小与修改时间，理论上存在改了内容却保持两者不变的情况，这个开关是那种情况的出路
	// Refresh ignores the on-disk cache and rescans every file from scratch
	// Fingerprints only compare size and modification time, so content can in principle change while both
	// stay put, and this switch is the way out of that case
	Refresh bool `json:"refresh"`
}

// IndexProgress 是构建过程中推送给前端的进度 / IndexProgress is the progress pushed to the frontend during a build
type IndexProgress struct {
	Done     int    `json:"done"`     // 已处理文件数 / Files processed
	Total    int    `json:"total"`    // 待处理文件总数 / Total files to process
	Names    int    `json:"names"`    // 已收集的名字数 / Names collected so far
	Current  string `json:"current"`  // 最近处理的文件名 / Most recently processed file name
	Finished bool   `json:"finished"` // 构建是否已结束 / Whether the build has finished
}

// IndexStats 是一次索引构建的结果概况 / IndexStats summarizes the outcome of one index build
type IndexStats struct {
	Root       string   `json:"root"`       // 被索引的根目录 / Indexed root directory
	Deep       bool     `json:"deep"`       // 是否解析了容器内部名 / Whether inner container names were parsed
	Ready      bool     `json:"ready"`      // 索引是否可用 / Whether an index is available
	Building   bool     `json:"building"`   // 是否正在构建 / Whether a build is running
	Cancelled  bool     `json:"cancelled"`  // 上次构建是否被取消 / Whether the last build was cancelled
	Catalogs   int      `json:"catalogs"`   // 成功解析的 .ct 数 / Successfully parsed .ct count
	Containers int      `json:"containers"` // 成功打开的容器数 / Successfully opened container count
	Names      int      `json:"names"`      // 索引中的名字总数 / Total names in the index
	InnerNames int      `json:"innerNames"` // 其中来自容器内部的名字数 / Names among them coming from inside containers
	Reused     int      `json:"reused"`     // 直接沿用磁盘缓存的来源数 / Sources taken straight from the on-disk cache
	FromCache  bool     `json:"fromCache"`  // 整份索引是否直接来自缓存 / Whether the whole index came straight from the cache
	ElapsedMs  int64    `json:"elapsedMs"`  // 构建耗时毫秒 / Build duration in milliseconds
	Warnings   []string `json:"warnings"`   // 跳过的文件及原因 / Skipped files and their reasons
}

// SearchQuery 是一次查询的条件 / SearchQuery is the condition of one query
type SearchQuery struct {
	Text       string   `json:"text"`       // 匹配文本，大小写不敏感子串 / Match text, a case-insensitive substring
	Extensions []string `json:"extensions"` // 限定扩展名，空表示不限 / Restrict to extensions, empty means no restriction
	Origins    []string `json:"origins"`    // 限定来源层，空表示不限 / Restrict to origin layers, empty means no restriction
	Limit      int      `json:"limit"`      // 返回条数上限 / Maximum hits to return
}

// SearchHit 是一条命中记录 / SearchHit is one matched record
type SearchHit struct {
	Name          string `json:"name"`          // 资源名 / Resource name
	Extension     string `json:"extension"`     // 资源扩展名 / Resource extension
	Detail        string `json:"detail"`        // .menu 的游戏内显示名或 .mate 的 shader 名 / In-game display name of a .menu or shader name of a .mate
	Origin        string `json:"origin"`        // 名字来源层 / Origin layer of the name
	Owner         string `json:"owner"`         // 宿主容器条目名，直接资源为空 / Owning container entry name, empty for direct resources
	ContainerPath string `json:"containerPath"` // 所在容器绝对路径，未能定位时为空 / Absolute path of the owning container, empty when it could not be located
	ContainerName string `json:"containerName"` // 所在容器文件名 / File name of the owning container
	CatalogPath   string `json:"catalogPath"`   // 配套 .ct 绝对路径，没有则为空 / Absolute path of the paired .ct, empty when absent
}

// SearchResult 是一次查询的结果 / SearchResult is the outcome of one query
type SearchResult struct {
	Hits      []SearchHit `json:"hits"`      // 命中记录，已按相关度排序 / Matched records ordered by relevance
	Total     int         `json:"total"`     // 命中总数，未被上限裁剪 / Total match count before the limit is applied
	Truncated bool        `json:"truncated"` // 是否因上限被裁剪 / Whether the limit truncated the list
	ElapsedMs int64       `json:"elapsedMs"` // 查询耗时毫秒 / Query duration in milliseconds
}

// indexSource 是一个名字的归属文件，多条记录共享同一份，避免每条都存一遍长路径
// indexSource is the owning file of a name, shared by many records so a long path is not stored once per record
type indexSource struct {
	containerPath string // 容器绝对路径 / Absolute container path
	containerName string // 容器文件名 / Container file name
	catalogPath   string // 配套 .ct 绝对路径 / Absolute path of the paired .ct
}

// indexRecord 是索引中的一条名字
// lower 与 detailLower 供大小写不敏感匹配；KCES 名字与日文显示名基本不含大写，
// strings.ToLower 对无大写字母的字符串直接返回原串，所以这两个字段通常不额外占内存
// indexRecord is one name in the index
// lower and detailLower back case-insensitive matching; KCES names and Japanese display names rarely contain
// uppercase letters, and strings.ToLower returns the original string when there is nothing to fold,
// so these two fields usually cost no extra memory
// indexRecord 是索引中的一条名字
//
// lower 与 detailLower 供大小写不敏感匹配；KCES 名字与日文显示名基本不含大写，
// strings.ToLower 对无大写字母的字符串直接返回原串，所以这两个字段通常与原串共用底层数组。
//
// 扩展名、宿主条目与来源层都用下标而不是字符串：这三项的取值只有几十到几千种却每条记录都要带一份，
// 存字符串意味着每条多付三个 16 字节的头。22 GB 的资源约 130 万条记录，光这部分就是几十 MB
//
// indexRecord is one name in the index
//
// lower and detailLower back case-insensitive matching; KCES names and Japanese display names rarely contain
// uppercase letters, and strings.ToLower returns the original string when there is nothing to fold,
// so these two fields usually share their backing array with the original
//
// The extension, owning entry, and origin layer are stored as table indices rather than strings: each has only
// tens to thousands of distinct values yet rides along on every record, and strings would cost three extra
// 16-byte headers per record. At roughly 1.3M records for 22 GB of resources that alone runs to tens of megabytes
type indexRecord struct {
	name        string
	lower       string
	detail      string
	detailLower string
	source      int32
	owner       int32
	extension   int32
	origin      uint8
}

// noOwner 表示一条记录不是从容器条目内部挖出来的 / noOwner marks a record that was not dug out of a container entry
const noOwner int32 = -1

// originNames 把 origin 下标映射回名字，顺序即 originID 的取值
// originNames maps an origin index back to its name, and the order defines the values of originID
var originNames = []string{OriginCatalog, OriginContainer, OriginMenu, OriginMaterial, OriginPmat}

// originID 返回来源层的表下标，未知来源归到 catalog
// originID returns the table index of an origin layer and folds an unknown origin into catalog
func originID(origin string) uint8 {
	for index, name := range originNames {
		if name == origin {
			return uint8(index)
		}
	}
	return 0
}

// originName 返回来源层下标对应的名字 / originName returns the name for an origin index
func originName(id uint8) string {
	if int(id) < len(originNames) {
		return originNames[id]
	}
	return OriginCatalog
}

// nameIndex 是构建完成的索引 / nameIndex is a completed index
type nameIndex struct {
	sources    []indexSource
	extensions []string // 去重后的扩展名表 / Deduplicated extension table
	owners     []string // 去重后的宿主条目名表 / Deduplicated owning-entry table
	records    []indexRecord
	stats      IndexStats
}

// extensionOf 返回一条记录的扩展名 / extensionOf returns the extension of one record
func (index *nameIndex) extensionOf(record *indexRecord) string {
	if record.extension < 0 || int(record.extension) >= len(index.extensions) {
		return ""
	}
	return index.extensions[record.extension]
}

// ownerOf 返回一条记录的宿主条目名，直接资源为空
// ownerOf returns the owning entry name of one record, empty for a direct resource
func (index *nameIndex) ownerOf(record *indexRecord) string {
	if record.owner < 0 || int(record.owner) >= len(index.owners) {
		return ""
	}
	return index.owners[record.owner]
}

// SearchService 在一个目录树上建立资源名索引，用于回答"这个名字在哪个 aba 里"
// 游戏按名字加载资源，而名字分布在两层：.ct 的 catalog 列出容器里的资源，
// .menu/.mate/.pmat 这类名字只存在于容器内部的 .menuassets/.materialassets/.pmatassets 里，catalog 不会列
// SearchService indexes resource names across a directory tree to answer "which .aba holds this name"
// The game loads resources by name, and names live on two layers: a .ct catalog lists the resources of a container,
// while names such as .menu, .mate, and .pmat exist only inside the .menuassets, .materialassets, and .pmatassets
// entries within a container and never appear in a catalog
type SearchService struct {
	// emit 把进度事件送到前端，测试里为 nil 表示不推送
	// emit sends progress events to the frontend and is nil in tests, meaning no push
	emit func(name string, data any)

	mu    sync.RWMutex
	index *nameIndex

	building atomic.Bool

	// buildMu 保护 cancelBuild，它只在一次构建进行期间非空
	// CancelIndex 走这个句柄而不是一个标志位：标志位要在构建开始时清零，
	// 那就存在"取消请求先于构建的清零动作到达"从而被悄悄吃掉的窗口
	// buildMu guards cancelBuild, which is non-nil only while a build is in flight
	// CancelIndex goes through this handle rather than a flag because a flag has to be reset when a build starts,
	// which leaves a window where a cancel request arriving first is silently swallowed by that reset
	buildMu     sync.Mutex
	cancelBuild context.CancelFunc
}

// NewSearchService 创建全局搜索服务 / NewSearchService creates the global search service
func NewSearchService() *SearchService {
	return &SearchService{}
}

// SetEmitter 注入进度事件发送函数，由 main.go 在应用建好后调用
// SetEmitter injects the progress-event sender and is called by main.go once the application exists
//
//wails:ignore
func (s *SearchService) SetEmitter(emit func(name string, data any)) {
	s.emit = emit
}

// scanTargets 是扫描出的待索引文件清单 / scanTargets is the list of files to index found by a scan
type scanTargets struct {
	catalogs   []string
	containers []string
}

// without 去掉已经被缓存覆盖的文件，剩下的才需要重新解析
// without drops the files already covered by the cache, leaving only those that need reparsing
func (targets scanTargets) without(skip map[string]bool) scanTargets {
	if len(skip) == 0 {
		return targets
	}
	filter := func(paths []string) []string {
		kept := make([]string, 0, len(paths))
		for _, path := range paths {
			if !skip[strings.ToLower(path)] {
				kept = append(kept, path)
			}
		}
		return kept
	}
	return scanTargets{catalogs: filter(targets.catalogs), containers: filter(targets.containers)}
}

// scanRoot 遍历目录树，挑出 .ct 与 UnityFS 容器
// scanRoot walks the directory tree and picks out .ct files and UnityFS containers
func scanRoot(ctx context.Context, root string) (scanTargets, error) {
	var targets scanTargets
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			// 单个目录读不动不该中断整棵树的遍历
			// One unreadable directory must not abort the whole walk
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch {
		case ext == ".ct":
			targets.catalogs = append(targets.catalogs, path)
		case containerExtensions[ext]:
			targets.containers = append(targets.containers, path)
		}
		return nil
	})
	if err != nil {
		return scanTargets{}, err
	}
	return targets, nil
}

// catalogScan 是一个 .ct 的解析结果 / catalogScan is the parse result of one .ct
type catalogScan struct {
	catalogPath string
	// stamp 在打开文件之前取，读的过程中文件若被改动，存下的指纹对应的是旧内容，
	// 下次校验就会失配并重扫。反过来先读后取指纹会把新指纹配上旧数据，从此再也不会被发现
	// stamp is taken before the file is opened, so a file modified mid-read leaves a fingerprint of the old
	// content that fails the next check and triggers a rescan
	// Taking it after reading would instead pair a new fingerprint with old data and hide the staleness forever
	stamp fileStamp
	// containerName 是 catalog 声明的资源文件名，通常就是同目录下的 .aba
	// containerName is the resource file name declared by the catalog, usually the .aba next to it
	containerNames []string
	entries        []scannedName
	warning        string
}

// containerScan 是一个容器的解析结果 / containerScan is the parse result of one container
type containerScan struct {
	containerPath string
	stamp         fileStamp
	entries       []scannedName
	warning       string
}

// scannedName 是解析出的一条名字，尚未归并进索引
// scannedName is one parsed name that has not been merged into the index yet
type scannedName struct {
	name   string
	detail string
	owner  string
	origin string
	// containerName 只在 catalog 层有意义，指向 catalog 声明的资源文件
	// containerName matters only on the catalog layer and points at the resource file the catalog declares
	containerName string
}

// BuildIndex 在 root 下建立名字索引，期间按文件推送进度事件，可用 CancelIndex 中断
// 索引常驻内存直到进程退出或再次构建，因此重复查询是毫秒级的
// BuildIndex builds a name index under root, pushes a progress event per file, and can be interrupted with CancelIndex
// The index stays in memory until the process exits or another build starts, so repeated queries take milliseconds
func (s *SearchService) BuildIndex(ctx context.Context, options IndexOptions) (IndexStats, error) {
	root := strings.TrimSpace(options.Root)
	if root == "" {
		return IndexStats{}, errors.New("no directory was given to index")
	}
	info, err := os.Stat(root)
	if err != nil {
		return IndexStats{}, fmt.Errorf("read %q: %w", root, err)
	}
	if !info.IsDir() {
		return IndexStats{}, fmt.Errorf("%q is not a directory", root)
	}
	if !s.building.CompareAndSwap(false, true) {
		return s.IndexStatus(), errors.New("an index build is already running")
	}
	defer s.building.Store(false)

	// 把这次构建挂到一个可取消的 ctx 上，CancelIndex 通过它中断
	// Hook this build onto a cancellable ctx that CancelIndex interrupts
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.buildMu.Lock()
	s.cancelBuild = cancel
	s.buildMu.Unlock()
	defer func() {
		s.buildMu.Lock()
		s.cancelBuild = nil
		s.buildMu.Unlock()
	}()

	start := time.Now()
	targets, err := scanRoot(ctx, root)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return s.cancelledStats(root, options.Deep, start), nil
		}
		return IndexStats{}, fmt.Errorf("scan %q: %w", root, err)
	}

	// 复用磁盘缓存里仍然对得上的来源，只把变动过和新增的文件交给 worker
	// 22 GB 的资源全量重建要二十秒上下，而加装一个 MOD 通常只动几个文件
	// Reuse every cached source still matching disk and hand only changed or new files to the workers
	// A full rebuild over 22 GB runs about twenty seconds, while installing one MOD usually touches a few files
	reused, skip := s.reusableSources(root, options)
	targets = targets.without(skip)

	total := len(targets.catalogs) + len(targets.containers)
	if total == 0 && len(reused) == 0 {
		index := buildIndex(root, options.Deep, nil, nil)
		index.stats.ElapsedMs = time.Since(start).Milliseconds()
		s.store(index)
		s.saveCache(index.stats, nil, nil)
		s.emitProgress(IndexProgress{Total: 0, Finished: true})
		return index.stats, nil
	}

	catalogResults, containerResults, cancelledDuringWork := s.runScans(ctx, targets, options.Deep, total)
	if cancelledDuringWork {
		return s.cancelledStats(root, options.Deep, start), nil
	}

	scanned, warnings := mergeScans(catalogResults, containerResults)
	sources := combineSources(reused, scanned)
	index := buildIndex(root, options.Deep, sources, warnings)
	index.stats.ElapsedMs = time.Since(start).Milliseconds()
	index.stats.Reused = len(reused)
	s.store(index)
	s.saveCache(index.stats, sources, warnings)
	s.emitProgress(IndexProgress{Done: total, Total: total, Names: index.stats.Names, Finished: true})
	return index.stats, nil
}

// reusableSources 从磁盘缓存里挑出仍然对得上的来源，并给出这些来源已经覆盖的文件路径
// 缓存对应的根目录或深度不同就整份作废：浅索引缺少容器内部名，拿它冒充深索引会让 .menu 查不到还查不出原因
// reusableSources picks the still-matching sources out of the on-disk cache and reports which file paths
// they already cover
// A cache for another root or another depth is discarded whole: a shallow index lacks inner container names,
// and passing it off as a deep one would make .menu lookups silently come up empty
func (s *SearchService) reusableSources(root string, options IndexOptions) ([]indexedSource, map[string]bool) {
	if options.Refresh {
		return nil, nil
	}
	cache, err := readIndexCache(root)
	if err != nil || cache == nil {
		return nil, nil
	}
	if !strings.EqualFold(filepath.Clean(cache.Root), filepath.Clean(root)) || cache.Deep != options.Deep {
		return nil, nil
	}
	reused := make([]indexedSource, 0, len(cache.Sources))
	skip := make(map[string]bool, len(cache.Sources)*2)
	for i := range cache.Sources {
		source := &cache.Sources[i]
		if !source.current() {
			continue
		}
		reused = append(reused, *source)
		if source.container.Path != "" {
			skip[strings.ToLower(source.container.Path)] = true
		}
		if source.catalog.Path != "" {
			skip[strings.ToLower(source.catalog.Path)] = true
		}
	}
	return reused, skip
}

// saveCache 把这次构建的结果写回磁盘，失败只记进警告
// 缓存写不下去不该让一份已经建好、完全可用的索引显示为失败
// saveCache writes the result of this build back to disk and records a failure as a warning only
// A cache that cannot be written must not make an already built, fully usable index report as failed
func (s *SearchService) saveCache(stats IndexStats, sources []indexedSource, warnings []string) {
	if err := writeIndexCache(&indexCache{
		Root:     stats.Root,
		Deep:     stats.Deep,
		Sources:  sources,
		Warnings: warnings,
	}); err != nil {
		s.mu.Lock()
		if s.index != nil {
			s.index.stats.Warnings = append(s.index.stats.Warnings, "index cache: "+err.Error())
		}
		s.mu.Unlock()
	}
}

// LoadCachedIndex 直接装载一个根目录的磁盘缓存，只在缓存完整且每个文件都没变过时成功
// 有任何一处对不上就返回未就绪，让用户去走构建：这里悄悄用半份缓存会让搜索结果少东西却看不出来
// LoadCachedIndex loads the on-disk cache of a root directory and succeeds only when the cache is complete
// and every file is unchanged
// Any mismatch returns a not-ready status so the user runs a build instead, because silently serving half a
// cache would drop results without any visible sign
func (s *SearchService) LoadCachedIndex(options IndexOptions) (IndexStats, error) {
	root := strings.TrimSpace(options.Root)
	if root == "" {
		return IndexStats{}, nil
	}
	cache, err := readIndexCache(root)
	if err != nil || cache == nil {
		return IndexStats{}, nil
	}
	if !strings.EqualFold(filepath.Clean(cache.Root), filepath.Clean(root)) || cache.Deep != options.Deep {
		return IndexStats{}, nil
	}
	for i := range cache.Sources {
		if !cache.Sources[i].current() {
			return IndexStats{}, nil
		}
	}
	start := time.Now()
	index := buildIndex(root, cache.Deep, cache.Sources, cache.Warnings)
	index.stats.ElapsedMs = time.Since(start).Milliseconds()
	index.stats.Reused = len(cache.Sources)
	index.stats.FromCache = true
	s.store(index)
	return index.stats, nil
}

// ClearCache 删除一个根目录的磁盘缓存 / ClearCache deletes the on-disk cache of a root directory
func (s *SearchService) ClearCache(root string) error {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	return removeIndexCache(root)
}

// runScans 用 worker 池并行解析全部目标文件
// 单个文件的解析是"少量随机读 + LZ4 解压 + MessagePack 解码"，读的字节数不到容器体积的 1%，
// 因此瓶颈在每文件的寻道与解码而不是吞吐，并行能实打实地缩短墙钟时间
// runScans parses every target file in parallel with a worker pool
// Parsing one file is a handful of random reads plus LZ4 decompression and MessagePack decoding, and it touches
// under one percent of a container's bytes, so the bottleneck is per-file seeking and decoding rather than
// throughput, which is exactly what parallelism shortens
func (s *SearchService) runScans(
	ctx context.Context,
	targets scanTargets,
	deep bool,
	total int,
) ([]catalogScan, []containerScan, bool) {
	workers := runtime.NumCPU()
	if workers > 16 {
		workers = 16
	}
	if workers < 1 {
		workers = 1
	}

	catalogResults := make([]catalogScan, len(targets.catalogs))
	containerResults := make([]containerScan, len(targets.containers))

	type job struct {
		index     int
		path      string
		isCatalog bool
	}
	jobs := make(chan job)

	var done int64
	var names int64
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				// 取消后仍要把 jobs 排空，但不再为已入队的文件做无用功
				// Draining jobs after a cancellation is still required, but queued files no longer get pointless work
				if ctx.Err() != nil {
					continue
				}
				if item.isCatalog {
					result := scanCatalogFile(item.path)
					atomic.AddInt64(&names, int64(len(result.entries)))
					catalogResults[item.index] = result
				} else {
					result := scanContainerFile(item.path, deep)
					atomic.AddInt64(&names, int64(len(result.entries)))
					containerResults[item.index] = result
				}
				count := atomic.AddInt64(&done, 1)
				// 每 32 个文件推一次进度，逐个推会让事件桥成为瓶颈
				// A progress event every 32 files keeps the event bridge from becoming the bottleneck
				if count%32 == 0 || int(count) == total {
					s.emitProgress(IndexProgress{
						Done:    int(count),
						Total:   total,
						Names:   int(atomic.LoadInt64(&names)),
						Current: filepath.Base(item.path),
					})
				}
			}
		}()
	}

	cancelled := false
dispatch:
	for index, path := range targets.catalogs {
		if ctx.Err() != nil {
			cancelled = true
			break dispatch
		}
		jobs <- job{index: index, path: path, isCatalog: true}
	}
	if !cancelled {
		for index, path := range targets.containers {
			if ctx.Err() != nil {
				cancelled = true
				break
			}
			jobs <- job{index: index, path: path}
		}
	}
	close(jobs)
	wg.Wait()
	// 派发循环可能刚好在最后一个文件之后才看到取消，此时结果其实是完整的
	// The dispatch loop can notice a cancellation only after the last file, in which case the results are actually complete
	return catalogResults, containerResults, cancelled || ctx.Err() != nil
}

// cancelledStats 组装一次被取消的构建的状态，索引保持上一次的内容不变
// cancelledStats builds the status of a cancelled run and leaves the previous index untouched
func (s *SearchService) cancelledStats(root string, deep bool, start time.Time) IndexStats {
	stats := s.IndexStatus()
	stats.Root = root
	stats.Deep = deep
	stats.Cancelled = true
	stats.Building = false
	stats.ElapsedMs = time.Since(start).Milliseconds()
	s.emitProgress(IndexProgress{Finished: true})
	return stats
}

// scanCatalogFile 解析一个 .ct，取出 catalog 里的全部资源名
// scanCatalogFile parses one .ct and extracts every resource name from its catalog
func scanCatalogFile(path string) catalogScan {
	result := catalogScan{catalogPath: path, stamp: stampFile(path)}
	handle, err := os.Open(path)
	if err != nil {
		result.warning = fmt.Sprintf("%s: %v", filepath.Base(path), err)
		return result
	}
	defer handle.Close()

	table, err := ct.ReadContentTable(handle)
	if err != nil {
		result.warning = fmt.Sprintf("%s: %v", filepath.Base(path), err)
		return result
	}
	catalog, err := ct.DecodeCatalogFromCt(table)
	if err != nil {
		result.warning = fmt.Sprintf("%s: catalog: %v", filepath.Base(path), err)
		return result
	}

	for _, name := range catalog.ResourceFileNames {
		if name != nil && *name != "" {
			result.containerNames = append(result.containerNames, *name)
		}
	}
	result.entries = make([]scannedName, 0, len(catalog.Items)+len(catalog.VirtualItems))
	for _, item := range catalog.Items {
		if item == nil || item.Name == nil || *item.Name == "" {
			continue
		}
		containerName := ""
		if index := int(item.ResourceIndex); index >= 0 && index < len(catalog.ResourceFileNames) {
			if resource := catalog.ResourceFileNames[index]; resource != nil {
				containerName = *resource
			}
		}
		result.entries = append(result.entries, scannedName{
			name:          *item.Name,
			origin:        OriginCatalog,
			containerName: containerName,
		})
	}
	// VirtualAsset catalog 的条目不指向 AssetBundle，资源就在游戏工程路径下
	// VirtualAsset catalog items do not point at an AssetBundle; the resource lives at a game project path
	for _, item := range catalog.VirtualItems {
		if item == nil || item.Name == nil || *item.Name == "" {
			continue
		}
		detail := ""
		if item.AssetPath != nil {
			detail = *item.AssetPath
		}
		result.entries = append(result.entries, scannedName{
			name:   *item.Name,
			detail: detail,
			origin: OriginCatalog,
		})
	}
	return result
}

// scanContainerFile 打开一个容器，取出 m_Container 加载名；deep 为真时再解析容器内部名
// scanContainerFile opens a container and extracts its m_Container load names, then parses inner names when deep is set
func scanContainerFile(path string, deep bool) containerScan {
	result := containerScan{containerPath: path, stamp: stampFile(path)}
	handle, err := os.Open(path)
	if err != nil {
		result.warning = fmt.Sprintf("%s: %v", filepath.Base(path), err)
		return result
	}
	defer handle.Close()

	abaFile, err := aba.ReadAba(handle)
	if err != nil {
		result.warning = fmt.Sprintf("%s: %v", filepath.Base(path), err)
		return result
	}

	for index := range abaFile.BlockInfo.DirectoryInfos {
		directory := &abaFile.BlockInfo.DirectoryInfos[index]
		if !directory.IsSerialized() {
			continue
		}
		directoryIndex := int64(index)
		assetsFile, err := aba.ReadAssetsFileRange(directory.DecompressedSize, func(offset int64, size int64) ([]byte, error) {
			return abaFile.GetFileDataRange(directoryIndex, offset, size)
		})
		if err != nil {
			result.warning = fmt.Sprintf("%s: %v", filepath.Base(path), err)
			continue
		}
		containers, err := assetsFile.GetAssetBundleContainerMap()
		if err != nil {
			result.warning = fmt.Sprintf("%s: m_Container: %v", filepath.Base(path), err)
			continue
		}
		for pathID, loadName := range containers {
			resourceName := resourceNameFromLoadName(loadName)
			if resourceName == "" {
				continue
			}
			result.entries = append(result.entries, scannedName{
				name:   resourceName,
				origin: OriginContainer,
			})
			if !deep {
				continue
			}
			origin, ok := containerSuffixes[strings.ToLower(filepath.Ext(resourceName))]
			if !ok {
				continue
			}
			info := assetsFile.GetAssetInfoByPathID(pathID)
			if info == nil || info.TypeId != aba.ClassIDTextAsset {
				continue
			}
			_, script, err := assetsFile.GetTextAssetData(info)
			if err != nil {
				result.warning = fmt.Sprintf("%s/%s: %v", filepath.Base(path), resourceName, err)
				continue
			}
			inner, err := decodeInnerNames(origin, script)
			if err != nil {
				result.warning = fmt.Sprintf("%s/%s: %v", filepath.Base(path), resourceName, err)
				continue
			}
			for i := range inner {
				inner[i].owner = resourceName
			}
			result.entries = append(result.entries, inner...)
		}
	}
	return result
}

// decodeInnerNames 解出一个容器条目里的全部内部名
// decodeInnerNames extracts every inner name from one container entry
func decodeInnerNames(origin string, script []byte) ([]scannedName, error) {
	switch origin {
	case OriginMenu:
		assets, err := KCES.DecodeMenuAssets(script)
		if err != nil {
			return nil, err
		}
		names := make([]scannedName, 0, len(assets.Assets))
		for _, menu := range assets.Assets {
			if menu == nil || menu.FileName == nil || *menu.FileName == "" {
				continue
			}
			// ItemName 是游戏编辑界面显示的名字，用户往往只记得它而不是文件名
			// ItemName is the label shown in the game's edit UI, and it is often all a user remembers
			detail := ""
			if menu.ItemName != nil {
				detail = *menu.ItemName
			}
			names = append(names, scannedName{name: *menu.FileName, detail: detail, origin: OriginMenu})
		}
		return names, nil
	case OriginMaterial:
		assets, err := KCES.DecodeMaterialAssets(script)
		if err != nil {
			return nil, err
		}
		names := make([]scannedName, 0, len(assets.Assets))
		for _, material := range assets.Assets {
			if material == nil || material.FileName == nil || *material.FileName == "" {
				continue
			}
			detail := ""
			if material.ShaderName != nil {
				detail = *material.ShaderName
			}
			names = append(names, scannedName{name: *material.FileName, detail: detail, origin: OriginMaterial})
		}
		return names, nil
	case OriginPmat:
		assets, err := KCES.DecodePriorityMaterialAssets(script)
		if err != nil {
			return nil, err
		}
		names := make([]scannedName, 0, len(assets.Assets))
		for _, material := range assets.Assets {
			if material == nil || material.FileName == nil || *material.FileName == "" {
				continue
			}
			names = append(names, scannedName{name: *material.FileName, origin: OriginPmat})
		}
		return names, nil
	default:
		return nil, fmt.Errorf("unknown container origin %q", origin)
	}
}

// resourceNameFromLoadName 把 m_Container 加载名还原成 KCES 资源名
// 加载名形如 assets/gamedata/.../crc_dress013_stkg.tex.png，末段是 Unity 存储该资源用的资产扩展名。
// 只有名字里还剩一个扩展名时才剥离，所以 bgm001.wav、parts_check.csv 这类本身就完整的名字不会被削掉
// resourceNameFromLoadName recovers the KCES resource name from an m_Container load name
// A load name looks like assets/gamedata/.../crc_dress013_stkg.tex.png where the final segment is the Unity asset
// extension used to store the resource. The suffix is stripped only when an extension remains underneath, so names
// that are already complete such as bgm001.wav and parts_check.csv keep their tail
func resourceNameFromLoadName(loadName string) string {
	base := loadName
	if index := strings.LastIndexAny(base, "/\\"); index >= 0 {
		base = base[index+1:]
	}
	if base == "" {
		return ""
	}
	suffix := strings.ToLower(filepath.Ext(base))
	if !unityLoadSuffixes[suffix] {
		return base
	}
	stripped := base[:len(base)-len(suffix)]
	if filepath.Ext(stripped) == "" {
		return base
	}
	return stripped
}

// mergeScans 把并行解析的结果按来源归组并去重
// catalog 与 m_Container 描述的是同一批资源，同一个来源里同名同宿主的条目只保留信息最全的一条
// 归组的单位是"一个容器加它配套的 .ct"，这同时也是缓存失效的单位：这两个文件任一变了就只重扫这一组
// mergeScans groups parallel scan results by source and deduplicates within each
// A catalog and an m_Container describe the same resources, so one source keeps only the richest entry for a
// given name and owner
// The grouping unit is one container plus its paired .ct, which doubles as the cache invalidation unit:
// when either file changes only that one group is rescanned
func mergeScans(catalogs []catalogScan, containers []containerScan) ([]indexedSource, []string) {
	var sources []indexedSource
	var warnings []string

	// 容器绝对路径 -> sources 下标，catalog 只写得出资源文件名，靠同目录解析成绝对路径
	// Absolute container path to sources index; a catalog only names the resource file, which is resolved
	// against the directory holding the .ct
	sourceByKey := make(map[string]int, len(containers)+len(catalogs))
	// 每个来源一张去重表，键是"小写名 + 宿主"
	// One dedupe table per source keyed by lowercase name plus owner
	seen := make([]map[string]int, 0, len(containers)+len(catalogs))

	sourceFor := func(container fileStamp, containerName string, catalog fileStamp) int {
		key := strings.ToLower(container.Path)
		if key == "" {
			// 定位不到容器文件时退回用 catalog 路径当归属，至少还能告诉用户是哪个 .ct 列出的
			// Without a locatable container the .ct path becomes the owner so the user still learns which catalog listed it
			key = "ct:" + strings.ToLower(catalog.Path)
		}
		if existing, ok := sourceByKey[key]; ok {
			source := &sources[existing]
			if source.catalog.Path == "" {
				source.catalog = catalog
			}
			if source.container.Path == "" {
				source.container = container
			}
			if source.containerName == "" {
				source.containerName = containerName
			}
			return existing
		}
		sources = append(sources, indexedSource{
			containerName: containerName,
			container:     container,
			catalog:       catalog,
		})
		seen = append(seen, map[string]int{})
		id := len(sources) - 1
		sourceByKey[key] = id
		return id
	}

	add := func(source int, entry scannedName) {
		key := strings.ToLower(entry.name) + "\x00" + strings.ToLower(entry.owner)
		if existing, ok := seen[source][key]; ok {
			// 已有同名条目时补齐缺失字段：容器层没有显示名，catalog 层没有宿主条目
			// Fill the gaps of an existing entry: the container layer lacks a display name and the catalog layer lacks an owning entry
			kept := &sources[source].entries[existing]
			if kept.detail == "" && entry.detail != "" {
				kept.detail = entry.detail
			}
			// catalog 说明游戏的查找表确实列了这个名字，比"容器里有这么个对象"更有说服力
			// A catalog says the game's lookup table really lists this name, which is a stronger statement than merely finding the object in a container
			if kept.origin == OriginContainer && entry.origin != OriginContainer {
				kept.origin = entry.origin
			}
			return
		}
		seen[source][key] = len(sources[source].entries)
		sources[source].entries = append(sources[source].entries, scannedName{
			name:   entry.name,
			detail: entry.detail,
			owner:  entry.owner,
			origin: entry.origin,
		})
	}

	// 先并容器层：容器路径是索引里最可靠的归属键，catalog 随后按文件名挂上去
	// Containers go first because a container path is the most reliable owner key in the index, and catalogs attach by file name afterwards
	for _, scan := range containers {
		if scan.warning != "" {
			warnings = append(warnings, scan.warning)
		}
		if len(scan.entries) == 0 {
			continue
		}
		source := sourceFor(scan.stamp, filepath.Base(scan.containerPath), fileStamp{})
		for _, entry := range scan.entries {
			add(source, entry)
		}
	}

	for _, scan := range catalogs {
		if scan.warning != "" {
			warnings = append(warnings, scan.warning)
		}
		if len(scan.entries) == 0 && len(scan.containerNames) == 0 {
			continue
		}
		directory := filepath.Dir(scan.catalogPath)
		// catalog 声明的资源文件按约定就在 .ct 旁边，解析成绝对路径才能和容器层归到一起
		// The resource file a catalog declares sits next to the .ct by convention, and resolving it to an
		// absolute path is what lets catalog and container records land on the same source
		resolve := func(containerName string) (fileStamp, string) {
			if containerName == "" {
				return fileStamp{}, ""
			}
			candidate := filepath.Join(directory, containerName)
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
				return stampFile(candidate), containerName
			}
			return fileStamp{}, containerName
		}
		for _, entry := range scan.entries {
			container, containerName := resolve(entry.containerName)
			add(sourceFor(container, containerName, scan.stamp), entry)
		}
		// 即使 catalog 没有条目也要把 .ct 挂到容器上，前端才能从命中直接打开配套内容表
		// Even an empty catalog is attached to its container so the frontend can open the paired content table from a hit
		for _, containerName := range scan.containerNames {
			container, resolvedName := resolve(containerName)
			sourceFor(container, resolvedName, scan.stamp)
		}
	}
	return sources, warnings
}

// combineSources 把缓存里沿用的来源和这次新扫的来源拼起来，指向同一个容器的两份合并成一份
// 一个 .aba 通常只有一个 .ct，但两个 .ct 声明同一个资源文件时，其中一个没变而另一个变了，
// 就会既沿用旧的又扫出新的，同一批名字于是出现两遍
// combineSources joins the sources reused from the cache with the freshly scanned ones and folds two
// entries pointing at the same container into one
// One .aba normally has a single .ct, but when two .ct files declare the same resource file and only one of
// them changed, the pair would be both reused and rescanned, listing the same names twice
func combineSources(reused []indexedSource, scanned []indexedSource) []indexedSource {
	if len(reused) == 0 {
		return scanned
	}
	byKey := make(map[string]int, len(reused))
	key := func(source *indexedSource) string {
		if source.container.Path != "" {
			return strings.ToLower(source.container.Path)
		}
		return "ct:" + strings.ToLower(source.catalog.Path)
	}
	combined := make([]indexedSource, 0, len(reused)+len(scanned))
	for _, source := range reused {
		byKey[key(&source)] = len(combined)
		combined = append(combined, source)
	}
	for _, source := range scanned {
		existing, ok := byKey[key(&source)]
		if !ok {
			byKey[key(&source)] = len(combined)
			combined = append(combined, source)
			continue
		}
		target := &combined[existing]
		seen := make(map[string]bool, len(target.entries))
		for _, entry := range target.entries {
			seen[strings.ToLower(entry.name)+"\x00"+strings.ToLower(entry.owner)] = true
		}
		for _, entry := range source.entries {
			if !seen[strings.ToLower(entry.name)+"\x00"+strings.ToLower(entry.owner)] {
				target.entries = append(target.entries, entry)
			}
		}
		if target.catalog.Path == "" {
			target.catalog = source.catalog
		}
	}
	return combined
}

// buildIndex 把按来源归好的条目摊平成可查询的索引
// buildIndex flattens source-grouped entries into a queryable index
func buildIndex(root string, deep bool, sources []indexedSource, warnings []string) *nameIndex {
	index := &nameIndex{
		sources: make([]indexSource, 0, len(sources)),
		stats:   IndexStats{Root: root, Deep: deep, Ready: true, Warnings: warnings},
	}

	// 记录切片一次分配到位：按需增长会翻倍扩容，百万级记录上要反复搬运整个数组
	// Allocate the record slice once, since growing on demand doubles the capacity and repeatedly copies
	// the whole array at a million records
	entryTotal := 0
	for i := range sources {
		entryTotal += len(sources[i].entries)
	}
	index.records = make([]indexRecord, 0, entryTotal)

	// intern 把低基数的字符串收敛到一张表，返回下标
	// 扩展名只有几十种、宿主条目名每个容器几个，让上百万条记录各存一份字符串纯属浪费
	// intern folds a low-cardinality string into a table and returns its index
	// There are only tens of extensions and a handful of owning entries per container, so letting a million
	// records each carry their own copy is pure waste
	intern := func(table *[]string, ids map[string]int32, value string) int32 {
		if id, ok := ids[value]; ok {
			return id
		}
		*table = append(*table, value)
		id := int32(len(*table) - 1)
		ids[value] = id
		return id
	}
	extensionIDs := map[string]int32{}
	ownerIDs := map[string]int32{}

	for i := range sources {
		source := &sources[i]
		index.sources = append(index.sources, indexSource{
			containerPath: source.container.Path,
			containerName: source.containerName,
			catalogPath:   source.catalog.Path,
		})
		if source.container.Path != "" {
			index.stats.Containers++
		}
		if source.catalog.Path != "" {
			index.stats.Catalogs++
		}
		sourceID := int32(len(index.sources) - 1)
		for _, entry := range source.entries {
			owner := noOwner
			if entry.owner != "" {
				owner = intern(&index.owners, ownerIDs, entry.owner)
				index.stats.InnerNames++
			}
			index.records = append(index.records, indexRecord{
				name:        entry.name,
				lower:       strings.ToLower(entry.name),
				detail:      entry.detail,
				detailLower: strings.ToLower(entry.detail),
				extension:   intern(&index.extensions, extensionIDs, strings.ToLower(filepath.Ext(entry.name))),
				owner:       owner,
				origin:      originID(entry.origin),
				source:      sourceID,
			})
		}
	}

	index.stats.Names = len(index.records)
	sort.Strings(index.stats.Warnings)
	return index
}

// store 替换当前索引 / store replaces the current index
func (s *SearchService) store(index *nameIndex) {
	s.mu.Lock()
	s.index = index
	s.mu.Unlock()
}

// emitProgress 推送一次进度，未注入发送函数时静默跳过
// emitProgress pushes one progress update and silently skips when no sender was injected
func (s *SearchService) emitProgress(progress IndexProgress) {
	if s.emit == nil {
		return
	}
	s.emit(IndexProgressEvent, progress)
}

// CancelIndex 请求中断正在进行的构建，已建成的索引保持不变
// 没有构建在跑时是空操作，不会影响下一次 BuildIndex
// CancelIndex asks a running build to stop and leaves an already built index untouched
// It is a no-op when no build is running and never affects the next BuildIndex
func (s *SearchService) CancelIndex() {
	s.buildMu.Lock()
	cancel := s.cancelBuild
	s.buildMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ClearIndex 丢弃当前索引 / ClearIndex discards the current index
func (s *SearchService) ClearIndex() {
	s.store(nil)
}

// IndexStatus 返回当前索引状态，供前端在页面挂载时恢复显示
// IndexStatus returns the current index status so the frontend can restore its display on mount
func (s *SearchService) IndexStatus() IndexStats {
	s.mu.RLock()
	index := s.index
	s.mu.RUnlock()

	stats := IndexStats{}
	if index != nil {
		stats = index.stats
	}
	stats.Building = s.building.Load()
	return stats
}

// Search 在当前索引里查名字，按相关度排序返回
// Search looks up names in the current index and returns them ordered by relevance
func (s *SearchService) Search(query SearchQuery) SearchResult {
	start := time.Now()
	s.mu.RLock()
	index := s.index
	s.mu.RUnlock()

	result := SearchResult{Hits: []SearchHit{}}
	if index == nil {
		return result
	}

	text := strings.ToLower(strings.TrimSpace(query.Text))
	// 过滤条件先换算成下标集合，让上百万条记录的热循环只比整数而不是逐条比字符串
	// Filters are converted to index sets up front so the hot loop over a million records compares integers rather than strings
	extensions := index.extensionFilter(query.Extensions)
	origins := originFilter(query.Origins)
	limit := query.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	// 按相关度分桶收集，每桶收满 limit 就只继续计数不再收
	// 全量排序在大索引上会拖垮逐键输入：22 GB 的资源约 85 万条名字，一次空查询要排完整份索引。
	// 分桶保证精确匹配永远不会被后面的子串命中挤掉，同时把排序规模压在 limit 量级
	// Collect into relevance buckets, and once a bucket holds limit records only keep counting
	// Sorting every match stalls per-keystroke input on a large index: 22 GB of resources hold roughly 850k names
	// and one empty query would sort the whole thing. Bucketing keeps exact matches from ever being crowded out
	// by later substring hits while holding the sort down to the order of limit
	var buckets [rankCount][]*indexRecord
	total := 0

	for i := range index.records {
		record := &index.records[i]
		if extensions != nil && !extensions[record.extension] {
			continue
		}
		if origins != nil && !origins[record.origin] {
			continue
		}
		rank, ok := matchRank(record, text)
		if !ok {
			continue
		}
		total++
		if len(buckets[rank]) < limit {
			buckets[rank] = append(buckets[rank], record)
		}
	}

	result.Total = total
	result.Hits = make([]SearchHit, 0, limit)
	for rank := range buckets {
		bucket := buckets[rank]
		// 同一桶内相关度相同，按名字排序让同族资源挨在一起
		// Records inside one bucket share a relevance, so sorting by name keeps a resource family together
		sort.SliceStable(bucket, func(left, right int) bool {
			if bucket[left].lower != bucket[right].lower {
				return bucket[left].lower < bucket[right].lower
			}
			return index.sources[bucket[left].source].containerName < index.sources[bucket[right].source].containerName
		})
		for _, record := range bucket {
			if len(result.Hits) >= limit {
				break
			}
			source := index.sources[record.source]
			result.Hits = append(result.Hits, SearchHit{
				Name:          record.name,
				Extension:     index.extensionOf(record),
				Detail:        record.detail,
				Origin:        originName(record.origin),
				Owner:         index.ownerOf(record),
				ContainerPath: source.containerPath,
				ContainerName: source.containerName,
				CatalogPath:   source.catalogPath,
			})
		}
	}
	result.Truncated = total > len(result.Hits)
	result.ElapsedMs = time.Since(start).Milliseconds()
	return result
}

// 匹配等级，数值越小越靠前 / Match ranks, where a smaller value sorts first
const (
	rankExactName  = 0 // 名字完全相同 / The name is identical
	rankPrefixName = 1 // 名字以查询开头 / The name starts with the query
	rankInName     = 2 // 名字包含查询 / The name contains the query
	rankInDetail   = 3 // 只有显示名包含查询 / Only the display name contains the query
	rankCount      = 4 // 等级总数，用作分桶数组长度 / Number of ranks, used as the bucket array length
)

// matchRank 判断一条记录是否命中并给出相关度，空查询按名字顺序全量返回
// matchRank decides whether a record matches and scores its relevance; an empty query returns everything by name order
func matchRank(record *indexRecord, text string) (int, bool) {
	if text == "" {
		return rankInName, true
	}
	switch {
	case record.lower == text:
		return rankExactName, true
	case strings.HasPrefix(record.lower, text):
		return rankPrefixName, true
	case strings.Contains(record.lower, text):
		return rankInName, true
	case record.detailLower != "" && strings.Contains(record.detailLower, text):
		return rankInDetail, true
	default:
		return 0, false
	}
}

// extensionFilter 把扩展名过滤列表换算成下标集合，空列表返回 nil 表示不过滤
// 查询里出现索引中不存在的扩展名时该项直接落空，集合非空但匹配不到任何记录，这正是期望的结果
// extensionFilter converts an extension filter list into an index set and returns nil for an empty list, meaning no filtering
// An extension absent from the index simply never matches, which leaves a non-empty set that selects nothing, exactly as intended
func (index *nameIndex) extensionFilter(values []string) map[int32]bool {
	if len(values) == 0 {
		return nil
	}
	lookup := make(map[string]int32, len(index.extensions))
	for id, extension := range index.extensions {
		lookup[extension] = int32(id)
	}
	set := make(map[int32]bool, len(values))
	for _, value := range values {
		if id, ok := lookup[strings.ToLower(strings.TrimSpace(value))]; ok {
			set[id] = true
		}
	}
	if len(set) == 0 {
		// 过滤项一个都对不上时也要保留一个空集，否则会被当成"不过滤"而返回全部记录
		// An all-miss filter still needs a non-nil empty set, otherwise it would read as no filtering and return everything
		return map[int32]bool{}
	}
	return set
}

// originFilter 把来源层过滤列表换算成下标集合，空列表返回 nil 表示不过滤
// originFilter converts an origin filter list into an index set and returns nil for an empty list, meaning no filtering
func originFilter(values []string) map[uint8]bool {
	if len(values) == 0 {
		return nil
	}
	set := make(map[uint8]bool, len(values))
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		for id, name := range originNames {
			if name == trimmed {
				set[uint8(id)] = true
			}
		}
	}
	return set
}

// IndexFacets 是当前索引里出现过的扩展名与来源层，供前端生成过滤选项
// IndexFacets lists the extensions and origin layers present in the current index so the frontend can build filters
type IndexFacets struct {
	Extensions []FacetCount `json:"extensions"` // 扩展名及其数量 / Extensions and their counts
	Origins    []FacetCount `json:"origins"`    // 来源层及其数量 / Origin layers and their counts
}

// FacetCount 是一个过滤项及其命中数 / FacetCount is one filter value and its count
type FacetCount struct {
	Value string `json:"value"` // 过滤值 / Filter value
	Count int    `json:"count"` // 索引中的数量 / Count in the index
}

// Facets 统计索引里各扩展名与来源层的数量
// Facets counts the extensions and origin layers in the index
func (s *SearchService) Facets() IndexFacets {
	s.mu.RLock()
	index := s.index
	s.mu.RUnlock()

	facets := IndexFacets{Extensions: []FacetCount{}, Origins: []FacetCount{}}
	if index == nil {
		return facets
	}
	extensions := map[string]int{}
	origins := map[string]int{}
	for i := range index.records {
		extensions[index.extensionOf(&index.records[i])]++
		origins[originName(index.records[i].origin)]++
	}
	facets.Extensions = sortedFacets(extensions)
	facets.Origins = sortedFacets(origins)
	return facets
}

// sortedFacets 把统计表按数量降序、同数量按名称升序排好
// sortedFacets orders a count table by descending count and ascending value within a tie
func sortedFacets(counts map[string]int) []FacetCount {
	result := make([]FacetCount, 0, len(counts))
	for value, count := range counts {
		result = append(result, FacetCount{Value: value, Count: count})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Count != result[right].Count {
			return result[left].Count > result[right].Count
		}
		return result[left].Value < result[right].Value
	})
	return result
}
