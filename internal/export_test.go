package internal

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// decodedPixels 把 PNG 解成一串规范化的 RGBA 字节，用于比较两张图的内容
// PNG 的字节表示不能直接比：BC7 之类的压缩格式要经过 ImageMagick，它会往 tIME 和 tEXt 里写
// 当前时间，于是同一张图每次编码出来的字节都不同，而 IHDR 与 IDAT 其实完全一致
// decodedPixels decodes a PNG into a canonical RGBA byte run for comparing the content of two images
// PNG bytes cannot be compared directly: compressed formats such as BC7 go through ImageMagick, which stamps
// the current time into tIME and tEXt, so the same image encodes to different bytes every time even though
// IHDR and IDAT are identical
func decodedPixels(t *testing.T, data []byte, label string) []byte {
	t.Helper()
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode the %s PNG: %v", label, err)
	}
	bounds := decoded.Bounds()
	if bounds.Empty() {
		t.Fatalf("the %s PNG is empty", label)
	}
	pixels := make([]byte, 0, bounds.Dx()*bounds.Dy()*4)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			red, green, blue, alpha := decoded.At(x, y).RGBA()
			pixels = append(pixels, byte(red>>8), byte(green>>8), byte(blue>>8), byte(alpha>>8))
		}
	}
	return pixels
}

// pngBounds 返回一张 PNG 的尺寸 / pngBounds returns the dimensions of a PNG
func pngBounds(t *testing.T, data []byte, label string) image.Rectangle {
	t.Helper()
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("read the %s PNG header: %v", label, err)
	}
	return image.Rect(0, 0, config.Width, config.Height)
}

// findAsset 在结构快照里找到第一个指定类型的对象及其所属 SerializedFile 名
// findAsset locates the first object of a given type in a snapshot along with its owning SerializedFile name
func findAsset(overview *AbaOverview, typeName string) (source string, asset *AbaAsset) {
	for _, file := range overview.SerializedFiles {
		for index := range file.Assets {
			if file.Assets[index].TypeName == typeName {
				return file.Name, &file.Assets[index]
			}
		}
	}
	return "", nil
}

// TestExportAssetMatchesUnpack 检查单个对象导出的结果与整体解包出来的同一个文件完全一致
// 这是这套导出最重要的性质：它走的是自己的读取路径，但绝不能和 MeidoSerialization 的解包产物有分歧
// TestExportAssetMatchesUnpack checks a single-object export is byte-identical to the same file produced by a full unpack
// That is the key property here: the export uses its own read path but must never diverge from MeidoSerialization's unpack output
func TestExportAssetMatchesUnpack(t *testing.T) {
	dir := testDataDir(t)
	source := filepath.Join(dir, "cm3d2_megane002.aba")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("sample is unavailable: %v", err)
	}

	service := NewAbaExplorerService()
	overview, err := service.Inspect(source)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	workDir := t.TempDir()
	unpacked, err := service.Unpack(source, filepath.Join(workDir, "unpacked"))
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	// TextAsset 的导出内容应当和解包目录里 TextAsset/<name> 的字节一致
	sourceName, asset := findAsset(overview, "TextAsset")
	if asset == nil {
		t.Skip("this sample has no TextAsset")
	}
	exported := filepath.Join(workDir, "exported"+filepath.Ext(asset.Name))
	if err := service.ExportAsset(source, sourceName, asset.PathId, exported); err != nil {
		t.Fatalf("ExportAsset TextAsset: %v", err)
	}
	exportedBytes, err := os.ReadFile(exported)
	if err != nil {
		t.Fatalf("read exported TextAsset: %v", err)
	}

	var unpackedPath string
	for _, file := range unpacked.Files {
		if file.Kind == "TextAsset" && strings.EqualFold(filepath.Base(file.RelPath), asset.Name) {
			unpackedPath = file.AbsPath
			break
		}
	}
	if unpackedPath == "" {
		t.Fatalf("unpack produced no TextAsset named %q", asset.Name)
	}
	unpackedBytes, err := os.ReadFile(unpackedPath)
	if err != nil {
		t.Fatalf("read unpacked TextAsset: %v", err)
	}
	if !bytes.Equal(exportedBytes, unpackedBytes) {
		t.Errorf("exported TextAsset (%d bytes) differs from the unpacked one (%d bytes)", len(exportedBytes), len(unpackedBytes))
	}
}

// TestExportImageAssetMatchesConversion 检查直接导出的贴图 PNG 与"解包后再转换"得到的 PNG 一致
// 两条路径都要经过同一套解码，图像不一致说明范围读取或流式 sidecar 定位出了问题
// TestExportImageAssetMatchesConversion checks a directly exported texture PNG matches the "unpack then convert" PNG
// Both paths share the same decoder, so a mismatch would mean the range reads or stream sidecar lookup went wrong
func TestExportImageAssetMatchesConversion(t *testing.T) {
	dir := testDataDir(t)
	source := filepath.Join(dir, "cm3d2_megane002.aba")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("sample is unavailable: %v", err)
	}

	service := NewAbaExplorerService()
	overview, err := service.Inspect(source)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	sourceName, asset := findAsset(overview, "Texture2D")
	if asset == nil {
		t.Skip("this sample has no Texture2D")
	}

	workDir := t.TempDir()
	exported := filepath.Join(workDir, "direct.png")
	if err := service.ExportAsset(source, sourceName, asset.PathId, exported); err != nil {
		t.Fatalf("ExportAsset Texture2D: %v", err)
	}
	directBytes, err := os.ReadFile(exported)
	if err != nil {
		t.Fatalf("read exported PNG: %v", err)
	}
	if !bytes.HasPrefix(directBytes, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("exported file is not a PNG")
	}

	unpacked, err := service.Unpack(source, filepath.Join(workDir, "unpacked"))
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	var nativePath string
	for _, file := range unpacked.Files {
		if file.Kind == "Texture2D" && strings.HasPrefix(filepath.Base(file.RelPath), strings.TrimSuffix(asset.Name, ".tex")) {
			nativePath = file.AbsPath
			break
		}
	}
	if nativePath == "" {
		t.Fatalf("unpack produced no Texture2D for %q", asset.Name)
	}
	converted := filepath.Join(workDir, "converted.png")
	if _, err := NewConvertService().Convert(context.Background(), nativePath, targetPng, converted); err != nil {
		t.Fatalf("convert unpacked Texture2D: %v", err)
	}
	convertedBytes, err := os.ReadFile(converted)
	if err != nil {
		t.Fatalf("read converted PNG: %v", err)
	}
	if !bytes.Equal(directBytes, convertedBytes) {
		// 字节不同还有可能只是 ImageMagick 的时间戳，图像本身要按像素比
		// Differing bytes can still be nothing but ImageMagick's timestamp, so the image is compared pixel by pixel
		directBounds := pngBounds(t, directBytes, "directly exported")
		convertedBounds := pngBounds(t, convertedBytes, "converted")
		if directBounds != convertedBounds {
			t.Fatalf("direct export is %v but unpack+convert is %v", directBounds, convertedBounds)
		}
		if !bytes.Equal(decodedPixels(t, directBytes, "directly exported"), decodedPixels(t, convertedBytes, "converted")) {
			t.Errorf("direct export (%d bytes) and unpack+convert (%d bytes) decode to different pixels",
				len(directBytes), len(convertedBytes))
		}
	}
}

// TestExportAssetRejectsUnsupported 检查不支持单独导出的类型给出明确错误而不是写出半成品
// TestExportAssetRejectsUnsupported checks unsupported types fail loudly instead of writing a half-finished file
func TestExportAssetRejectsUnsupported(t *testing.T) {
	dir := testDataDir(t)
	source := filepath.Join(dir, "cm3d2_megane002.aba")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("sample is unavailable: %v", err)
	}

	service := NewAbaExplorerService()
	overview, err := service.Inspect(source)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	for _, typeName := range []string{"Mesh", "AssetBundle"} {
		sourceName, asset := findAsset(overview, typeName)
		if asset == nil {
			continue
		}
		if kind := service.AssetExportKindForType(asset.TypeId); kind != "" {
			t.Errorf("AssetExportKindForType(%s) = %q, want empty", typeName, kind)
		}
		target := filepath.Join(t.TempDir(), "should-not-exist")
		if err := service.ExportAsset(source, sourceName, asset.PathId, target); err == nil {
			t.Errorf("ExportAsset(%s) succeeded, want an error", typeName)
		}
		if _, err := os.Stat(target); err == nil {
			t.Errorf("ExportAsset(%s) wrote a file despite failing", typeName)
		}
	}

	// PathID 不存在与目标条目不存在都要被明确拒绝
	if err := service.ExportAsset(source, overview.SerializedFiles[0].Name, "1", filepath.Join(t.TempDir(), "x")); err == nil {
		t.Error("ExportAsset with an unknown PathID succeeded, want an error")
	}
	if err := service.ExportAsset(source, "no-such-entry", "1", filepath.Join(t.TempDir(), "x")); err == nil {
		t.Error("ExportAsset with an unknown SerializedFile succeeded, want an error")
	}
}
