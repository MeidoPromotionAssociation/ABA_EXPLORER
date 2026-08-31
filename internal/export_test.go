package internal

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
		t.Errorf("direct export (%d bytes) differs from unpack+convert (%d bytes)", len(directBytes), len(convertedBytes))
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
