package internal

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/MeidoPromotionAssociation/MeidoSerialization/v2/serialization/KCES/aba"
	KCESService "github.com/MeidoPromotionAssociation/MeidoSerialization/v2/service/KCES"
)

// 压缩类型常量与 serialization/KCES/aba 保持一致，用于把 Flags 低六位翻译成显示名
// Compression constants mirror serialization/KCES/aba and translate the low six Flags bits into display names
const (
	compressionNone  = 0x00
	compressionLZMA  = 0x01
	compressionLZ4   = 0x02
	compressionLZ4HC = 0x03
)

// blockFlagStreamed 是数据块 Flags 的 bit6，标记流式块 / blockFlagStreamed is bit6 of a data-block Flags value marking a streamed block
const blockFlagStreamed = 0x40

// AbaExplorerService 提供 UnityFS 容器（.aba/.asset_bg/.asset_scene）的结构浏览、解包与打包
// AbaExplorerService browses, unpacks, and packs UnityFS containers (.aba/.asset_bg/.asset_scene)
type AbaExplorerService struct {
	aba        *KCESService.AbaService
	assetBG    *KCESService.AssetBGService
	assetScene *KCESService.AssetSceneService
	pack       *KCESService.PackService
}

// NewAbaExplorerService 创建 ABA 浏览服务 / NewAbaExplorerService creates the ABA browsing service
func NewAbaExplorerService() *AbaExplorerService {
	return &AbaExplorerService{
		aba:        &KCESService.AbaService{},
		assetBG:    &KCESService.AssetBGService{},
		assetScene: &KCESService.AssetSceneService{},
		pack:       &KCESService.PackService{},
	}
}

// AbaOverview 是一个 UnityFS 容器的完整结构快照
// AbaOverview is a complete structural snapshot of one UnityFS container
type AbaOverview struct {
	Path                string              `json:"path"`                // 容器文件路径 / Container file path
	Kind                string              `json:"kind"`                // 容器种类：aba/asset_bg/asset_scene / Container kind
	FileSize            int64               `json:"fileSize"`            // 磁盘上的字节数 / On-disk byte count
	Signature           string              `json:"signature"`           // UnityFS 签名 / UnityFS signature
	FormatVersion       uint32              `json:"formatVersion"`       // UnityFS 格式版本 / UnityFS format version
	GenerationVersion   string              `json:"generationVersion"`   // 生成版本字符串 / Generation version string
	EngineVersion       string              `json:"engineVersion"`       // Unity 引擎版本 / Unity engine version
	TotalFileSize       int64               `json:"totalFileSize"`       // 头部记录的总大小 / Total size recorded in the header
	MetadataCompression string              `json:"metadataCompression"` // 块目录元数据压缩方式 / Block-directory metadata compression
	MetadataCompressed  uint32              `json:"metadataCompressed"`  // 块目录元数据压缩后大小 / Compressed block-directory metadata size
	MetadataRaw         uint32              `json:"metadataRaw"`         // 块目录元数据解压后大小 / Decompressed block-directory metadata size
	HasDirectoryInfo    bool                `json:"hasDirectoryInfo"`    // 是否组合保存块与目录信息 / Whether block and directory info are combined
	Hash                string              `json:"hash"`                // 块目录 16 字节哈希的十六进制 / Hex of the 16-byte block-directory hash
	CompressedData      int64               `json:"compressedData"`      // 压缩数据区总字节数 / Total compressed data-area bytes
	Blocks              []AbaBlock          `json:"blocks"`              // 数据块列表 / Data block list
	Directories         []AbaDirectory      `json:"directories"`         // 目录条目列表 / Directory entry list
	SerializedFiles     []AbaSerializedFile `json:"serializedFiles"`     // 序列化文件及其对象 / Serialized files and their objects
	AssetCount          int                 `json:"assetCount"`          // 全部序列化文件中的对象总数 / Total object count across serialized files
}

// AbaBlock 是一个压缩数据块的元数据 / AbaBlock is the metadata of one compressed data block
type AbaBlock struct {
	Index            int    `json:"index"`            // 块序号 / Block index
	CompressedSize   uint32 `json:"compressedSize"`   // 压缩后大小 / Compressed size
	DecompressedSize uint32 `json:"decompressedSize"` // 解压后大小 / Decompressed size
	Compression      string `json:"compression"`      // 压缩方式名 / Compression name
	Streamed         bool   `json:"streamed"`         // 是否为流式块 / Whether the block is streamed
	Flags            uint16 `json:"flags"`            // 原始标志位 / Raw flags
}

// AbaDirectory 是容器内一个目录条目 / AbaDirectory is one directory entry inside the container
type AbaDirectory struct {
	Index            int    `json:"index"`            // 条目序号 / Entry index
	Name             string `json:"name"`             // 条目名 / Entry name
	Offset           int64  `json:"offset"`           // 相对数据区起始的偏移 / Offset relative to the data-area start
	DecompressedSize int64  `json:"decompressedSize"` // 解压后大小 / Decompressed size
	Flags            uint32 `json:"flags"`            // 原始标志位 / Raw flags
	Serialized       bool   `json:"serialized"`       // 是否为序列化 AssetsFile / Whether the entry is a serialized AssetsFile
}

// AbaSerializedFile 是容器内一个 SerializedFile 的头部、元数据与对象列表
// AbaSerializedFile is the header, metadata, and object list of one SerializedFile in the container
type AbaSerializedFile struct {
	DirectoryIndex  int                `json:"directoryIndex"`  // 所在目录条目序号 / Owning directory entry index
	Name            string             `json:"name"`            // 目录条目名 / Directory entry name
	FormatVersion   uint32             `json:"formatVersion"`   // SerializedFile 格式版本 / SerializedFile format version
	UnityVersion    string             `json:"unityVersion"`    // Unity 版本字符串 / Unity version string
	TargetPlatform  uint32             `json:"targetPlatform"`  // 目标平台 ID / Target platform ID
	TypeTreeEnabled bool               `json:"typeTreeEnabled"` // 是否含类型树 / Whether type tree data is present
	BigEndian       bool               `json:"bigEndian"`       // 是否 Big-Endian / Whether the file is Big-Endian
	MetadataSize    uint32             `json:"metadataSize"`    // 元数据块大小 / Metadata block size
	DataOffset      int64              `json:"dataOffset"`      // 数据区起始偏移 / Data-area start offset
	FileSize        int64              `json:"fileSize"`        // SerializedFile 总大小 / Total SerializedFile size
	TypeCount       int                `json:"typeCount"`       // 类型树定义数量 / Type tree definition count
	UserInformation string             `json:"userInformation"`  // 元数据尾部用户信息 / User information at the metadata tail
	ExternalFiles   []AbaExternalFile  `json:"externalFiles"`   // 外部文件引用 / External file references
	Assets          []AbaAsset         `json:"assets"`          // 对象列表 / Object list
	ContainerError  string             `json:"containerError"`  // m_Container 读取失败原因，成功时为空 / Reason m_Container reading failed, empty on success
}

// AbaExternalFile 是一条外部文件引用 / AbaExternalFile is one external file reference
type AbaExternalFile struct {
	AssetPath string `json:"assetPath"` // 缓存资源虚拟路径 / Virtual cached-asset path
	Guid      string `json:"guid"`      // GUID 十六进制 / Hex GUID
	Type      int32  `json:"type"`      // 引用类型 / Reference type
	PathName  string `json:"pathName"`  // 路径名 / Path name
}

// AbaAsset 是 SerializedFile 中的一个 Unity 对象
// PathId 走字符串通道，Unity PathID 是随机 64 位值，超出 JS number 的安全整数范围
// AbaAsset is one Unity object in a SerializedFile
// PathId travels as a string because a Unity PathID is a random 64-bit value beyond the JS safe-integer range
type AbaAsset struct {
	PathId    string `json:"pathId"`    // Unity PathID 的十进制字符串 / Decimal string of the Unity PathID
	TypeId    int32  `json:"typeId"`    // Unity class ID / Unity class ID
	TypeName  string `json:"typeName"`  // 类型名 / Type name
	Name      string `json:"name"`      // 对象 m_Name / Object m_Name
	Size      uint32 `json:"size"`      // 序列化数据字节数 / Serialized data size in bytes
	Offset    int64  `json:"offset"`    // 相对数据区的偏移 / Offset relative to the data area
	Container string `json:"container"` // AssetBundle m_Container 加载名 / AssetBundle m_Container load name
}

// compressionName 把 Flags 低六位的压缩类型翻译成显示名
// compressionName translates the compression type in the low six Flags bits into a display name
func compressionName(compression byte) string {
	switch compression {
	case compressionNone:
		return "None"
	case compressionLZMA:
		return "LZMA"
	case compressionLZ4:
		return "LZ4"
	case compressionLZ4HC:
		return "LZ4HC"
	default:
		return fmt.Sprintf("Unknown(%d)", compression)
	}
}

// readerForKind 按容器种类返回对应的读取函数，未知扩展名回落到 .aba 读取器
// readerForKind returns the reader for a container kind and falls back to the .aba reader for unknown extensions
func (s *AbaExplorerService) readerForKind(kind string) func(string) (*aba.Aba, *os.File, error) {
	switch kind {
	case "asset_bg":
		return s.assetBG.ReadAssetBG
	case "asset_scene":
		return s.assetScene.ReadAssetScene
	default:
		return s.aba.ReadAba
	}
}

// ContainerKind 按扩展名判定 UnityFS 容器种类，供前端选择解包入口
// ContainerKind determines the UnityFS container kind from the extension so the frontend can pick an unpack entry point
func (s *AbaExplorerService) ContainerKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".asset_bg":
		return "asset_bg"
	case ".asset_scene":
		return "asset_scene"
	default:
		return "aba"
	}
}

// Inspect 解析容器并返回完整结构快照，包含每个 SerializedFile 的对象列表与 m_Container 加载名
// Inspect parses a container and returns a complete structural snapshot including each SerializedFile's objects and m_Container load names
func (s *AbaExplorerService) Inspect(path string) (*AbaOverview, error) {
	kind := s.ContainerKind(path)
	abaFile, handle, err := s.readerForKind(kind)(path)
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	fileSize := int64(0)
	if info, statErr := handle.Stat(); statErr == nil {
		fileSize = info.Size()
	}

	header := abaFile.Header
	overview := &AbaOverview{
		Path:                path,
		Kind:                kind,
		FileSize:            fileSize,
		Signature:           header.Signature,
		FormatVersion:       header.Version,
		GenerationVersion:   header.GenerationVersion,
		EngineVersion:       header.EngineVersion,
		TotalFileSize:       header.FSHeader.TotalFileSize,
		MetadataCompression: compressionName(header.FSHeader.GetCompressionType()),
		MetadataCompressed:  header.FSHeader.CompressedSize,
		MetadataRaw:         header.FSHeader.DecompressedSize,
		HasDirectoryInfo:    header.FSHeader.Flags&0x40 != 0,
		Hash:                hex.EncodeToString(abaFile.BlockInfo.Hash[:]),
		Blocks:              make([]AbaBlock, 0, len(abaFile.BlockInfo.BlockInfos)),
		Directories:         make([]AbaDirectory, 0, len(abaFile.BlockInfo.DirectoryInfos)),
		SerializedFiles:     []AbaSerializedFile{},
	}

	for index := range abaFile.BlockInfo.BlockInfos {
		block := &abaFile.BlockInfo.BlockInfos[index]
		overview.CompressedData += int64(block.CompressedSize)
		overview.Blocks = append(overview.Blocks, AbaBlock{
			Index:            index,
			CompressedSize:   block.CompressedSize,
			DecompressedSize: block.DecompressedSize,
			Compression:      compressionName(block.GetCompressionType()),
			Streamed:         block.Flags&blockFlagStreamed != 0,
			Flags:            block.Flags,
		})
	}

	for index := range abaFile.BlockInfo.DirectoryInfos {
		dir := &abaFile.BlockInfo.DirectoryInfos[index]
		overview.Directories = append(overview.Directories, AbaDirectory{
			Index:            index,
			Name:             dir.Name,
			Offset:           dir.Offset,
			DecompressedSize: dir.DecompressedSize,
			Flags:            dir.Flags,
			Serialized:       dir.IsSerialized(),
		})
		if !dir.IsSerialized() {
			continue
		}
		serialized, err := s.inspectSerializedFile(abaFile, index, dir.Name)
		if err != nil {
			return nil, err
		}
		overview.AssetCount += len(serialized.Assets)
		overview.SerializedFiles = append(overview.SerializedFiles, *serialized)
	}
	return overview, nil
}

// inspectSerializedFile 解析一个序列化目录条目的头部、外部引用与全部对象
// inspectSerializedFile parses the header, external references, and every object of one serialized directory entry
func (s *AbaExplorerService) inspectSerializedFile(abaFile *aba.Aba, directoryIndex int, name string) (*AbaSerializedFile, error) {
	data, err := abaFile.GetFileData(int64(directoryIndex))
	if err != nil {
		return nil, fmt.Errorf("read serialized entry %q at directory index %d: %w", name, directoryIndex, err)
	}
	assetsFile, err := aba.ReadAssetsFile(data)
	if err != nil {
		return nil, fmt.Errorf("parse serialized entry %q at directory index %d: %w", name, directoryIndex, err)
	}

	result := &AbaSerializedFile{
		DirectoryIndex:  directoryIndex,
		Name:            name,
		FormatVersion:   assetsFile.Header.Version,
		UnityVersion:    assetsFile.Metadata.UnityVersion,
		TargetPlatform:  assetsFile.Metadata.TargetPlatform,
		TypeTreeEnabled: assetsFile.Metadata.TypeTreeEnabled,
		BigEndian:       assetsFile.Header.Endianness,
		MetadataSize:    assetsFile.Header.MetadataSize,
		DataOffset:      assetsFile.Header.DataOffset,
		FileSize:        assetsFile.Header.FileSize,
		TypeCount:       len(assetsFile.Metadata.TypeTreeTypes),
		UserInformation: assetsFile.Metadata.UserInformation,
		ExternalFiles:   make([]AbaExternalFile, 0, len(assetsFile.Metadata.ExternalFiles)),
	}

	for _, external := range assetsFile.Metadata.ExternalFiles {
		result.ExternalFiles = append(result.ExternalFiles, AbaExternalFile{
			AssetPath: external.AssetPath,
			Guid:      hex.EncodeToString(external.Guid[:]),
			Type:      external.Type,
			PathName:  external.PathName,
		})
	}

	// m_Container 读取失败不该让整个容器无法浏览，记下原因后继续列出对象
	// A failed m_Container read must not block browsing the container, so the reason is recorded and objects are still listed
	containers, err := assetsFile.GetAssetBundleContainerMap()
	if err != nil {
		result.ContainerError = err.Error()
		containers = nil
	}

	entries := assetsFile.GetAssetEntries()
	result.Assets = make([]AbaAsset, 0, len(entries))
	for _, entry := range entries {
		result.Assets = append(result.Assets, AbaAsset{
			PathId:    strconv.FormatInt(entry.PathId, 10),
			TypeId:    entry.TypeId,
			TypeName:  entry.TypeName,
			Name:      entry.Name,
			Size:      entry.Size,
			Offset:    entry.Offset,
			Container: containers[entry.PathId],
		})
	}
	return result, nil
}

// DefaultUnpackDir 返回解包目标目录的默认建议，沿用 MeidoSerialization 的 <文件名>_unpacked 约定
// DefaultUnpackDir suggests the default unpack destination following MeidoSerialization's <file name>_unpacked convention
func (s *AbaExplorerService) DefaultUnpackDir(path string) string {
	return path + "_unpacked"
}

// UnpackedFile 是解包目录中的一个文件 / UnpackedFile is one file in an unpacked directory
type UnpackedFile struct {
	RelPath string `json:"relPath"` // 相对解包根目录的斜杠分隔路径 / Slash-separated path relative to the unpack root
	AbsPath string `json:"absPath"` // 绝对路径 / Absolute path
	Kind    string `json:"kind"`    // 顶层类型目录名，如 Texture2D / Top-level type directory name such as Texture2D
	Size    int64  `json:"size"`    // 字节数 / Byte count
}

// UnpackResult 是一次解包的结果 / UnpackResult is the outcome of one unpack operation
type UnpackResult struct {
	OutDir string         `json:"outDir"` // 解包输出目录 / Unpack output directory
	Files  []UnpackedFile `json:"files"`  // 解包产生的文件 / Files produced by the unpack
}

// Unpack 将容器提取为不含 metadata、预览图和外部流文件的纯资源目录，并返回产生的文件清单
// Unpack extracts a container into a plain resource directory without metadata, previews, or external stream files and returns the resulting file list
func (s *AbaExplorerService) Unpack(path string, outDir string) (*UnpackResult, error) {
	if strings.TrimSpace(outDir) == "" {
		outDir = s.DefaultUnpackDir(path)
	}
	var err error
	switch s.ContainerKind(path) {
	case "asset_bg":
		err = s.assetBG.UnpackAssetBG(path, outDir)
	case "asset_scene":
		err = s.assetScene.UnpackAssetScene(path, outDir)
	default:
		err = s.aba.UnpackAba(path, outDir)
	}
	if err != nil {
		return nil, err
	}
	files, err := listUnpackedFiles(outDir)
	if err != nil {
		return nil, err
	}
	return &UnpackResult{OutDir: outDir, Files: files}, nil
}

// ListUnpackedDir 列出一个已存在解包目录中的文件，供前端在不重新解包时浏览并转换
// ListUnpackedDir lists files in an existing unpacked directory so the frontend can browse and convert without unpacking again
func (s *AbaExplorerService) ListUnpackedDir(dirPath string) (*UnpackResult, error) {
	files, err := listUnpackedFiles(dirPath)
	if err != nil {
		return nil, err
	}
	return &UnpackResult{OutDir: dirPath, Files: files}, nil
}

// listUnpackedFiles 递归收集解包目录中的普通文件，按相对路径排序
// listUnpackedFiles recursively collects regular files in an unpacked directory sorted by relative path
func listUnpackedFiles(dirPath string) ([]UnpackedFile, error) {
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("stat unpacked directory %q: %w", dirPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", dirPath)
	}
	files := make([]UnpackedFile, 0, 64)
	err = filepath.WalkDir(dirPath, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(dirPath, current)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		fileInfo, statErr := entry.Info()
		if statErr != nil {
			return statErr
		}
		kind := ""
		if index := strings.IndexByte(rel, '/'); index > 0 {
			kind = rel[:index]
		}
		files = append(files, UnpackedFile{RelPath: rel, AbsPath: current, Kind: kind, Size: fileInfo.Size()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk unpacked directory %q: %w", dirPath, err)
	}
	return files, nil
}

// PackResult 是一次打包的结果，包含两个输出路径与打包器给出的游戏加载警告
// PackResult is the outcome of one pack operation with both output paths and the packer's game-loading warnings
type PackResult struct {
	AbaPath  string   `json:"abaPath"`  // 生成的 ABA 路径 / Path of the generated ABA
	CtPath   string   `json:"ctPath"`   // 生成的配套 CT 路径 / Path of the generated companion CT
	Warnings []string `json:"warnings"` // 打包器输出的警告，例如包名与内部容器名不匹配 / Warnings from the packer, such as a bundle name that does not match the inner container names
}

// Pack 扫描纯资源目录并在其父目录生成固定 Unity 2022.3.35f1 的 ABA 与配套 CT
// outputBaseName 为空时取目录名，并去掉 MeidoSerialization 解包时追加的 _unpacked 后缀
// Pack scans a plain resource directory and emits a fixed Unity 2022.3.35f1 ABA with its companion CT in the parent directory
// An empty outputBaseName falls back to the directory name with the _unpacked suffix added by MeidoSerialization removed
func (s *AbaExplorerService) Pack(dirPath string, outputBaseName string) (*PackResult, error) {
	name := strings.TrimSpace(outputBaseName)
	if name == "" {
		name = defaultPackName(dirPath)
	}
	// 打包器把"MOD 能加载但部件定义不生效"这类警告写到标准错误，GUI 里没人看得到，
	// 所以在调用期间收集标准输出与标准错误并随结果返回
	// The packer writes warnings such as "the MOD loads but its parts never take effect" to standard error,
	// which nobody sees in a GUI, so the call captures both standard streams and returns them with the result
	output, err := captureOutput(func() error {
		return s.pack.PackToAbaAndCt(dirPath, name)
	})
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(filepath.Clean(dirPath))
	return &PackResult{
		AbaPath:  filepath.Join(parent, name+".aba"),
		CtPath:   filepath.Join(parent, name+".ct"),
		Warnings: splitPackWarnings(output),
	}, nil
}

// outputCaptureLock 串行化标准流重定向，避免并发打包互相截获对方的输出
// outputCaptureLock serializes standard-stream redirection so concurrent packs cannot capture each other's output
var outputCaptureLock sync.Mutex

// captureOutput 在执行 fn 期间把标准输出与标准错误重定向到同一管道并收集内容
// 建管道失败时直接执行 fn，宁可丢掉警告也不能让打包本身失败
// captureOutput redirects both standard output and standard error to one pipe while fn runs and collects what was written
// When the pipe cannot be created fn still runs, because losing warnings must not fail the pack itself
func captureOutput(fn func() error) (string, error) {
	outputCaptureLock.Lock()
	defer outputCaptureLock.Unlock()

	reader, writer, err := os.Pipe()
	if err != nil {
		return "", fn()
	}
	originalStdout, originalStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = writer, writer

	collected := make(chan string, 1)
	go func() {
		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, reader)
		collected <- buffer.String()
	}()

	callErr := fn()

	os.Stdout, os.Stderr = originalStdout, originalStderr
	_ = writer.Close()
	output := <-collected
	_ = reader.Close()
	return output, callErr
}

// splitPackWarnings 按 warning 标记切分打包器输出，每段保留其英文与中文两种说明
// splitPackWarnings splits packer output on the warning marker and keeps both the English and Chinese wording of each entry
func splitPackWarnings(output string) []string {
	warnings := make([]string, 0, 2)
	for _, part := range strings.Split(output, "warning:") {
		text := strings.TrimSpace(part)
		if text != "" {
			warnings = append(warnings, text)
		}
	}
	return warnings
}

// DefaultPackName 返回打包输出基名的默认建议 / DefaultPackName suggests the default pack output base name
func (s *AbaExplorerService) DefaultPackName(dirPath string) string {
	return defaultPackName(dirPath)
}

// defaultPackName 从解包目录名推导打包基名，剥离 _unpacked 后缀与原有容器扩展名
// defaultPackName derives the pack base name from an unpacked directory name by stripping the _unpacked suffix and any original container extension
func defaultPackName(dirPath string) string {
	name := filepath.Base(filepath.Clean(dirPath))
	name = strings.TrimSuffix(name, "_unpacked")
	for _, ext := range []string{".aba", ".asset_bg", ".asset_scene"} {
		if strings.EqualFold(filepath.Ext(name), ext) {
			return strings.TrimSuffix(name, filepath.Ext(name))
		}
	}
	return name
}
