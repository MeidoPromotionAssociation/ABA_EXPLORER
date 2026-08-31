package internal

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	KCESService "github.com/MeidoPromotionAssociation/MeidoSerialization/v2/service/KCES"
)

// maxConvertOutputBytes 是单次转换允许写出的最大字节数，与 MeidoSerialization 的默认上限一致
// maxConvertOutputBytes caps the bytes one conversion may write, matching MeidoSerialization's default limit
const maxConvertOutputBytes int64 = 10 << 30

// 转换目标键，前端按键选择目标格式并做 i18n / Conversion target keys the frontend selects by and localizes
const (
	targetJson  = "json"
	targetPng   = "png"
	targetGlb   = "glb"
	targetGltf  = "gltf"
	targetAudio = "audio"
	targetCsv   = "csv"
)

// ConvertService 把解包产物与 KCES 原生文件转换为通用格式，全部直接调用 MeidoSerialization 的 service 包
// ConvertService converts unpacked assets and KCES native files into common formats by calling MeidoSerialization's service packages directly
type ConvertService struct {
	media *KCESService.NativeUnityMediaService
	model *KCESService.ModelService
}

// NewConvertService 创建转换服务 / NewConvertService creates the conversion service
func NewConvertService() *ConvertService {
	return &ConvertService{
		media: &KCESService.NativeUnityMediaService{},
		model: &KCESService.ModelService{},
	}
}

// ConvertTarget 是一个文件可用的转换目标 / ConvertTarget is one available conversion target for a file
type ConvertTarget struct {
	Key string `json:"key"` // 目标键：json/png/glb/gltf/audio/csv / Target key: json/png/glb/gltf/audio/csv
	Ext string `json:"ext"` // 输出扩展名，audio 在转换时探测所以为空 / Output extension, empty for audio which is detected at conversion time
}

// ConvertOutcome 是一个文件的转换结果 / ConvertOutcome is the conversion result of one file
type ConvertOutcome struct {
	InputPath  string `json:"inputPath"`  // 输入路径 / Input path
	OutputPath string `json:"outputPath"` // 输出路径，失败时为空 / Output path, empty on failure
	Target     string `json:"target"`     // 使用的目标键 / Target key used
	Error      string `json:"error"`      // 失败原因，成功时为空 / Failure reason, empty on success
}

// Targets 返回一个文件可用的转换目标，同一文件可能同时支持结构化 JSON 与媒体导出
// Targets returns the conversion targets available for one file, which may support both structured JSON and a media export
func (s *ConvertService) Targets(path string) []ConvertTarget {
	targets := make([]ConvertTarget, 0, 3)
	if KCESService.IsKCESNativeTexture2DFile(path) || KCESService.IsKCESNativeSpriteFile(path) {
		targets = append(targets, ConvertTarget{Key: targetPng, Ext: ".png"})
	}
	if KCESService.IsKCESNativeMeshFile(path) || KCESService.IsKCESNativeAnimationClipFile(path) || KCESService.IsKCESModelFile(path) {
		targets = append(targets, ConvertTarget{Key: targetGlb, Ext: ".glb"}, ConvertTarget{Key: targetGltf, Ext: ".gltf"})
	}
	if KCESService.IsKCESNativeAudioClipFile(path) {
		targets = append(targets, ConvertTarget{Key: targetAudio})
	}
	if KCESService.IsKCESNeiFile(path) {
		targets = append(targets, ConvertTarget{Key: targetCsv, Ext: ".csv"})
	}
	if supportsJsonConversion(path) {
		targets = append(targets, ConvertTarget{Key: targetJson, Ext: ".json"})
	}
	return targets
}

// supportsJsonConversion 判断文件是否有对应的结构化 JSON 转换服务
// 判定集合与 MeidoSerialization CLI 的 convert2json 保持一致
// supportsJsonConversion reports whether a file has a structured JSON conversion service
// The set matches MeidoSerialization's convert2json CLI
func supportsJsonConversion(path string) bool {
	return KCESService.IsKCESBridgeSessionFile(path) ||
		KCESService.IsKCESGP03BridgeFile(path) ||
		KCESService.IsKCESExportNameMapFile(path) ||
		KCESService.IsKCESSavedAttachFile(path) ||
		KCESService.IsKCESSystemDataFile(path) ||
		KCESService.IsKCESPathsFile(path) ||
		KCESService.IsKCESMaidColliderFile(path) ||
		KCESService.IsKCESPayloadFile(path) ||
		KCESService.IsKCESPartsFile(path) ||
		KCESService.IsKCESMiscFile(path) ||
		KCESService.IsKCESRawUnityBytesFile(path) ||
		KCESService.IsKCESCtFile(path) ||
		KCESService.IsKCESPresetFile(path) ||
		KCESService.IsKCESPersetFile(path) ||
		KCESService.IsKCESPskFile(path) ||
		KCESService.IsKCESDataFile(path)
}

// convertToJson 按 MeidoSerialization CLI 的判定顺序分发到对应的结构化 JSON 转换服务
// .perset 排在 .preset 之前，因为 IsKCESPresetFile 同时匹配两种扩展名
// convertToJson dispatches to the matching structured JSON conversion service in MeidoSerialization's CLI order
// .perset comes before .preset because IsKCESPresetFile matches both extensions
func (s *ConvertService) convertToJson(ctx context.Context, path string, outPath string) error {
	switch {
	case KCESService.IsKCESBridgeSessionFile(path):
		return (&KCESService.BridgeSessionService{}).ConvertBridgeSessionToJSON(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESGP03BridgeFile(path):
		return (&KCESService.GP03BridgeService{}).ConvertBridgeToJSON(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESExportNameMapFile(path):
		return (&KCESService.ExportNameMapService{}).ConvertExportNameMapToJSON(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESSavedAttachFile(path):
		return (&KCESService.SavedAttachService{}).ConvertSavedAttachToJSON(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESSystemDataFile(path):
		return (&KCESService.SystemDataService{}).ConvertSystemDataToJSON(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESPathsFile(path):
		return (&KCESService.PathsService{}).ConvertPathsToJSON(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESMaidColliderFile(path):
		return (&KCESService.MaidColliderService{}).ConvertMaidColliderToJSON(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESPayloadFile(path):
		return (&KCESService.PayloadService{}).ConvertPayloadToJson(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESPartsFile(path):
		return (&KCESService.PartsService{}).ConvertPartsToJson(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESMiscFile(path):
		return (&KCESService.MiscService{}).ConvertMiscToJson(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESRawUnityBytesFile(path):
		return (&KCESService.RawUnityObjectService{}).ConvertRawUnityObjectToJson(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESCtFile(path):
		return (&KCESService.CtService{}).ConvertCtToJson(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESPersetFile(path):
		return (&KCESService.PersetService{}).ConvertPersetToJson(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESPresetFile(path):
		return (&KCESService.PresetService{}).ConvertPresetToJson(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESDataFile(path):
		return (&KCESService.DataService{}).ConvertDataToJson(ctx, path, outPath, maxConvertOutputBytes)
	default:
		return fmt.Errorf("no structured JSON conversion for %s", filepath.Ext(path))
	}
}

// Convert 按目标键转换单个文件，outPath 为空时按目标的默认命名写到输入文件旁
// Convert converts one file for a target key and writes next to the input using the target's default naming when outPath is empty
func (s *ConvertService) Convert(ctx context.Context, path string, target string, outPath string) (string, error) {
	outPath = strings.TrimSpace(outPath)
	switch target {
	case targetPng:
		if outPath == "" {
			outPath = replaceExt(path, ".png")
		}
		return outPath, s.convertToImage(ctx, path, outPath)
	case targetGlb, targetGltf:
		if outPath == "" {
			outPath = replaceExt(path, "."+target)
		}
		return outPath, s.convertToGltf(ctx, path, outPath, target)
	case targetAudio:
		return s.convertAudio(ctx, path, outPath)
	case targetCsv:
		if outPath == "" {
			outPath = replaceExt(path, ".csv")
		}
		return outPath, (&KCESService.DataService{}).ConvertNeiToCSV(path, outPath)
	case targetJson:
		if outPath == "" {
			outPath = path + ".json"
		}
		return outPath, s.convertToJson(ctx, path, outPath)
	default:
		return "", fmt.Errorf("unsupported conversion target %q", target)
	}
}

// ConvertBatch 批量转换，单个文件失败不会中断其余文件，逐条返回结果
// outDir 为空时输出写到各输入文件旁
// ConvertBatch converts many files, records each result, and keeps going when one file fails
// An empty outDir writes each output next to its input file
func (s *ConvertService) ConvertBatch(ctx context.Context, paths []string, target string, outDir string) ([]ConvertOutcome, error) {
	outDir = strings.TrimSpace(outDir)
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return nil, fmt.Errorf("create %q: %w", outDir, err)
		}
	}
	outcomes := make([]ConvertOutcome, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return outcomes, err
		}
		outcome := ConvertOutcome{InputPath: path, Target: target}
		outPath := ""
		if outDir != "" {
			outPath = batchOutputPath(outDir, path, target)
		}
		written, err := s.Convert(ctx, path, target, outPath)
		if err != nil {
			outcome.Error = err.Error()
		} else {
			outcome.OutputPath = written
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

// replaceExt 用新扩展名替换路径的最后一层扩展名 / replaceExt swaps the last extension of a path for a new one
func replaceExt(path string, ext string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + ext
}

// batchOutputPath 为批量转换在输出目录内构造目标路径，JSON 追加扩展名而媒体格式替换扩展名
// batchOutputPath builds the destination inside the output directory for a batch conversion, appending the extension for JSON and replacing it for media formats
func batchOutputPath(outDir string, inputPath string, target string) string {
	base := filepath.Base(inputPath)
	switch target {
	case targetJson:
		return filepath.Join(outDir, base+".json")
	case targetAudio:
		// 音频扩展名要等探测结果，先给出无扩展名的基名，convertAudio 会补齐
		// The audio extension depends on detection, so the stem goes out bare and convertAudio completes it
		return filepath.Join(outDir, strings.TrimSuffix(base, filepath.Ext(base)))
	default:
		return filepath.Join(outDir, strings.TrimSuffix(base, filepath.Ext(base))+"."+target)
	}
}

// convertToImage 把原生 Texture2D 或 Sprite 导出为 PNG / convertToImage exports a native Texture2D or Sprite to PNG
func (s *ConvertService) convertToImage(ctx context.Context, path string, outPath string) error {
	switch {
	case KCESService.IsKCESNativeSpriteFile(path):
		return s.media.ConvertSpriteToPNG(ctx, path, outPath, maxConvertOutputBytes)
	case KCESService.IsKCESNativeTexture2DFile(path):
		return s.media.ConvertTexture2DToImage(ctx, path, outPath, "png", maxConvertOutputBytes)
	default:
		return fmt.Errorf("not a native Texture2D or Sprite file: %s", path)
	}
}

// convertToGltf 把 Model、Mesh 或 AnimationClip 导出为 glTF
// 独立 Mesh 只含几何体，带骨架的导出需要用 .model 作为输入
// convertToGltf exports a Model, Mesh, or AnimationClip to glTF
// A standalone Mesh carries geometry only, and a skinned export needs the .model file as input
func (s *ConvertService) convertToGltf(ctx context.Context, path string, outPath string, format string) error {
	switch {
	case KCESService.IsKCESModelFile(path):
		return s.model.ConvertModelToGLTF(ctx, path, outPath, format, maxConvertOutputBytes)
	case KCESService.IsKCESNativeMeshFile(path):
		return s.media.ConvertMeshToGLTF(ctx, path, outPath, format, maxConvertOutputBytes)
	case KCESService.IsKCESNativeAnimationClipFile(path):
		return s.media.ConvertAnimationClipToGLTF(ctx, path, outPath, format, maxConvertOutputBytes)
	default:
		return fmt.Errorf("not a KCES Model, native Mesh, or native AnimationClip file: %s", path)
	}
}

// convertAudio 无损提取 AudioClip 的内联编码载荷，输出扩展名由载荷探测决定
// convertAudio losslessly extracts an AudioClip's inline encoded payload with the extension chosen by payload detection
func (s *ConvertService) convertAudio(ctx context.Context, path string, outPath string) (string, error) {
	extension, err := s.media.DetectAudioClipExtension(ctx, path)
	if err != nil {
		return "", err
	}
	if outPath == "" {
		outPath = replaceExt(path, extension)
	} else if filepath.Ext(outPath) == "" {
		outPath += extension
	}
	return outPath, s.media.ExtractAudioClip(ctx, path, outPath, maxConvertOutputBytes)
}

// maxImagePreviewBytes 是图片预览允许的 PNG 字节数上限，超过就不再传给前端
// 4K 纹理的 PNG 通常在个位数 MB，这个上限留了足够余量又不会把巨大贴图塞进 webview
// maxImagePreviewBytes caps the PNG bytes an image preview may carry to the frontend
// A 4K texture usually lands in the low single-digit megabytes, so this leaves headroom without pushing huge atlases into the webview
const maxImagePreviewBytes int64 = 24 << 20

// CanPreviewImage 判断文件是否为可渲染的原生 Texture2D 或 Sprite
// CanPreviewImage reports whether a file is a renderable native Texture2D or Sprite
func (s *ConvertService) CanPreviewImage(path string) bool {
	return KCESService.IsKCESNativeTexture2DFile(path) || KCESService.IsKCESNativeSpriteFile(path)
}

// PreviewImage 把原生 Texture2D 或 Sprite 解码为 PNG 并以 data URL 返回
// 库的转换接口是文件到文件，所以先写入临时目录再读回，返回前删除临时文件
// PreviewImage decodes a native Texture2D or Sprite into a PNG returned as a data URL
// The library converts file to file, so the PNG goes to a temporary directory and is removed before returning
func (s *ConvertService) PreviewImage(ctx context.Context, path string) (string, error) {
	if !s.CanPreviewImage(path) {
		return "", fmt.Errorf("not a native Texture2D or Sprite file: %s", path)
	}
	tempDir, err := os.MkdirTemp("", "aba-explorer-preview-")
	if err != nil {
		return "", fmt.Errorf("create preview directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	target := filepath.Join(tempDir, "preview.png")
	if err := s.convertToImage(ctx, path, target); err != nil {
		return "", err
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("stat decoded preview: %w", err)
	}
	if info.Size() > maxImagePreviewBytes {
		return "", fmt.Errorf("decoded preview needs %d bytes but the limit is %d", info.Size(), maxImagePreviewBytes)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("read decoded preview: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data), nil
}
