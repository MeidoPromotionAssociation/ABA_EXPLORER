package internal

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// testDataDir 定位相邻 MeidoSerialization 仓库的 KCES 测试样本
// 样本不在仓库内，找不到时测试跳过而不是失败，这样单独 clone 本项目也能跑测试
// testDataDir locates the KCES samples in the neighbouring MeidoSerialization checkout
// The samples live outside this repository, so tests skip instead of failing when the directory is absent
func testDataDir(t *testing.T) string {
	t.Helper()
	candidate := filepath.Join("..", "..", "MeidoSerialization", "testdata", "KCES")
	info, err := os.Stat(candidate)
	if err != nil || !info.IsDir() {
		t.Skipf("KCES sample directory %q is unavailable", candidate)
	}
	absolute, err := filepath.Abs(candidate)
	if err != nil {
		t.Fatalf("resolve sample directory: %v", err)
	}
	return absolute
}

// TestInspectContainerSamples 对多个真实 ABA 样本检查结构快照的自洽性
// TestInspectContainerSamples checks snapshot self-consistency across several real ABA samples
func TestInspectContainerSamples(t *testing.T) {
	dir := testDataDir(t)
	service := NewAbaExplorerService()

	samples := []string{
		"cm3d2_megane002.aba",
		"parts_bv001.aba",
		"motion.aba",
		"csv.aba",
		"language.aba",
		"bg.aba",
	}

	for _, sample := range samples {
		t.Run(sample, func(t *testing.T) {
			path := filepath.Join(dir, sample)
			if _, err := os.Stat(path); err != nil {
				t.Skipf("sample %q is unavailable", sample)
			}

			overview, err := service.Inspect(path)
			if err != nil {
				t.Fatalf("Inspect(%q): %v", sample, err)
			}
			if overview.Signature != "UnityFS" {
				t.Errorf("signature = %q, want UnityFS", overview.Signature)
			}
			if overview.FormatVersion < 6 || overview.FormatVersion > 8 {
				t.Errorf("format version = %d, want 6..8", overview.FormatVersion)
			}
			if overview.FileSize <= 0 {
				t.Errorf("file size = %d, want positive", overview.FileSize)
			}
			if len(overview.Blocks) == 0 {
				t.Error("no data blocks reported")
			}
			if len(overview.Directories) == 0 {
				t.Error("no directory entries reported")
			}
			if len(overview.Hash) != 32 {
				t.Errorf("hash hex length = %d, want 32", len(overview.Hash))
			}

			// 每个序列化条目都必须出现在目录列表里，且对象总数与拉平后的数量一致
			// Every serialized entry must appear in the directory list and the object total must match the flattened count
			serializedCount := 0
			for _, dirEntry := range overview.Directories {
				if dirEntry.Serialized {
					serializedCount++
				}
			}
			if serializedCount != len(overview.SerializedFiles) {
				t.Errorf("serialized directories = %d, serialized files = %d", serializedCount, len(overview.SerializedFiles))
			}

			total := 0
			for _, file := range overview.SerializedFiles {
				total += len(file.Assets)
				if file.UnityVersion == "" {
					t.Errorf("serialized file %q reports an empty Unity version", file.Name)
				}
				for _, asset := range file.Assets {
					if _, err := strconv.ParseInt(asset.PathId, 10, 64); err != nil {
						t.Errorf("asset PathID %q is not a decimal int64: %v", asset.PathId, err)
					}
				}
			}
			if total != overview.AssetCount {
				t.Errorf("asset count = %d, flattened = %d", overview.AssetCount, total)
			}
		})
	}
}

// TestInspectCtSamples 对真实 .ct 样本检查 catalog 与虚拟文件表的一致性
// TestInspectCtSamples checks catalog and virtual-file consistency across real .ct samples
func TestInspectCtSamples(t *testing.T) {
	dir := testDataDir(t)
	service := NewCtExplorerService()

	samples := []string{
		"cm3d2_megane002.ct",
		"parts_bv001.ct",
		"motion.ct",
		"csv.ct",
		"language.ct",
		"bg.ct",
	}

	for _, sample := range samples {
		t.Run(sample, func(t *testing.T) {
			path := filepath.Join(dir, sample)
			if _, err := os.Stat(path); err != nil {
				t.Skipf("sample %q is unavailable", sample)
			}

			overview, err := service.Inspect(path)
			if err != nil {
				t.Fatalf("Inspect(%q): %v", sample, err)
			}
			if overview.FileSize <= 0 {
				t.Errorf("file size = %d, want positive", overview.FileSize)
			}
			if overview.Framing != "legacy" && overview.Framing != "extended" {
				t.Errorf("framing = %q, want legacy or extended", overview.Framing)
			}
			if len(overview.Files) == 0 {
				t.Fatal("no virtual files reported")
			}
			// catalog 是游戏读取资源索引的入口，样本里必须存在
			// The catalog is how the game reads the resource index, so samples must contain it
			hasCatalog := false
			for _, file := range overview.Files {
				if file.Name == "catalog" {
					hasCatalog = true
				}
				if file.Position <= 0 || file.Size < 0 {
					t.Errorf("virtual file %q has position %d and size %d", file.Name, file.Position, file.Size)
				}
			}
			if !hasCatalog {
				t.Error("no catalog virtual file reported")
			}

			if overview.CatalogError != "" {
				t.Fatalf("catalog decode failed: %s", overview.CatalogError)
			}
			catalog := overview.Catalog
			if catalog == nil {
				t.Fatal("catalog is nil while CatalogError is empty")
			}
			if catalog.Kind != "assetBundle" && catalog.Kind != "virtualAsset" {
				t.Errorf("catalog kind = %q", catalog.Kind)
			}
			if _, err := strconv.ParseUint(catalog.Hash, 10, 64); err != nil {
				t.Errorf("catalog hash %q is not a decimal uint64: %v", catalog.Hash, err)
			}
			if len(catalog.CatalogTypeNames) == 0 {
				t.Error("catalog type flags decoded to no names")
			}
			for _, item := range catalog.Items {
				if _, err := strconv.ParseUint(item.Hash, 10, 64); err != nil {
					t.Errorf("catalog item hash %q is not a decimal uint64: %v", item.Hash, err)
				}
			}
			// 每个 ExtensionNameList 分组都应有对应的虚拟文件
			// Every ExtensionNameList group should have a matching virtual file
			for _, list := range overview.Extensions {
				found := false
				for _, file := range overview.Files {
					if file.Name == list.Key {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("extension list %q has no matching virtual file", list.Key)
				}
			}
		})
	}
}

// TestUnpackPackRoundTrip 解包一个真实 ABA 再打包回去，并比较两次结构快照
// TestUnpackPackRoundTrip unpacks a real ABA, packs it back, and compares both structural snapshots
func TestUnpackPackRoundTrip(t *testing.T) {
	dir := testDataDir(t)
	source := filepath.Join(dir, "cm3d2_megane002.aba")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("sample is unavailable: %v", err)
	}

	service := NewAbaExplorerService()
	original, err := service.Inspect(source)
	if err != nil {
		t.Fatalf("Inspect source: %v", err)
	}

	workDir := t.TempDir()
	unpackDir := filepath.Join(workDir, "cm3d2_megane002.aba_unpacked")
	unpacked, err := service.Unpack(source, unpackDir)
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	if len(unpacked.Files) == 0 {
		t.Fatal("unpack produced no files")
	}
	for _, file := range unpacked.Files {
		if file.RelPath == "" || file.AbsPath == "" {
			t.Errorf("unpacked file has empty paths: %+v", file)
		}
		if file.Size < 0 {
			t.Errorf("unpacked file %q reports size %d", file.RelPath, file.Size)
		}
	}

	// 解包目录名带 .aba_unpacked 后缀，DefaultPackName 应把后缀与容器扩展名都剥掉
	// The directory ends with .aba_unpacked, so DefaultPackName must strip both the suffix and the container extension
	if name := service.DefaultPackName(unpackDir); name != "cm3d2_megane002" {
		t.Errorf("DefaultPackName = %q, want cm3d2_megane002", name)
	}

	packed, err := service.Pack(unpackDir, "roundtrip")
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if _, err := os.Stat(packed.AbaPath); err != nil {
		t.Fatalf("packed ABA missing: %v", err)
	}
	// 打包同时写出配套 .ct，两者都落在解包目录的父目录
	// Packing also writes the companion .ct, and both land in the parent of the unpacked directory
	companionCt := filepath.Join(workDir, "roundtrip.ct")
	if packed.CtPath != companionCt {
		t.Errorf("companion .ct path = %q, want %q", packed.CtPath, companionCt)
	}
	if _, err := os.Stat(companionCt); err != nil {
		t.Fatalf("companion .ct missing: %v", err)
	}
	// 基名与内部 .menuassets 容器名不一致，打包器应报告 KCES 1.34.5 的加载警告
	// The base name does not match the inner .menuassets container, so the packer must report the KCES 1.34.5 loading warning
	if len(packed.Warnings) == 0 {
		t.Error("expected packer warnings for the renamed bundle, got none")
	}

	repacked, err := service.Inspect(packed.AbaPath)
	if err != nil {
		t.Fatalf("Inspect packed: %v", err)
	}
	if repacked.Signature != "UnityFS" {
		t.Errorf("packed signature = %q", repacked.Signature)
	}
	if repacked.AssetCount != original.AssetCount {
		t.Errorf("packed asset count = %d, original = %d", repacked.AssetCount, original.AssetCount)
	}

	ctOverview, err := NewCtExplorerService().Inspect(companionCt)
	if err != nil {
		t.Fatalf("Inspect companion .ct: %v", err)
	}
	if ctOverview.CatalogError != "" {
		t.Fatalf("companion catalog decode failed: %s", ctOverview.CatalogError)
	}
	if ctOverview.Catalog == nil || ctOverview.Catalog.Name != "roundtrip" {
		t.Errorf("companion catalog name = %v, want roundtrip", ctOverview.Catalog)
	}
}

// TestConvertUnpackedAssets 检查解包产物的可用转换目标，并真正执行 PNG 与 JSON 转换
// TestConvertUnpackedAssets checks the conversion targets of unpacked assets and actually runs the PNG and JSON conversions
func TestConvertUnpackedAssets(t *testing.T) {
	dir := testDataDir(t)
	source := filepath.Join(dir, "cm3d2_megane002.aba")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("sample is unavailable: %v", err)
	}

	workDir := t.TempDir()
	unpacked, err := NewAbaExplorerService().Unpack(source, filepath.Join(workDir, "unpacked"))
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	service := NewConvertService()
	ctx := context.Background()
	convertible := 0
	var texture, structured string

	for _, file := range unpacked.Files {
		targets := service.Targets(file.AbsPath)
		if len(targets) == 0 {
			continue
		}
		convertible++
		keys := make(map[string]bool, len(targets))
		for _, target := range targets {
			keys[target.Key] = true
		}
		if file.Kind == "Texture2D" {
			if !keys[targetPng] {
				t.Errorf("Texture2D %q has no png target, got %v", file.RelPath, keys)
			}
			texture = file.AbsPath
		}
		if filepath.Ext(file.RelPath) == ".menuassets" || filepath.Ext(file.RelPath) == ".materialassets" {
			if !keys[targetJson] {
				t.Errorf("%q has no json target, got %v", file.RelPath, keys)
			}
			structured = file.AbsPath
		}
	}
	if convertible == 0 {
		t.Fatal("no unpacked file reported a conversion target")
	}

	if texture != "" {
		written, err := service.Convert(ctx, texture, targetPng, "")
		if err != nil {
			t.Fatalf("convert %q to png: %v", texture, err)
		}
		if info, err := os.Stat(written); err != nil || info.Size() == 0 {
			t.Fatalf("png output %q is missing or empty: %v", written, err)
		}
	}

	if structured != "" {
		written, err := service.Convert(ctx, structured, targetJson, "")
		if err != nil {
			t.Fatalf("convert %q to json: %v", structured, err)
		}
		if info, err := os.Stat(written); err != nil || info.Size() == 0 {
			t.Fatalf("json output %q is missing or empty: %v", written, err)
		}
	}

	// 批量转换对不支持的目标要逐条报告错误，而不是中断整批
	// A batch conversion must report per-file errors for an unsupported target instead of aborting the whole batch
	paths := make([]string, 0, 2)
	for _, file := range unpacked.Files {
		paths = append(paths, file.AbsPath)
		if len(paths) == 2 {
			break
		}
	}
	outcomes, err := service.ConvertBatch(ctx, paths, targetCsv, filepath.Join(workDir, "converted"))
	if err != nil {
		t.Fatalf("ConvertBatch: %v", err)
	}
	if len(outcomes) != len(paths) {
		t.Fatalf("outcomes = %d, want %d", len(outcomes), len(paths))
	}
	for _, outcome := range outcomes {
		if outcome.Error == "" && outcome.OutputPath == "" {
			t.Errorf("outcome for %q reports neither error nor output", outcome.InputPath)
		}
	}
}

// TestPreviewImage 检查原生纹理能解码成可直接嵌入 webview 的 PNG data URL
// TestPreviewImage checks that a native texture decodes into a PNG data URL the webview can embed directly
func TestPreviewImage(t *testing.T) {
	dir := testDataDir(t)
	source := filepath.Join(dir, "cm3d2_megane002.aba")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("sample is unavailable: %v", err)
	}

	unpacked, err := NewAbaExplorerService().Unpack(source, filepath.Join(t.TempDir(), "unpacked"))
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	service := NewConvertService()
	previewed := 0
	for _, file := range unpacked.Files {
		if !service.CanPreviewImage(file.AbsPath) {
			continue
		}
		dataURL, err := service.PreviewImage(context.Background(), file.AbsPath)
		if err != nil {
			t.Fatalf("PreviewImage(%q): %v", file.RelPath, err)
		}
		const prefix = "data:image/png;base64,"
		if len(dataURL) <= len(prefix) || dataURL[:len(prefix)] != prefix {
			t.Fatalf("preview of %q is not a PNG data URL", file.RelPath)
		}
		previewed++
	}
	if previewed == 0 {
		t.Skip("this sample contains no previewable texture")
	}
}
