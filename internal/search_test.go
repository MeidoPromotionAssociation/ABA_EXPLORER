package internal

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestResourceNameFromLoadName 覆盖 m_Container 加载名还原成 KCES 资源名的全部实测形态
// 用例取自 testdata 中真实出现过的后缀分布，既要剥掉 Unity 追加的资产扩展名，
// 也不能削掉本身就是完整资产名的 .anim/.wav/.csv
// TestResourceNameFromLoadName covers every measured shape of recovering a KCES resource name from an m_Container load name
// The cases come from the suffix distribution actually observed in testdata: the Unity asset extension must be stripped,
// while names that are already complete such as .anim, .wav, and .csv must keep their tail
func TestResourceNameFromLoadName(t *testing.T) {
	cases := []struct {
		loadName string
		want     string
	}{
		// KCES 名字 + Unity 资产扩展名，剥掉末段 / A KCES name plus a Unity asset extension, with the tail stripped
		{"assets/gamedata/parts/parts/parts/dress/crc_dress013_stkg_i_.tex.png", "crc_dress013_stkg_i_.tex"},
		{"assets/gamedata/parts/cm3d2_eyes/cm3d2_eyes.menuassets.bytes", "cm3d2_eyes.menuassets"},
		{"assets/gamedata/parts/parts/parts.materialassets.bytes", "parts.materialassets"},
		{"assets/gamedata/parts/crc_hair_ponyr6.mmesh.asset", "crc_hair_ponyr6.mmesh"},
		{"assets/gamedata/parts/cm3d2_megane002.mmesh.crmesh", "cm3d2_megane002.mmesh"},
		{"assets/gamedata/parts/cm3d2_eyes_icon.partsatlas.spriteatlas", "cm3d2_eyes_icon.partsatlas"},
		{"assets/gamedata/parts/crc_edit_pose_dg18w_001_f.anm.asset", "crc_edit_pose_dg18w_001_f.anm"},

		// 本身就是完整 Unity 资产名，下面没有第二个扩展名，保持原样
		// Already a complete Unity asset name with no second extension underneath, so it is kept whole
		{"assets/ui/kces2dropdownoptionanimation_close.anim", "kces2dropdownoptionanimation_close.anim"},
		{"assets/bg/empireclub_rotary_night.prefab", "empireclub_rotary_night.prefab"},
		{"assets/sound/bgm001.wav", "bgm001.wav"},
		{"assets/gamedata/partsmeta/parts_mpn_restrict_check.csv", "parts_mpn_restrict_check.csv"},
		{"assets/settings/post processing profile.asset", "post processing profile.asset"},
		{"assets/ui/imagetogglebuttonanimationcontroller.controller", "imagetogglebuttonanimationcontroller.controller"},

		// 反斜杠分隔与无目录前缀 / Backslash separators and names without a directory prefix
		{`assets\gamedata\parts\crc_acc.tex.png`, "crc_acc.tex"},
		{"parts.menuassets.bytes", "parts.menuassets"},
		{"", ""},
	}

	for _, item := range cases {
		if got := resourceNameFromLoadName(item.loadName); got != item.want {
			t.Errorf("resourceNameFromLoadName(%q) = %q, want %q", item.loadName, got, item.want)
		}
	}
}

// buildSampleIndex 在真实样本目录上建一次深度索引，供后面的用例共用
// buildSampleIndex builds one deep index over the real sample directory shared by the cases below
func buildSampleIndex(t *testing.T) (*SearchService, IndexStats) {
	t.Helper()
	isolatedConfigDir(t)
	dir := testDataDir(t)
	service := NewSearchService()
	stats, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true})
	if err != nil {
		t.Fatalf("BuildIndex(%q): %v", dir, err)
	}
	if !stats.Ready {
		t.Fatal("index is not ready after a successful build")
	}
	return service, stats
}

// TestBuildIndexOnSamples 检查索引在真实样本上的规模与自洽性
// TestBuildIndexOnSamples checks the scale and self-consistency of an index over real samples
func TestBuildIndexOnSamples(t *testing.T) {
	service, stats := buildSampleIndex(t)

	if stats.Catalogs == 0 {
		t.Error("no .ct file was indexed")
	}
	if stats.Containers == 0 {
		t.Error("no container was indexed")
	}
	if stats.Names < 10000 {
		t.Errorf("indexed %d names, want at least 10000 from the sample set", stats.Names)
	}
	// 深度索引的意义就是拿到 catalog 里根本没有的容器内部名，一条都没有说明这一层没跑起来
	// The whole point of deep indexing is recovering inner names a catalog never lists, so zero of them means the layer never ran
	if stats.InnerNames == 0 {
		t.Error("deep indexing produced no inner container names")
	}
	if stats.Building {
		t.Error("index reports building after the build returned")
	}

	status := service.IndexStatus()
	if status.Names != stats.Names || status.Root != stats.Root {
		t.Errorf("IndexStatus = %+v, want it to match the build stats %+v", status, stats)
	}
}

// TestSearchFindsMenuInsideContainer 验证核心场景：只知道一个 .menu 名字，要问出它在哪个 aba 里
// .menu 只存在于 .menuassets 内部，catalog 不会列出它，所以这条路径必须走深度索引才能通
// TestSearchFindsMenuInsideContainer covers the core scenario: knowing only a .menu name and asking which .aba holds it
// A .menu exists only inside .menuassets and is never listed by a catalog, so this path only works through deep indexing
func TestSearchFindsMenuInsideContainer(t *testing.T) {
	service, _ := buildSampleIndex(t)

	result := service.Search(SearchQuery{Text: "crc_dress013_stkg_color_a_i_.menu"})
	if len(result.Hits) == 0 {
		t.Fatal("searching a known .menu name returned no hit")
	}

	hit := result.Hits[0]
	if !strings.EqualFold(hit.Name, "crc_dress013_stkg_color_a_i_.menu") {
		t.Errorf("top hit name = %q, want the exact match to rank first", hit.Name)
	}
	if hit.Origin != OriginMenu {
		t.Errorf("origin = %q, want %q", hit.Origin, OriginMenu)
	}
	if hit.ContainerName == "" {
		t.Error("hit does not say which container holds the name")
	}
	// 命中必须给出可直接打开的绝对路径，否则"它在哪个 aba 里"这个问题只答了一半
	// A hit must carry an openable absolute path, otherwise the question of which .aba holds it is only half answered
	if hit.ContainerPath == "" {
		t.Error("hit has no container path")
	} else if info, err := os.Stat(hit.ContainerPath); err != nil || !info.Mode().IsRegular() {
		t.Errorf("container path %q does not point at a readable file: %v", hit.ContainerPath, err)
	}
	if !strings.EqualFold(filepath.Ext(hit.Owner), ".menuassets") {
		t.Errorf("owner = %q, want the .menuassets entry holding the menu", hit.Owner)
	}
	if hit.Detail == "" {
		t.Error("menu hit carries no in-game display name")
	}
}

// TestSearchFindsCatalogResource 验证 catalog 层的名字同样能定位到容器与配套内容表
// TestSearchFindsCatalogResource checks that a catalog-layer name also resolves to its container and paired content table
func TestSearchFindsCatalogResource(t *testing.T) {
	service, _ := buildSampleIndex(t)

	result := service.Search(SearchQuery{Text: "crc_dress013_stkg.mmesh"})
	if len(result.Hits) == 0 {
		t.Fatal("searching a known catalog resource returned no hit")
	}
	// 同名资源出现在多个 aba 里是常态，命中必须逐个列出而不是只留一条
	// The same name living in several .aba files is normal, and every one of them must be listed rather than collapsed
	containers := map[string]bool{}
	for _, hit := range result.Hits {
		if strings.EqualFold(hit.Name, "crc_dress013_stkg.mmesh") {
			containers[strings.ToLower(hit.ContainerName)] = true
		}
	}
	if len(containers) < 2 {
		t.Errorf("found the name in %d containers, want the duplicates across .aba files to be preserved", len(containers))
	}

	withCatalog := 0
	for _, hit := range result.Hits {
		if hit.CatalogPath != "" {
			withCatalog++
			if info, err := os.Stat(hit.CatalogPath); err != nil || !info.Mode().IsRegular() {
				t.Errorf("catalog path %q does not point at a readable file: %v", hit.CatalogPath, err)
			}
		}
	}
	if withCatalog == 0 {
		t.Error("no hit carries the paired .ct path")
	}
}

// TestSearchDeduplicatesCatalogAndContainer 检查同一容器里 catalog 与 m_Container 描述的同一资源只留一条
// 两层扫的是同一批资源，不去重的话每个名字都会出现两遍
// TestSearchDeduplicatesCatalogAndContainer checks that one resource described by both a catalog and an m_Container yields a single record
// The two layers scan the same resources, so without deduplication every name would appear twice
func TestSearchDeduplicatesCatalogAndContainer(t *testing.T) {
	service, _ := buildSampleIndex(t)

	result := service.Search(SearchQuery{Text: "crc_dress013_stkg_i_.tex", Limit: maxSearchLimit})
	perContainer := map[string]int{}
	for _, hit := range result.Hits {
		if !strings.EqualFold(hit.Name, "crc_dress013_stkg_i_.tex") {
			continue
		}
		perContainer[strings.ToLower(hit.ContainerPath)]++
	}
	if len(perContainer) == 0 {
		t.Fatal("searching a known texture returned no hit")
	}
	for container, count := range perContainer {
		if count != 1 {
			t.Errorf("container %q yielded %d records for one resource, want exactly 1", container, count)
		}
	}
}

// testEntry 是构造合成索引用的一条可读记录，让用例不必关心索引的内部存储布局
// testEntry is one readable record for building a synthetic index so cases need not know the index storage layout
type testEntry struct {
	name   string
	detail string
	owner  string
	origin string
}

// newTestIndex 用一个容器和若干条目拼出一份可查询的索引
// newTestIndex assembles a queryable index from one container and a set of entries
func newTestIndex(containerPath string, entries []testEntry) *nameIndex {
	index := &nameIndex{
		sources: []indexSource{{containerPath: containerPath, containerName: filepath.Base(containerPath)}},
		stats:   IndexStats{Ready: true},
	}
	extensionIDs := map[string]int32{}
	ownerIDs := map[string]int32{}
	intern := func(table *[]string, ids map[string]int32, value string) int32 {
		if id, ok := ids[value]; ok {
			return id
		}
		*table = append(*table, value)
		id := int32(len(*table) - 1)
		ids[value] = id
		return id
	}
	for _, entry := range entries {
		owner := noOwner
		if entry.owner != "" {
			owner = intern(&index.owners, ownerIDs, entry.owner)
		}
		index.records = append(index.records, indexRecord{
			name:        entry.name,
			lower:       strings.ToLower(entry.name),
			detail:      entry.detail,
			detailLower: strings.ToLower(entry.detail),
			extension:   intern(&index.extensions, extensionIDs, strings.ToLower(filepath.Ext(entry.name))),
			owner:       owner,
			origin:      originID(entry.origin),
		})
	}
	index.stats.Names = len(index.records)
	return index
}

// TestSearchRanksExactMatchFirst 检查精确匹配排在前缀与子串命中之前
// TestSearchRanksExactMatchFirst checks that an exact match sorts ahead of prefix and substring hits
func TestSearchRanksExactMatchFirst(t *testing.T) {
	service := NewSearchService()
	service.store(newTestIndex(`D:\game\parts.aba`, []testEntry{
		{name: "other_crc_dress.menu", origin: OriginMenu},
		{name: "crc_dress.menu.bak", origin: OriginMenu},
		{name: "crc_dress.menu", origin: OriginMenu},
	}))

	result := service.Search(SearchQuery{Text: "crc_dress.menu"})
	if len(result.Hits) != 3 {
		t.Fatalf("got %d hits, want all 3 records to match", len(result.Hits))
	}
	if result.Hits[0].Name != "crc_dress.menu" {
		t.Errorf("first hit = %q, want the exact match", result.Hits[0].Name)
	}
	if result.Hits[1].Name != "crc_dress.menu.bak" {
		t.Errorf("second hit = %q, want the prefix match", result.Hits[1].Name)
	}
}

// TestSearchMatchesDisplayName 检查按游戏内显示名也能搜到，用户往往只记得这个
// TestSearchMatchesDisplayName checks that the in-game display name is searchable too, since it is often all a user remembers
func TestSearchMatchesDisplayName(t *testing.T) {
	service := NewSearchService()
	service.store(newTestIndex(`D:\game\parts.aba`, []testEntry{{
		name:   "crc_dress013_stkg_color_a_i_.menu",
		detail: "レストランコックソックス",
		owner:  "parts.menuassets",
		origin: OriginMenu,
	}}))

	result := service.Search(SearchQuery{Text: "コックソックス"})
	if len(result.Hits) != 1 {
		t.Fatalf("got %d hits searching a display name, want 1", len(result.Hits))
	}
	if result.Hits[0].ContainerName != "parts.aba" {
		t.Errorf("container = %q, want parts.aba", result.Hits[0].ContainerName)
	}
	// 下标存储必须能还原回可读字段，否则前端拿到的是空白
	// The index-based storage must decode back into readable fields, otherwise the frontend receives blanks
	if result.Hits[0].Owner != "parts.menuassets" {
		t.Errorf("owner = %q, want parts.menuassets", result.Hits[0].Owner)
	}
	if result.Hits[0].Extension != ".menu" {
		t.Errorf("extension = %q, want .menu", result.Hits[0].Extension)
	}
	if result.Hits[0].Origin != OriginMenu {
		t.Errorf("origin = %q, want %q", result.Hits[0].Origin, OriginMenu)
	}
}

// TestSearchKeepsExactMatchInLargeIndex 检查大索引下精确匹配不会被海量子串命中挤掉
// 结果集有上限，如果收集时先到先得，排在几十万条子串命中之后的精确匹配就永远看不到了
// TestSearchKeepsExactMatchInLargeIndex checks that an exact match survives a flood of substring hits in a large index
// The result set is capped, and with first-come-first-served collection an exact match sitting behind
// hundreds of thousands of substring hits would never surface
func TestSearchKeepsExactMatchInLargeIndex(t *testing.T) {
	const size = 300000
	entries := make([]testEntry, 0, size+1)
	for i := range size {
		entries = append(entries, testEntry{
			name:   "crc_dress_filler_" + strconv.Itoa(i) + "_stkg.tex",
			origin: OriginCatalog,
		})
	}
	// 精确匹配放在最后一条，任何"收满就停"的实现都会漏掉它
	// The exact match is the very last record, which any collect-until-full implementation would miss
	entries = append(entries, testEntry{name: "stkg", origin: OriginMenu})

	service := NewSearchService()
	service.store(newTestIndex(`D:\game\parts.aba`, entries))

	start := time.Now()
	result := service.Search(SearchQuery{Text: "stkg", Limit: 500})
	elapsed := time.Since(start)

	if len(result.Hits) == 0 {
		t.Fatal("searching a common substring returned nothing")
	}
	if result.Hits[0].Name != "stkg" {
		t.Errorf("first hit = %q, want the exact match to outrank %d substring hits", result.Hits[0].Name, size)
	}
	if result.Total != size+1 {
		t.Errorf("Total = %d, want every one of the %d matches counted", result.Total, size+1)
	}
	if !result.Truncated {
		t.Error("a capped result over a 300k match set is not flagged as truncated")
	}
	// 逐键输入时每次按键都会查一次，几百毫秒的查询会让搜索框卡住
	// Every keystroke runs a query, and a several-hundred-millisecond one makes the search box stutter
	if elapsed > 500*time.Millisecond {
		t.Errorf("query over %d records took %v, too slow for per-keystroke search", size, elapsed)
	}
}

// isolatedConfigDir 把用户配置目录指向一个临时目录，避免测试写进真实的索引缓存与设置文件
// os.UserConfigDir 在 Windows 上取 %AppData%，在 Linux 上取 $XDG_CONFIG_HOME，两个都设才跨平台
// isolatedConfigDir points the user config directory at a temporary directory so tests never touch the real
// index cache or settings file
// os.UserConfigDir reads %AppData% on Windows and $XDG_CONFIG_HOME on Linux, so both must be set to cover
// the platforms this project targets
func isolatedConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
}

// sampleCopy 把样本目录里的若干文件复制到一个可写的临时目录
// 增量更新的用例要改文件的修改时间，绝不能动到真实样本
// sampleCopy copies a few files from the sample directory into a writable temporary directory
// Incremental update cases have to touch modification times and must never disturb the real samples
func sampleCopy(t *testing.T, names ...string) string {
	t.Helper()
	source := testDataDir(t)
	target := t.TempDir()
	copied := 0
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(target, name), data, 0644); err != nil {
			t.Fatalf("copy sample %q: %v", name, err)
		}
		copied++
	}
	if copied == 0 {
		t.Skip("no sample file could be copied")
	}
	return target
}

// TestIndexCacheRoundTrip 检查磁盘缓存写出再读回后索引内容完全一致
// 缓存要是丢字段或错位，搜索会安静地少结果，比直接报错更难发现
// TestIndexCacheRoundTrip checks that an index is identical after being written to disk and read back
// A cache that drops or misaligns a field makes searches quietly return less, which is harder to notice
// than an outright error
func TestIndexCacheRoundTrip(t *testing.T) {
	isolatedConfigDir(t)
	dir := testDataDir(t)

	service := NewSearchService()
	built, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true})
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	before := service.Search(SearchQuery{Text: "stkg", Limit: maxSearchLimit})

	// 换一个干净的服务，只能从磁盘缓存恢复
	// A fresh service can only recover from the on-disk cache
	restored := NewSearchService()
	loaded, err := restored.LoadCachedIndex(IndexOptions{Root: dir, Deep: true})
	if err != nil {
		t.Fatalf("LoadCachedIndex: %v", err)
	}
	if !loaded.Ready {
		t.Fatal("a freshly written cache did not load")
	}
	if !loaded.FromCache {
		t.Error("a cache load is not flagged as coming from the cache")
	}
	if loaded.Names != built.Names || loaded.InnerNames != built.InnerNames {
		t.Errorf("cache holds %d names (%d inner), want %d (%d)",
			loaded.Names, loaded.InnerNames, built.Names, built.InnerNames)
	}
	if loaded.Containers != built.Containers || loaded.Catalogs != built.Catalogs {
		t.Errorf("cache holds %d containers and %d catalogs, want %d and %d",
			loaded.Containers, loaded.Catalogs, built.Containers, built.Catalogs)
	}

	after := restored.Search(SearchQuery{Text: "stkg", Limit: maxSearchLimit})
	if after.Total != before.Total || len(after.Hits) != len(before.Hits) {
		t.Fatalf("restored index returns %d/%d hits, want %d/%d",
			len(after.Hits), after.Total, len(before.Hits), before.Total)
	}
	for i := range before.Hits {
		if before.Hits[i] != after.Hits[i] {
			t.Fatalf("hit %d differs after a cache round trip:\n  before %+v\n  after  %+v",
				i, before.Hits[i], after.Hits[i])
		}
	}
}

// TestBuildIndexReusesCache 检查文件没变时第二次构建整份沿用缓存
// TestBuildIndexReusesCache checks that a second build reuses the whole cache when no file changed
func TestBuildIndexReusesCache(t *testing.T) {
	isolatedConfigDir(t)
	dir := testDataDir(t)

	service := NewSearchService()
	first, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true})
	if err != nil {
		t.Fatalf("first BuildIndex: %v", err)
	}
	second, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true})
	if err != nil {
		t.Fatalf("second BuildIndex: %v", err)
	}

	if second.Reused == 0 {
		t.Error("the second build reused nothing from the cache")
	}
	if second.Names != first.Names {
		t.Errorf("reusing the cache produced %d names, want the original %d", second.Names, first.Names)
	}
	if second.Containers != first.Containers || second.Catalogs != first.Catalogs {
		t.Errorf("reusing the cache produced %d containers and %d catalogs, want %d and %d",
			second.Containers, second.Catalogs, first.Containers, first.Catalogs)
	}
}

// TestBuildIndexRescansChangedFile 检查文件变了以后那一份被重扫而不是继续用旧缓存
// 缓存最危险的失败不是慢，而是文件已经改了却还在报旧内容
// TestBuildIndexRescansChangedFile checks that a changed file is rescanned rather than served from the cache
// The dangerous cache failure is not slowness but reporting stale content after a file has changed
func TestBuildIndexRescansChangedFile(t *testing.T) {
	isolatedConfigDir(t)
	dir := sampleCopy(t, "cm3d2_eyes.aba", "cm3d2_eyes.ct", "nt008_chignon.aba", "nt008_chignon.ct")

	service := NewSearchService()
	first, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true})
	if err != nil {
		t.Fatalf("first BuildIndex: %v", err)
	}
	if first.Names == 0 {
		t.Fatal("the copied samples produced an empty index")
	}
	// 容器内部名只能从容器里读出来，是判断"这一份到底重扫了没有"最直接的证据
	// Inner container names can only come from the container itself, making them the most direct evidence
	// of whether that source was actually rescanned
	innerBefore := innerNamesOfContainer(service, "cm3d2_eyes.aba")
	if innerBefore == 0 {
		t.Fatal("the sample container yielded no inner names to begin with")
	}

	// 把一个容器换成完全读不出的内容，重扫过就一定会丢掉它的容器内部名
	// Replacing one container with unreadable content guarantees a rescan drops its inner names
	target := filepath.Join(dir, "cm3d2_eyes.aba")
	if err := os.WriteFile(target, []byte("not a UnityFS container at all"), 0644); err != nil {
		t.Fatalf("corrupt the sample: %v", err)
	}

	second, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true})
	if err != nil {
		t.Fatalf("second BuildIndex: %v", err)
	}
	if second.Names >= first.Names {
		t.Errorf("index still holds %d names after a container was destroyed, want fewer than %d",
			second.Names, first.Names)
	}
	if innerAfter := innerNamesOfContainer(service, "cm3d2_eyes.aba"); innerAfter != 0 {
		t.Errorf("the destroyed container still contributes %d inner names, want 0", innerAfter)
	}
	// 配套 .ct 没有损坏，它声明的资源名理应还在：那是 catalog 的说法，不因容器读不出而失效
	// The paired .ct is intact, so the resource names it declares must remain: that is the catalog's claim
	// and it does not become false because the container cannot be read
	if result := service.Search(SearchQuery{Text: "cm3d2_eyes.menuassets", Limit: 10}); result.Total == 0 {
		t.Error("names declared by the intact catalog disappeared along with the container")
	}
	// 另一个来源没动过，必须继续沿用而不是陪着一起重扫
	// The untouched source must stay reused rather than be rescanned alongside
	if second.Reused == 0 {
		t.Error("the untouched source was not reused")
	}
}

// innerNamesOfContainer 数一个容器贡献了多少条容器内部名（.menu/.mate/.pmat）
// innerNamesOfContainer counts how many inner container names (.menu, .mate, .pmat) one container contributes
func innerNamesOfContainer(service *SearchService, containerName string) int {
	result := service.Search(SearchQuery{
		Origins: []string{OriginMenu, OriginMaterial, OriginPmat},
		Limit:   maxSearchLimit,
	})
	count := 0
	for _, hit := range result.Hits {
		if strings.EqualFold(hit.ContainerName, containerName) {
			count++
		}
	}
	return count
}

// TestBuildIndexPicksUpNewFile 检查新增文件会被索引进来
// 装一个 MOD 就是往目录里放文件，这是增量更新最常见的场景
// TestBuildIndexPicksUpNewFile checks that a newly added file gets indexed
// Installing a MOD means dropping files into the directory, the most common incremental case
func TestBuildIndexPicksUpNewFile(t *testing.T) {
	isolatedConfigDir(t)
	source := testDataDir(t)
	dir := sampleCopy(t, "cm3d2_eyes.aba", "cm3d2_eyes.ct")

	service := NewSearchService()
	first, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true})
	if err != nil {
		t.Fatalf("first BuildIndex: %v", err)
	}

	added, err := os.ReadFile(filepath.Join(source, "nt008_chignon.aba"))
	if err != nil {
		t.Skip("the sample to add is unavailable")
	}
	if err := os.WriteFile(filepath.Join(dir, "nt008_chignon.aba"), added, 0644); err != nil {
		t.Fatalf("add a sample: %v", err)
	}

	second, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true})
	if err != nil {
		t.Fatalf("second BuildIndex: %v", err)
	}
	if second.Names <= first.Names {
		t.Errorf("index holds %d names after adding a container, want more than %d", second.Names, first.Names)
	}
	if second.Reused == 0 {
		t.Error("the untouched source was not reused while adding a file")
	}
	if result := service.Search(SearchQuery{Text: "nt008_chignon", Limit: 10}); result.Total == 0 {
		t.Error("names from the newly added container are not searchable")
	}
}

// TestBuildIndexRefreshIgnoresCache 检查强制刷新会绕过缓存重扫
// TestBuildIndexRefreshIgnoresCache checks that a forced refresh bypasses the cache and rescans
func TestBuildIndexRefreshIgnoresCache(t *testing.T) {
	isolatedConfigDir(t)
	dir := testDataDir(t)

	service := NewSearchService()
	if _, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true}); err != nil {
		t.Fatalf("first BuildIndex: %v", err)
	}
	refreshed, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true, Refresh: true})
	if err != nil {
		t.Fatalf("refreshing BuildIndex: %v", err)
	}
	if refreshed.Reused != 0 {
		t.Errorf("a forced refresh still reused %d sources", refreshed.Reused)
	}
	if refreshed.Names == 0 {
		t.Error("a forced refresh produced an empty index")
	}
}

// TestCacheIsNotSharedAcrossDepth 检查浅索引的缓存不会被当成深索引用
// 浅索引没有容器内部名，冒名顶替会让 .menu 查不到却看不出任何异常
// TestCacheIsNotSharedAcrossDepth checks that a shallow cache is never served as a deep index
// A shallow index carries no inner container names, so passing it off as deep makes .menu lookups come up
// empty with nothing to indicate why
func TestCacheIsNotSharedAcrossDepth(t *testing.T) {
	isolatedConfigDir(t)
	dir := testDataDir(t)

	service := NewSearchService()
	shallow, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: false})
	if err != nil {
		t.Fatalf("shallow BuildIndex: %v", err)
	}
	if shallow.InnerNames != 0 {
		t.Errorf("a shallow index carries %d inner names, want none", shallow.InnerNames)
	}

	fresh := NewSearchService()
	if loaded, err := fresh.LoadCachedIndex(IndexOptions{Root: dir, Deep: true}); err != nil || loaded.Ready {
		t.Errorf("a shallow cache was loaded as a deep index: %+v (err %v)", loaded, err)
	}

	deep, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true})
	if err != nil {
		t.Fatalf("deep BuildIndex: %v", err)
	}
	if deep.Reused != 0 {
		t.Errorf("a deep build reused %d sources from a shallow cache", deep.Reused)
	}
	if deep.InnerNames == 0 {
		t.Error("the deep build produced no inner container names")
	}
}

// TestLoadCachedIndexRejectsStaleCache 检查任何一个文件变了就整份缓存不予装载
// 半份缓存看起来一切正常却少结果，宁可让用户重建
// TestLoadCachedIndexRejectsStaleCache checks that one changed file makes the whole cache refuse to load
// Half a cache looks perfectly normal while returning less, so a rebuild is the safer answer
func TestLoadCachedIndexRejectsStaleCache(t *testing.T) {
	isolatedConfigDir(t)
	dir := sampleCopy(t, "cm3d2_eyes.aba", "cm3d2_eyes.ct")

	service := NewSearchService()
	if _, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true}); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	fresh := NewSearchService()
	if loaded, err := fresh.LoadCachedIndex(IndexOptions{Root: dir, Deep: true}); err != nil || !loaded.Ready {
		t.Fatalf("an untouched cache did not load: %+v (err %v)", loaded, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "cm3d2_eyes.aba"), []byte("changed"), 0644); err != nil {
		t.Fatalf("modify the sample: %v", err)
	}

	stale := NewSearchService()
	loaded, err := stale.LoadCachedIndex(IndexOptions{Root: dir, Deep: true})
	if err != nil {
		t.Fatalf("LoadCachedIndex over a stale cache: %v", err)
	}
	if loaded.Ready {
		t.Error("a stale cache was loaded as ready")
	}
	if result := stale.Search(SearchQuery{Text: ".menu"}); len(result.Hits) != 0 {
		t.Error("a stale cache still answers queries")
	}
}

// TestLoadCachedIndexSurvivesCorruptCache 检查缓存损坏时退回未就绪而不是崩溃
// 缓存文件可能被写坏、被杀毒软件截断或被手工乱改，解码器不能因此让应用挂掉
// TestLoadCachedIndexSurvivesCorruptCache checks that a corrupt cache falls back to not-ready instead of crashing
// A cache file can be half-written, truncated by antivirus, or hand-edited, and the decoder must not take
// the application down with it
func TestLoadCachedIndexSurvivesCorruptCache(t *testing.T) {
	isolatedConfigDir(t)
	dir := sampleCopy(t, "cm3d2_eyes.aba", "cm3d2_eyes.ct")

	service := NewSearchService()
	if _, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true}); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	cachePath, err := indexCachePath(dir)
	if err != nil {
		t.Fatalf("resolve the cache path: %v", err)
	}

	original, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read the cache: %v", err)
	}
	corruptions := map[string][]byte{
		"empty":       {},
		"wrong magic": []byte("NOTIDX\x01\x00"),
		"truncated":   original[:len(original)/2],
		"garbage tail": append(append([]byte{}, original...),
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff),
	}
	for name, content := range corruptions {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(cachePath, content, 0644); err != nil {
				t.Fatalf("write the corrupt cache: %v", err)
			}
			fresh := NewSearchService()
			loaded, err := fresh.LoadCachedIndex(IndexOptions{Root: dir, Deep: true})
			if err != nil {
				t.Fatalf("a corrupt cache returned an error instead of not-ready: %v", err)
			}
			if loaded.Ready {
				t.Error("a corrupt cache was loaded as ready")
			}
			// 损坏的缓存不能挡住重建
			// A corrupt cache must not block a rebuild
			rebuilt, err := fresh.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true})
			if err != nil {
				t.Fatalf("BuildIndex over a corrupt cache: %v", err)
			}
			if rebuilt.Names == 0 {
				t.Error("the rebuild over a corrupt cache produced an empty index")
			}
		})
	}
}

// TestClearCacheRemovesCacheFile 检查清除缓存后不再能从磁盘恢复
// TestClearCacheRemovesCacheFile checks that nothing can be restored from disk after the cache is cleared
func TestClearCacheRemovesCacheFile(t *testing.T) {
	isolatedConfigDir(t)
	dir := sampleCopy(t, "cm3d2_eyes.aba", "cm3d2_eyes.ct")

	service := NewSearchService()
	if _, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true}); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if err := service.ClearCache(dir); err != nil {
		t.Fatalf("ClearCache: %v", err)
	}
	// 缓存本来就没有时再删一次也不该报错
	// Clearing an already absent cache must not error either
	if err := service.ClearCache(dir); err != nil {
		t.Fatalf("ClearCache on an absent cache: %v", err)
	}

	fresh := NewSearchService()
	if loaded, err := fresh.LoadCachedIndex(IndexOptions{Root: dir, Deep: true}); err != nil || loaded.Ready {
		t.Errorf("an index loaded after the cache was cleared: %+v (err %v)", loaded, err)
	}
}

// TestSearchFiltersAndLimit 检查扩展名过滤、来源层过滤与条数上限
// TestSearchFiltersAndLimit checks extension filtering, origin filtering, and the hit limit
func TestSearchFiltersAndLimit(t *testing.T) {
	service, _ := buildSampleIndex(t)

	menus := service.Search(SearchQuery{Extensions: []string{".menu"}, Limit: maxSearchLimit})
	if menus.Total == 0 {
		t.Fatal("filtering by .menu returned nothing")
	}
	for _, hit := range menus.Hits {
		if hit.Extension != ".menu" {
			t.Fatalf("extension filter leaked %q", hit.Extension)
		}
	}

	catalogOnly := service.Search(SearchQuery{Origins: []string{OriginCatalog}, Limit: maxSearchLimit})
	for _, hit := range catalogOnly.Hits {
		if hit.Origin != OriginCatalog {
			t.Fatalf("origin filter leaked %q", hit.Origin)
		}
	}

	limited := service.Search(SearchQuery{Extensions: []string{".menu"}, Limit: 5})
	if len(limited.Hits) != 5 {
		t.Errorf("got %d hits with limit 5", len(limited.Hits))
	}
	// Total 必须报未截断前的真实命中数，否则用户看到的 5 会被误认为全部
	// Total must report the real match count before truncation, otherwise the 5 on screen reads as the whole answer
	if limited.Total != menus.Total {
		t.Errorf("truncated Total = %d, want the untruncated %d", limited.Total, menus.Total)
	}
	if !limited.Truncated {
		t.Error("truncated result is not flagged as truncated")
	}
}

// TestFacetsCoverIndexedRecords 检查过滤选项的计数与索引总数一致
// TestFacetsCoverIndexedRecords checks that the filter counts add up to the indexed total
func TestFacetsCoverIndexedRecords(t *testing.T) {
	service, stats := buildSampleIndex(t)

	facets := service.Facets()
	if len(facets.Extensions) == 0 || len(facets.Origins) == 0 {
		t.Fatal("facets are empty for a populated index")
	}
	total := 0
	for _, facet := range facets.Origins {
		total += facet.Count
	}
	if total != stats.Names {
		t.Errorf("origin facet counts sum to %d, want the index size %d", total, stats.Names)
	}
}

// TestSearchWithoutIndexIsEmpty 检查没有索引时查询返回空结果而不是崩溃
// TestSearchWithoutIndexIsEmpty checks that querying without an index returns an empty result instead of panicking
func TestSearchWithoutIndexIsEmpty(t *testing.T) {
	service := NewSearchService()
	result := service.Search(SearchQuery{Text: "anything"})
	if len(result.Hits) != 0 || result.Total != 0 {
		t.Errorf("got %+v, want an empty result", result)
	}
	if status := service.IndexStatus(); status.Ready {
		t.Error("status reports ready without an index")
	}
}

// TestBuildIndexRejectsBadRoot 检查根目录不存在或不是目录时报错而不是建出空索引
// TestBuildIndexRejectsBadRoot checks that a missing or non-directory root errors instead of producing an empty index
func TestBuildIndexRejectsBadRoot(t *testing.T) {
	isolatedConfigDir(t)
	service := NewSearchService()

	if _, err := service.BuildIndex(context.Background(), IndexOptions{Root: ""}); err == nil {
		t.Error("empty root was accepted")
	}
	if _, err := service.BuildIndex(context.Background(), IndexOptions{Root: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Error("missing root was accepted")
	}

	file := filepath.Join(t.TempDir(), "plain.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if _, err := service.BuildIndex(context.Background(), IndexOptions{Root: file}); err == nil {
		t.Error("a regular file was accepted as the index root")
	}
}

// TestBuildIndexOnEmptyDirectory 检查空目录建出一个可用但为空的索引
// TestBuildIndexOnEmptyDirectory checks that an empty directory yields a usable but empty index
func TestBuildIndexOnEmptyDirectory(t *testing.T) {
	isolatedConfigDir(t)
	service := NewSearchService()
	stats, err := service.BuildIndex(context.Background(), IndexOptions{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildIndex on an empty directory: %v", err)
	}
	if !stats.Ready || stats.Names != 0 {
		t.Errorf("stats = %+v, want a ready empty index", stats)
	}
}

// TestCancelIndexKeepsPreviousIndex 检查构建途中取消不会破坏已经建好的索引
// 取消落在扫描或解析中间，此时把半成品写进去会让后续查询给出残缺答案
// TestCancelIndexKeepsPreviousIndex checks that cancelling mid-build does not damage an already built index
// A cancellation lands mid-scan, and storing the half-finished result would make later queries answer with partial data
func TestCancelIndexKeepsPreviousIndex(t *testing.T) {
	service, stats := buildSampleIndex(t)

	// 在第一条进度事件上取消，保证取消确实落在构建进行中而不是开始前
	// Cancel on the first progress event so the cancellation genuinely lands mid-build rather than before it starts
	var once sync.Once
	service.SetEmitter(func(name string, data any) {
		once.Do(service.CancelIndex)
	})

	// 强制刷新，否则第一次构建写下的缓存会让这次无事可做，也就无从取消
	// A forced refresh is required, otherwise the cache written by the first build leaves nothing to do and
	// therefore nothing to cancel
	cancelled, err := service.BuildIndex(context.Background(), IndexOptions{Root: stats.Root, Deep: true, Refresh: true})
	if err != nil {
		t.Fatalf("cancelled BuildIndex returned an error: %v", err)
	}
	if !cancelled.Cancelled {
		t.Error("cancelled build is not flagged as cancelled")
	}
	if cancelled.Names != stats.Names {
		t.Errorf("index size changed to %d after cancellation, want the previous %d", cancelled.Names, stats.Names)
	}
	if result := service.Search(SearchQuery{Text: ".menu", Limit: 1}); len(result.Hits) == 0 {
		t.Error("the previous index stopped answering after a cancelled build")
	}
}

// TestCancelIndexBeforeBuildDoesNotLeak 检查空闲时的取消不会波及下一次构建
// 前端很容易在没有构建时误触取消，那次点击不该让紧接着的构建直接失效
// TestCancelIndexBeforeBuildDoesNotLeak checks that cancelling while idle does not bleed into the next build
// The frontend can easily fire a cancel with no build running, and that click must not void the build that follows
func TestCancelIndexBeforeBuildDoesNotLeak(t *testing.T) {
	isolatedConfigDir(t)
	dir := testDataDir(t)
	service := NewSearchService()
	service.CancelIndex()

	stats, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true})
	if err != nil {
		t.Fatalf("BuildIndex after an idle cancel: %v", err)
	}
	if stats.Cancelled {
		t.Error("build was flagged as cancelled by an idle CancelIndex call")
	}
	if stats.Names == 0 {
		t.Error("build after an idle cancel produced an empty index")
	}
}

// TestClearIndexDropsResults 检查清空索引后查询不再返回旧结果
// TestClearIndexDropsResults checks that queries stop returning stale results after the index is cleared
func TestClearIndexDropsResults(t *testing.T) {
	service, _ := buildSampleIndex(t)
	service.ClearIndex()

	if result := service.Search(SearchQuery{Text: ".menu"}); len(result.Hits) != 0 {
		t.Errorf("got %d hits after clearing the index", len(result.Hits))
	}
	if service.IndexStatus().Ready {
		t.Error("status reports ready after the index was cleared")
	}
}

// TestBuildIndexEmitsProgress 检查构建过程会推送进度并以结束事件收尾
// TestBuildIndexEmitsProgress checks that a build pushes progress and finishes with a completion event
func TestBuildIndexEmitsProgress(t *testing.T) {
	isolatedConfigDir(t)
	dir := testDataDir(t)
	service := NewSearchService()

	var updates []IndexProgress
	// 进度事件来自多个 worker goroutine，收集时要加锁
	// Progress events arrive from several worker goroutines, so collecting them needs a lock
	var mu sync.Mutex
	service.SetEmitter(func(name string, data any) {
		if name != IndexProgressEvent {
			t.Errorf("emitted event %q, want %q", name, IndexProgressEvent)
			return
		}
		progress, ok := data.(IndexProgress)
		if !ok {
			t.Errorf("emitted payload %T, want IndexProgress", data)
			return
		}
		mu.Lock()
		updates = append(updates, progress)
		mu.Unlock()
	})

	if _, err := service.BuildIndex(context.Background(), IndexOptions{Root: dir, Deep: true}); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(updates) == 0 {
		t.Fatal("no progress event was emitted")
	}
	last := updates[len(updates)-1]
	if !last.Finished {
		t.Error("the final progress event is not flagged as finished")
	}
	if last.Total == 0 || last.Done != last.Total {
		t.Errorf("final progress = %+v, want Done to reach Total", last)
	}
}
