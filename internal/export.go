package internal

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MeidoPromotionAssociation/MeidoSerialization/v2/serialization/KCES/aba"
)

// 单个对象的导出形态：TextAsset 的内容本身就是可用的 KCES 格式文件，图像类解码成 PNG
// Export shapes for a single object: a TextAsset's content is already a usable KCES file, image types decode to PNG
const (
	ExportKindRaw = "raw"
	ExportKindPng = "png"
)

// AssetExportKind 判断一个对象能否单独导出，以及导出成什么形态
// 其余类型（Mesh、AnimationClip、Material 等）要重建独立 Unity 对象并重写引用，
// 那套逻辑在 MeidoSerialization 的解包流程里，单独复刻容易和解包结果不一致，因此交给"解包"功能
// AssetExportKind reports whether an object can be exported on its own and in what shape
// Other types such as Mesh, AnimationClip, and Material need a standalone Unity object with rewritten references,
// which lives in MeidoSerialization's unpack flow; reimplementing it here risks diverging from unpack output, so those go through Unpack
func AssetExportKind(typeId int32) string {
	switch typeId {
	case aba.ClassIDTextAsset:
		return ExportKindRaw
	case aba.ClassIDTexture2D, aba.ClassIDSprite:
		return ExportKindPng
	default:
		return ""
	}
}

// AssetExportKindForType 供前端按类型 ID 查询导出形态，空字符串表示不支持单独导出
// AssetExportKindForType lets the frontend query the export shape by class ID, with an empty string meaning single-object export is unsupported
func (s *AbaExplorerService) AssetExportKindForType(typeId int32) string {
	return AssetExportKind(typeId)
}

// LocatedAsset 是搜索结果在容器内定位到的对象
// 搜索索引只记名字与归属，PathID 和所在 SerializedFile 要打开容器才知道，所以导出前在这里现查一次
// LocatedAsset is the object a search result was located to inside a container
// The search index records names and owners only, so the PathID and the owning SerializedFile are resolved here,
// right before an export
type LocatedAsset struct {
	SourceName string `json:"sourceName"` // 所在 SerializedFile 条目名 / Owning SerializedFile entry name
	PathId     string `json:"pathId"`     // Unity PathID 的十进制字符串 / Decimal string of the Unity PathID
	Name       string `json:"name"`       // 还原出的 KCES 资源名，可直接用作导出文件名 / Recovered KCES resource name, usable as the export file name
	ExportKind string `json:"exportKind"` // 导出形态，空表示不能单独导出 / Export shape, empty when single-object export is unsupported
}

// LocateAsset 在容器内按搜索结果定位对象
// owner 非空时按宿主条目定位：.menu/.mate/.pmat 这些名字只存在于容器条目的内部，索引里记的宿主就是承载它们的那个对象；
// owner 为空时按 m_Container 加载名定位，catalog 层与容器层记的都是这个名字
// 两种情况下都拿加载名还原出的资源名去比，与建索引时的口径一致
// LocateAsset locates the object behind a search result inside a container
// A non-empty owner locates by owning entry: names such as .menu, .mate, and .pmat live only inside a container entry,
// and the owner recorded in the index is the object carrying them. An empty owner locates by m_Container load name,
// which is the name both the catalog layer and the container layer record.
// Both compare against the resource name recovered from the load name, the same rule the index was built with
func (s *AbaExplorerService) LocateAsset(containerPath string, owner string, name string) (LocatedAsset, error) {
	target := strings.TrimSpace(owner)
	if target == "" {
		target = strings.TrimSpace(name)
	}
	if target == "" {
		return LocatedAsset{}, fmt.Errorf("no name was given to locate")
	}

	abaFile, handle, err := s.readerForKind(s.ContainerKind(containerPath))(containerPath)
	if err != nil {
		return LocatedAsset{}, err
	}
	defer handle.Close()

	// 某个条目解析失败不该盖掉后面条目里可能存在的命中，但全都没命中时要能说清原因
	// A failing entry must not mask a hit in a later one, yet a total miss still has to explain itself
	var firstErr error
	for index := range abaFile.BlockInfo.DirectoryInfos {
		dir := &abaFile.BlockInfo.DirectoryInfos[index]
		if !dir.IsSerialized() {
			continue
		}
		directoryIndex := int64(index)
		assetsFile, err := aba.ReadAssetsFileRange(dir.DecompressedSize, func(offset int64, size int64) ([]byte, error) {
			return abaFile.GetFileDataRange(directoryIndex, offset, size)
		})
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("parse serialized entry %q: %w", dir.Name, err)
			}
			continue
		}
		containers, err := assetsFile.GetAssetBundleContainerMap()
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("read the m_Container of %q: %w", dir.Name, err)
			}
			continue
		}
		for pathID, loadName := range containers {
			resource := resourceNameFromLoadName(loadName)
			if resource == "" || !strings.EqualFold(resource, target) {
				continue
			}
			info := assetsFile.GetAssetInfoByPathID(pathID)
			if info == nil {
				continue
			}
			return LocatedAsset{
				SourceName: dir.Name,
				PathId:     strconv.FormatInt(pathID, 10),
				Name:       resource,
				ExportKind: AssetExportKind(info.TypeId),
			}, nil
		}
	}
	if firstErr != nil {
		return LocatedAsset{}, firstErr
	}
	return LocatedAsset{}, fmt.Errorf("%q is not present in %q", target, containerPath)
}

// tempExportDir 是"导出到临时目录"的落脚点
// 固定一个子目录而不是每次新建：同一个对象反复打开时复用同一个文件，编辑器里拿到的路径稳定，
// 也不会在临时目录里堆下一串副本
// tempExportDir is where exports destined for the temporary directory land
// One fixed subdirectory instead of a fresh one per call: reopening the same object reuses the same file,
// keeping the path stable inside the editor and avoiding a pile of copies in the temporary directory
func tempExportDir() (string, error) {
	dir := filepath.Join(os.TempDir(), "ABA_EXPLORER")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create the temporary export directory %q: %w", dir, err)
	}
	return dir, nil
}

// ExportAssetToTemp 把容器内的单个对象导出到应用的临时目录并返回写出的绝对路径
// 供"用 KCES MOD EDITOR 打开"这类必须先落盘再交给外部程序的流程使用，临时文件交给系统清理
// fileName 是写出的文件名，只取末段，避免调用方给出的名字带上路径把文件写到别处
// ExportAssetToTemp writes one object from the container into the application's temporary directory and returns the absolute path written
// It backs flows that must land on disk before an external program can take over, such as opening the file in KCES MOD EDITOR,
// and the system cleans the temporary copy up afterwards
// fileName is the name to write under, reduced to its last segment so a caller-supplied name cannot place the file elsewhere
func (s *AbaExplorerService) ExportAssetToTemp(containerPath string, sourceName string, pathId string, fileName string) (string, error) {
	name, err := tempExportName(fileName)
	if err != nil {
		return "", err
	}
	dir, err := tempExportDir()
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, name)
	if err := s.ExportAsset(containerPath, sourceName, pathId, target); err != nil {
		return "", err
	}
	return target, nil
}

// tempExportName 从调用方给出的文件名里取末段作为临时文件名
// 只取末段，这样调用方给的名字带上路径时也写不到临时目录之外
// tempExportName takes the last segment of a caller-supplied file name as the temporary file name
// Reducing to the last segment keeps a name carrying a path from writing outside the temporary directory
func tempExportName(fileName string) (string, error) {
	name := strings.TrimSpace(fileName)
	if index := strings.LastIndexAny(name, `/\`); index >= 0 {
		name = name[index+1:]
	}
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("no file name was given for the temporary export")
	}
	return name, nil
}

// ExportAsset 把容器内的单个对象写到 outPath
// 只读取目标 SerializedFile 的必要范围，因此从 500 MB 级别的整合包里取一张贴图也不必解压整个条目
// ExportAsset writes one object from the container to outPath
// It reads only the ranges it needs from the target SerializedFile, so pulling one texture out of a 500 MB bundle avoids decompressing the whole entry
func (s *AbaExplorerService) ExportAsset(containerPath string, sourceName string, pathId string, outPath string) error {
	if strings.TrimSpace(outPath) == "" {
		return fmt.Errorf("no output path was given")
	}
	id, err := strconv.ParseInt(strings.TrimSpace(pathId), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid PathID %q: %w", pathId, err)
	}

	abaFile, handle, err := s.readerForKind(s.ContainerKind(containerPath))(containerPath)
	if err != nil {
		return err
	}
	defer handle.Close()

	files, target, err := openSerializedFiles(abaFile, sourceName)
	if err != nil {
		return err
	}
	info := target.GetAssetInfoByPathID(id)
	if info == nil {
		return fmt.Errorf("PathID %d is not present in %q", id, sourceName)
	}

	entry := info.TypeId
	switch AssetExportKind(entry) {
	case ExportKindRaw:
		_, script, err := target.GetTextAssetData(info)
		if err != nil {
			return fmt.Errorf("read TextAsset PathID %d: %w", id, err)
		}
		if err := os.WriteFile(outPath, script, 0644); err != nil {
			return fmt.Errorf("write %q: %w", outPath, err)
		}
		return nil
	case ExportKindPng:
		return exportImageAsset(abaFile, files, target, info, entry, outPath)
	default:
		return fmt.Errorf("this object type cannot be exported on its own; unpack the container instead")
	}
}

// openSerializedFiles 按范围读取容器内全部 SerializedFile 并定位目标条目
// 用范围读取而不是整体解压：一个整合包的 SerializedFile 解压后可达数百 MB，取一个对象没必要全载入
// openSerializedFiles opens every SerializedFile in the container through range reads and locates the target entry
// Range reads instead of whole-entry decompression: one bundle's SerializedFile can exceed hundreds of megabytes and a single object does not need all of it
func openSerializedFiles(abaFile *aba.Aba, sourceName string) (map[string]*aba.AssetsFile, *aba.AssetsFile, error) {
	files := make(map[string]*aba.AssetsFile, 2)
	var target *aba.AssetsFile

	for index := range abaFile.BlockInfo.DirectoryInfos {
		dir := &abaFile.BlockInfo.DirectoryInfos[index]
		if !dir.IsSerialized() {
			continue
		}
		directoryIndex := int64(index)
		assetsFile, err := aba.ReadAssetsFileRange(dir.DecompressedSize, func(offset int64, size int64) ([]byte, error) {
			return abaFile.GetFileDataRange(directoryIndex, offset, size)
		})
		if err != nil {
			return nil, nil, fmt.Errorf("parse serialized entry %q: %w", dir.Name, err)
		}
		// 两种键都登记：Sprite 的图集引用可能写全名也可能写 basename
		// Both keys are registered because a Sprite's atlas reference may use the full name or just the basename
		files[strings.ToLower(filepath.ToSlash(dir.Name))] = assetsFile
		files[strings.ToLower(path.Base(strings.ReplaceAll(dir.Name, "\\", "/")))] = assetsFile
		if dir.Name == sourceName {
			target = assetsFile
		}
	}
	if target == nil {
		return nil, nil, fmt.Errorf("serialized entry %q is not present in the container", sourceName)
	}
	return files, target, nil
}

// abaStreamResolver 返回读取非序列化条目（.resS 之类）字节范围的解析器
// 贴图的 m_StreamData 指向这些 sidecar，范围读取避免为一张贴图载入整个流文件
// abaStreamResolver returns a resolver for byte ranges inside non-serialized entries such as .resS
// Texture m_StreamData points into those sidecars, and range reads avoid loading a whole stream file for one texture
func abaStreamResolver(abaFile *aba.Aba) aba.AbaFileRangeResolver {
	index := make(map[string]int64, 2)
	for position := range abaFile.BlockInfo.DirectoryInfos {
		dir := &abaFile.BlockInfo.DirectoryInfos[position]
		if dir.IsSerialized() {
			continue
		}
		index[strings.ToLower(path.Base(strings.ReplaceAll(dir.Name, "\\", "/")))] = int64(position)
	}
	return func(name string, offset int64, size int64) ([]byte, error) {
		key := strings.ToLower(path.Base(strings.ReplaceAll(name, "\\", "/")))
		position, ok := index[key]
		if !ok {
			return nil, fmt.Errorf("stream entry %q is not present in the container", name)
		}
		return abaFile.GetFileDataRange(position, offset, size)
	}
}

// exportImageAsset 把 Texture2D 或 Sprite 解码为 PNG 写出
// exportImageAsset decodes a Texture2D or Sprite into a PNG and writes it out
func exportImageAsset(
	abaFile *aba.Aba,
	files map[string]*aba.AssetsFile,
	target *aba.AssetsFile,
	info *aba.AssetInfo,
	typeId int32,
	outPath string,
) error {
	streams := abaStreamResolver(abaFile)
	if typeId == aba.ClassIDSprite {
		sprite, err := target.GetSpriteExportRange(info, aba.AbaAssetResolver(files), streams)
		if err != nil {
			return fmt.Errorf("read Sprite PathID %d: %w", info.PathId, err)
		}
		if err := aba.WriteSpritePNG(sprite, outPath); err != nil {
			return fmt.Errorf("write %q: %w", outPath, err)
		}
		return nil
	}
	texture, err := target.GetTexture2DDataRange(info, streams)
	if err != nil {
		return fmt.Errorf("read Texture2D PathID %d: %w", info.PathId, err)
	}
	if err := aba.WriteTexturePNG(texture, outPath); err != nil {
		return fmt.Errorf("write %q: %w", outPath, err)
	}
	return nil
}
