package internal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/MeidoPromotionAssociation/MeidoSerialization/v2/serialization/KCES/ct"
	KCESService "github.com/MeidoPromotionAssociation/MeidoSerialization/v2/service/KCES"
)

// maxCtJsonBytes 是 .ct 与编辑 JSON 互转的输出上限 / maxCtJsonBytes caps the output of .ct to editing-JSON conversions
const maxCtJsonBytes int64 = 1 << 30

// CtExplorerService 提供 .ct 内容表的浏览、虚拟文件提取、从 ABA 生成与 JSON 互转
// CtExplorerService browses .ct content tables, extracts virtual files, generates from ABA, and converts to and from JSON
type CtExplorerService struct {
	ct *KCESService.CtService
}

// NewCtExplorerService 创建 CT 浏览服务 / NewCtExplorerService creates the CT browsing service
func NewCtExplorerService() *CtExplorerService {
	return &CtExplorerService{ct: &KCESService.CtService{}}
}

// CtOverview 是一个 .ct 内容表的完整结构快照
// CtOverview is a complete structural snapshot of one .ct content table
type CtOverview struct {
	Path         string                `json:"path"`         // .ct 文件路径 / .ct file path
	FileSize     int64                 `json:"fileSize"`     // 磁盘上的字节数 / On-disk byte count
	Version      int32                 `json:"version"`      // 根 VirtualDirectory 版本 / Root VirtualDirectory version
	Framing      string                `json:"framing"`      // 外层封装：legacy 或 extended / Outer framing, legacy or extended
	Directories  []CtDirectory         `json:"directories"`  // 子目录及其版本 / Child directories and their versions
	Files        []CtVirtualFile       `json:"files"`        // 虚拟文件表 / Virtual file table
	Catalog      *CtCatalog            `json:"catalog"`      // 解码后的 catalog，缺失或解析失败时为空 / Decoded catalog, empty when missing or unparsable
	CatalogError string                `json:"catalogError"` // catalog 解析失败原因 / Reason catalog parsing failed
	Extensions   []CtExtensionNameList `json:"extensions"`   // 按扩展名分组的名称表 / Name lists grouped by extension
}

// CtDirectory 是内容表中的一个子目录 / CtDirectory is one child directory in a content table
type CtDirectory struct {
	Path    string `json:"path"`    // 规范斜杠分隔路径 / Canonical slash-separated path
	Version int32  `json:"version"` // 子 VirtualDirectory 版本 / Child VirtualDirectory version
}

// CtVirtualFile 是内容表中的一个虚拟文件 / CtVirtualFile is one virtual file in a content table
type CtVirtualFile struct {
	Name     string `json:"name"`     // 虚拟文件名 / Virtual file name
	Position int64  `json:"position"` // 在 .ct 中的绝对字节偏移 / Absolute byte offset inside the .ct
	Size     int32  `json:"size"`     // 字节数 / Byte count
}

// CtCatalog 是 catalog 虚拟文件的解码结果
// Hash 与 CreateTime 走字符串通道，两者是 64 位值，超出 JS number 的安全整数范围
// CtCatalog is the decoded content of the catalog virtual file
// Hash and CreateTime travel as strings because both are 64-bit values beyond the JS safe-integer range
type CtCatalog struct {
	Kind              string          `json:"kind"`              // assetBundle 或 virtualAsset / assetBundle or virtualAsset
	Version           int32           `json:"version"`           // 序列化版本 / Serialization version
	CatalogType       int32           `json:"catalogType"`       // 资源分类标志位原值 / Raw resource-category flags
	CatalogTypeNames  []string        `json:"catalogTypeNames"`  // 标志位拆解出的分类名 / Category names decoded from the flags
	PackageType       int32           `json:"packageType"`       // 包类型原值 / Raw package type
	PackageTypeName   string          `json:"packageTypeName"`   // 包类型名 / Package type name
	Priority          int32           `json:"priority"`          // 排序优先级 / Ordering priority
	Name              string          `json:"name"`              // catalog 名称 / Catalog name
	SubName           string          `json:"subName"`           // catalog 子名称 / Catalog sub-name
	Hash              string          `json:"hash"`              // catalog 哈希的十进制字符串 / Decimal string of the catalog hash
	CreateTime        string          `json:"createTime"`        // 创建时间原值的十进制字符串 / Decimal string of the raw creation-time value
	IsEncrypted       bool            `json:"isEncrypted"`       // AssetBundle 是否加密 / Whether AssetBundles are encrypted
	ResourceFileNames []string        `json:"resourceFileNames"` // AssetBundle 资源文件名 / AssetBundle resource file names
	ExtensionList     []string        `json:"extensionList"`     // 扩展名虚拟文件列表 / Extension-name virtual-file list
	Items             []CtCatalogItem `json:"items"`             // catalog 条目 / Catalog items
}

// CtCatalogItem 是一条 catalog 资源条目，同时覆盖 AssetBundle 与 VirtualAsset 两种布局
// CtCatalogItem is one catalog resource entry covering both the AssetBundle and VirtualAsset layouts
type CtCatalogItem struct {
	Name          string `json:"name"`          // 资源名称 / Resource name
	Hash          string `json:"hash"`          // 资源哈希的十进制字符串 / Decimal string of the resource hash
	ResourceIndex int32  `json:"resourceIndex"` // ResourceFileNames 索引，VirtualAsset 恒为 -1 / Index into ResourceFileNames, always -1 for VirtualAsset
	AssetPath     string `json:"assetPath"`     // VirtualAsset 的 Unity 工程资源路径 / Unity project asset path for VirtualAsset
}

// CtExtensionNameList 是一个扩展名分组下的名称表 / CtExtensionNameList is the name list under one extension group
type CtExtensionNameList struct {
	Key       string            `json:"key"`       // ExtensionNameLists 的键 / Key in ExtensionNameLists
	Extension string            `json:"extension"` // 游戏字段 extention 的值 / Value of the game's extention field
	Data      []CtExtensionName `json:"data"`      // 名称与哈希条目 / Name and hash entries
}

// CtExtensionName 是扩展名表中的一条记录 / CtExtensionName is one record in an extension name list
type CtExtensionName struct {
	Name string `json:"name"` // 资源名称 / Resource name
	Hash string `json:"hash"` // 哈希的十进制字符串 / Decimal string of the hash
}

// catalogTypeNames 把 CatalogType 标志位拆解为分类名，未知位以 Bit_<n> 形式保留
// catalogTypeNames decodes CatalogType flags into category names and keeps unknown bits as Bit_<n>
func catalogTypeNames(value int32) []string {
	known := []struct {
		flag ct.CatalogType
		name string
	}{
		{ct.CatalogTypeUnknown, "Unknown"},
		{ct.CatalogTypeLanguage, "Language"},
		{ct.CatalogTypeProduct, "Product"},
		{ct.CatalogTypeMovie, "Movie"},
		{ct.CatalogTypeScript, "Script"},
		{ct.CatalogTypeSound, "Sound"},
		{ct.CatalogTypeVoice, "Voice"},
		{ct.CatalogTypeCsv, "Csv"},
		{ct.CatalogTypeSystem, "System"},
		{ct.CatalogTypeBg, "Bg"},
		{ct.CatalogTypeMotion, "Motion"},
		{ct.CatalogTypePartsMeta, "PartsMeta"},
		{ct.CatalogTypeParts, "Parts"},
	}
	names := make([]string, 0, 4)
	remaining := value
	for _, item := range known {
		flag := int32(item.flag)
		if value&flag != 0 {
			names = append(names, item.name)
			remaining &^= flag
		}
	}
	for bit := 0; bit < 32; bit++ {
		if remaining&(1<<bit) != 0 {
			names = append(names, fmt.Sprintf("Bit_%d", bit))
		}
	}
	return names
}

// packageTypeName 返回 CatalogPackageType 的显示名 / packageTypeName returns the display name of a CatalogPackageType
func packageTypeName(value int32) string {
	switch ct.CatalogPackageType(value) {
	case ct.PackageTypeBase:
		return "Base"
	case ct.PackageTypePlugin:
		return "Plugin"
	case ct.PackageTypePluginPatch:
		return "PluginPatch"
	case ct.PackageTypeBasePatch:
		return "BasePatch"
	case ct.PackageTypeExtraBase:
		return "ExtraBase"
	case ct.PackageTypeExtraPatch:
		return "ExtraPatch"
	default:
		return fmt.Sprintf("Unknown(%d)", value)
	}
}

// derefString 解引用可空字符串字段，nil 与空串在展示上等价
// derefString dereferences a nullable string field, treating nil and the empty string alike for display
func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// Inspect 解析 .ct 并返回结构快照
// catalog 解码失败时仍返回虚拟文件表，让损坏或非标准的内容表也能被浏览
// Inspect parses a .ct and returns a structural snapshot
// A failed catalog decode still returns the virtual file table so damaged or non-standard content tables remain browsable
func (s *CtExplorerService) Inspect(path string) (*CtOverview, error) {
	table, err := s.ct.ReadCt(path)
	if err != nil {
		return nil, err
	}

	overview := &CtOverview{
		Path:        path,
		Version:     table.Version,
		Framing:     "legacy",
		Directories: []CtDirectory{},
		Files:       make([]CtVirtualFile, 0, len(table.Files)),
		Extensions:  []CtExtensionNameList{},
	}
	if table.Framing == ct.VirtualDirectoryFramingExtended {
		overview.Framing = "extended"
	}
	if info, statErr := os.Stat(path); statErr == nil {
		overview.FileSize = info.Size()
	}

	for name, file := range table.Files {
		overview.Files = append(overview.Files, CtVirtualFile{Name: name, Position: file.Position, Size: file.Size})
	}
	sort.Slice(overview.Files, func(i, j int) bool { return overview.Files[i].Name < overview.Files[j].Name })

	for path, meta := range table.GetVirtualDirectoryMetadata() {
		overview.Directories = append(overview.Directories, CtDirectory{Path: path, Version: meta.Version})
	}
	sort.Slice(overview.Directories, func(i, j int) bool { return overview.Directories[i].Path < overview.Directories[j].Path })

	envelope, err := s.ct.ReadCtEnvelope(path)
	if err != nil {
		overview.CatalogError = err.Error()
		return overview, nil
	}
	overview.Catalog = convertCatalog(&envelope.Catalog)
	for key, list := range envelope.ExtensionNameLists {
		overview.Extensions = append(overview.Extensions, convertExtensionNameList(key, list))
	}
	sort.Slice(overview.Extensions, func(i, j int) bool { return overview.Extensions[i].Key < overview.Extensions[j].Key })
	return overview, nil
}

// convertCatalog 把解码后的 catalog 映射为前端结构，并把两种 catalog 布局的条目合并为一张表
// convertCatalog maps a decoded catalog to the frontend structure and merges both catalog layouts into one item table
func convertCatalog(catalog *ct.AssetBundleCatalog) *CtCatalog {
	result := &CtCatalog{
		Kind:              string(catalog.Kind),
		Version:           catalog.Version,
		CatalogType:       int32(catalog.CatalogType),
		CatalogTypeNames:  catalogTypeNames(int32(catalog.CatalogType)),
		PackageType:       int32(catalog.PackageType),
		PackageTypeName:   packageTypeName(int32(catalog.PackageType)),
		Priority:          catalog.Priority,
		Name:              derefString(catalog.Name),
		SubName:           derefString(catalog.SubName),
		Hash:              strconv.FormatUint(catalog.Hash, 10),
		CreateTime:        strconv.FormatInt(catalog.CreateTime, 10),
		IsEncrypted:       catalog.IsEncrypted,
		ResourceFileNames: make([]string, 0, len(catalog.ResourceFileNames)),
		ExtensionList:     make([]string, 0, len(catalog.ExtensionList)),
		Items:             make([]CtCatalogItem, 0, len(catalog.Items)+len(catalog.VirtualItems)),
	}
	for _, name := range catalog.ResourceFileNames {
		result.ResourceFileNames = append(result.ResourceFileNames, derefString(name))
	}
	for _, extension := range catalog.ExtensionList {
		result.ExtensionList = append(result.ExtensionList, derefString(extension))
	}
	for _, item := range catalog.Items {
		if item == nil {
			continue
		}
		result.Items = append(result.Items, CtCatalogItem{
			Name:          derefString(item.Name),
			Hash:          strconv.FormatUint(item.Hash, 10),
			ResourceIndex: item.ResourceIndex,
		})
	}
	for _, item := range catalog.VirtualItems {
		if item == nil {
			continue
		}
		result.Items = append(result.Items, CtCatalogItem{
			Name:          derefString(item.Name),
			Hash:          strconv.FormatUint(item.Hash, 10),
			ResourceIndex: -1,
			AssetPath:     derefString(item.AssetPath),
		})
	}
	return result
}

// convertExtensionNameList 映射一个扩展名分组的名称表 / convertExtensionNameList maps the name list of one extension group
func convertExtensionNameList(key string, list *ct.ExtensionNameList) CtExtensionNameList {
	result := CtExtensionNameList{Key: key, Data: []CtExtensionName{}}
	if list == nil {
		return result
	}
	result.Extension = derefString(list.Extension)
	result.Data = make([]CtExtensionName, 0, len(list.Data))
	for _, pack := range list.Data {
		if pack == nil {
			continue
		}
		result.Data = append(result.Data, CtExtensionName{
			Name: derefString(pack.Name),
			Hash: strconv.FormatUint(pack.Hash, 10),
		})
	}
	return result
}

// ExtractVirtualFile 把一个虚拟文件写到指定路径 / ExtractVirtualFile writes one virtual file to the given path
func (s *CtExplorerService) ExtractVirtualFile(ctPath string, fileName string, outPath string) error {
	table, err := s.ct.ReadCt(ctPath)
	if err != nil {
		return err
	}
	data, err := table.GetFileData(fileName)
	if err != nil {
		return fmt.Errorf("read virtual file %q: %w", fileName, err)
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return fmt.Errorf("write %q: %w", outPath, err)
	}
	return nil
}

// ExtractAllVirtualFiles 把全部虚拟文件提取到目录，虚拟文件名先经过文件名清理
// ExtractAllVirtualFiles extracts every virtual file into a directory after sanitizing each virtual file name
func (s *CtExplorerService) ExtractAllVirtualFiles(ctPath string, outDir string) ([]string, error) {
	table, err := s.ct.ReadCt(ctPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, fmt.Errorf("create %q: %w", outDir, err)
	}
	names := table.GetFileNames()
	written := make([]string, 0, len(names))
	for _, name := range names {
		data, err := table.GetFileData(name)
		if err != nil {
			return written, fmt.Errorf("read virtual file %q: %w", name, err)
		}
		target := filepath.Join(outDir, sanitizeFileName(name))
		if err := os.WriteFile(target, data, 0644); err != nil {
			return written, fmt.Errorf("write %q: %w", target, err)
		}
		written = append(written, target)
	}
	return written, nil
}

// sanitizeFileName 把虚拟文件名收敛成单个跨平台安全的文件名组件，阻止路径穿越
// sanitizeFileName reduces a virtual file name to one cross-platform-safe file-name component and blocks path traversal
func sanitizeFileName(name string) string {
	var builder strings.Builder
	builder.Grow(len(name))
	for _, character := range name {
		if character < 0x20 || strings.ContainsRune(`/\:*?"<>|`, character) {
			builder.WriteByte('_')
			continue
		}
		builder.WriteRune(character)
	}
	safe := strings.TrimRight(builder.String(), ". ")
	if safe == "" || safe == "." || safe == ".." {
		return "unnamed"
	}
	return safe
}

// GenerateFromAba 读取 .aba 的 AssetBundle 容器并生成配套 .ct，outPath 为空时输出到 .aba 同目录同基名
// GenerateFromAba reads the AssetBundle container of a .aba and generates its companion .ct, writing next to the .aba with the same base name when outPath is empty
func (s *CtExplorerService) GenerateFromAba(abaPath string, outPath string) (string, error) {
	outPath = strings.TrimSpace(outPath)
	if err := s.ct.GenerateCtFromAba(abaPath, outPath); err != nil {
		return "", err
	}
	if outPath == "" {
		base := strings.TrimSuffix(filepath.Base(abaPath), filepath.Ext(abaPath))
		outPath = filepath.Join(filepath.Dir(abaPath), base+".ct")
	}
	return outPath, nil
}

// ConvertToJson 把 .ct 转换为可编辑 JSON 文件 / ConvertToJson converts a .ct into an editable JSON file
func (s *CtExplorerService) ConvertToJson(ctx context.Context, inputPath string, outputPath string) error {
	return s.ct.ConvertCtToJson(ctx, inputPath, outputPath, maxCtJsonBytes)
}

// ConvertFromJson 把编辑 JSON 文件转换回 .ct / ConvertFromJson converts an editing JSON file back into a .ct
func (s *CtExplorerService) ConvertFromJson(ctx context.Context, inputPath string, outputPath string) error {
	return s.ct.ConvertJsonToCt(ctx, inputPath, outputPath, maxCtJsonBytes)
}
