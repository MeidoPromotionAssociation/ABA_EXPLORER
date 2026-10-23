package internal

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	serializationCRC3D3 "github.com/MeidoPromotionAssociation/MeidoSerialization/v2/serialization/CRC3D3"
	"github.com/MeidoPromotionAssociation/MeidoSerialization/v2/serialization/KCES/msgpack"
	CRC3D3Service "github.com/MeidoPromotionAssociation/MeidoSerialization/v2/service/CRC3D3"
)

// sampleNeimpTable 构造一张与游戏表格同形的 .neimp 值树：字符串键的映射，值是行数组
// 值树里同时放了字符串、无符号整数与 float32，覆盖 JSON 转换需要保留的几种类型
// sampleNeimpTable builds a .neimp value tree shaped like an in-game table: a map with string keys whose values are row arrays
// The tree carries strings, unsigned integers, and float32 so the JSON conversion has the kinds it must preserve
func sampleNeimpTable() serializationCRC3D3.Value {
	row := func(cells ...string) serializationCRC3D3.Value {
		values := make([]serializationCRC3D3.Value, 0, len(cells))
		for _, cell := range cells {
			values = append(values, serializationCRC3D3.Value{Kind: msgpack.KindString, Str: cell})
		}
		return serializationCRC3D3.Value{Kind: msgpack.KindArray, Array: values}
	}
	return serializationCRC3D3.Value{
		Kind: msgpack.KindMap,
		Map: []msgpack.MapEntry{
			{Key: serializationCRC3D3.Value{Kind: msgpack.KindString, Str: "version"}, Value: serializationCRC3D3.Value{Kind: msgpack.KindUint, Uint: 3}},
			{Key: serializationCRC3D3.Value{Kind: msgpack.KindString, Str: "scale"}, Value: serializationCRC3D3.Value{Kind: msgpack.KindFloat32, Float32: 1.5}},
			{
				Key: serializationCRC3D3.Value{Kind: msgpack.KindString, Str: "rows"},
				Value: serializationCRC3D3.Value{
					Kind: msgpack.KindArray,
					Array: []serializationCRC3D3.Value{
						row("name", "slot", "price"),
						row("maid_01", "1", "100"),
					},
				},
			},
		},
	}
}

// writeNeimpSample 把值树写成临时目录里的 .neimp 文件并返回其路径
// writeNeimpSample writes a value tree as a .neimp file in a temporary directory and returns its path
func writeNeimpSample(t *testing.T, name string, value serializationCRC3D3.Value) string {
	t.Helper()
	encoded, err := serializationCRC3D3.EncodeNeimp(value)
	if err != nil {
		t.Fatalf("encode the %s sample: %v", name, err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, encoded, 0644); err != nil {
		t.Fatalf("write the %s sample: %v", name, err)
	}
	return path
}

// TestNeimpTargets 检查 .neimp 只给出 JSON 目标，不会串到 .nei 的 CSV 目标上
// TestNeimpTargets checks .neimp offers the JSON target only and never leaks into the CSV target of .nei
func TestNeimpTargets(t *testing.T) {
	service := NewConvertService()

	targets := service.Targets(writeNeimpSample(t, "table.neimp", sampleNeimpTable()))
	if len(targets) != 1 || targets[0].Key != targetJson || targets[0].Ext != ".json" {
		t.Fatalf("Targets(.neimp) = %+v, want a single json target", targets)
	}

	// .nei 走 CSV，.neimp 走 JSON，两者只差一个字母但不能互相误判
	// .nei goes to CSV and .neimp goes to JSON, so a one-letter difference must not cross over
	neiTargets := service.Targets(filepath.Join(t.TempDir(), "table.nei"))
	if len(neiTargets) != 1 || neiTargets[0].Key != targetCsv {
		t.Errorf("Targets(.nei) = %+v, want a single csv target", neiTargets)
	}
}

// TestNeimpConvertToJson 检查 .neimp 转出的 JSON 落在 .neimp.json 上，且解析回来与原值树一致
// TestNeimpConvertToJson checks the JSON of a .neimp lands on .neimp.json and parses back into the original value tree
func TestNeimpConvertToJson(t *testing.T) {
	path := writeNeimpSample(t, "table.neimp", sampleNeimpTable())

	service := NewConvertService()
	outPath, err := service.Convert(context.Background(), path, targetJson, "")
	if err != nil {
		t.Fatalf("Convert(%q, json): %v", path, err)
	}
	// JSON 追加在原生扩展名之后，与 MeidoSerialization 的 .neimp.json 复合后缀一致
	// The JSON is appended after the native extension, matching MeidoSerialization's .neimp.json compound suffix
	if want := path + ".json"; outPath != want {
		t.Errorf("Convert output = %q, want %q", outPath, want)
	}

	written, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read the converted JSON: %v", err)
	}
	decoded, err := serializationCRC3D3.DecodeNeimpJSON(written)
	if err != nil {
		t.Fatalf("decode the converted JSON: %v", err)
	}
	// 解码会把映射键排序，所以比较基准取「原生文件自己解出来的值树」而不是手工构造的那份，
	// 两边再走同一个 JSON 编码器即可覆盖类型宽度与内容
	// Decoding sorts the map keys, so the baseline is the tree decoded from the native file rather than the
	// hand-built one, and running both sides through the same JSON encoder covers kinds, widths, and content
	native, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the native sample: %v", err)
	}
	baseline, err := serializationCRC3D3.DecodeNeimp(native)
	if err != nil {
		t.Fatalf("decode the native sample: %v", err)
	}
	wantJSON, err := serializationCRC3D3.EncodeNeimpJSON(baseline)
	if err != nil {
		t.Fatalf("encode the baseline as JSON: %v", err)
	}
	gotJSON, err := serializationCRC3D3.EncodeNeimpJSON(decoded)
	if err != nil {
		t.Fatalf("re-encode the decoded JSON: %v", err)
	}
	if !bytes.Equal(wantJSON, gotJSON) {
		t.Errorf("round-tripped JSON = %s, want %s", gotJSON, wantJSON)
	}
}

// TestNeimpConvertBatch 检查批量转换按目标目录写出，并把 .neimp.json 的复合后缀拼完整
// TestNeimpConvertBatch checks a batch conversion writes into the target directory and keeps the .neimp.json compound suffix whole
func TestNeimpConvertBatch(t *testing.T) {
	path := writeNeimpSample(t, "table.neimp", sampleNeimpTable())
	outDir := t.TempDir()

	outcomes, err := NewConvertService().ConvertBatch(context.Background(), []string{path}, targetJson, outDir)
	if err != nil {
		t.Fatalf("ConvertBatch: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("ConvertBatch returned %d outcomes, want 1", len(outcomes))
	}
	if outcomes[0].Error != "" {
		t.Fatalf("ConvertBatch(%q) failed: %s", path, outcomes[0].Error)
	}
	want := filepath.Join(outDir, "table.neimp.json")
	if outcomes[0].OutputPath != want {
		t.Errorf("ConvertBatch output = %q, want %q", outcomes[0].OutputPath, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("stat the batch output: %v", err)
	}
}

// TestNeimpJSONDetection 检查 .neimp.json 不会被当成原生 .neimp 输入
// TestNeimpJSONDetection checks .neimp.json is not taken for a native .neimp input
func TestNeimpJSONDetection(t *testing.T) {
	if CRC3D3Service.IsCRC3D3NeimpFile("table.neimp.json") {
		t.Error("IsCRC3D3NeimpFile(.neimp.json) = true, want false")
	}
	if !CRC3D3Service.IsCRC3D3NeimpJSONFile("table.neimp.json") {
		t.Error("IsCRC3D3NeimpJSONFile(.neimp.json) = false, want true")
	}
	if supportsJsonConversion("table.neimp.json") {
		t.Error("supportsJsonConversion(.neimp.json) = true, want false, a JSON form is not a conversion input")
	}
}
