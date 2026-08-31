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
