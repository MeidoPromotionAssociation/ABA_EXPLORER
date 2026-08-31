package internal

import (
	"net/url"
	"strings"
	"testing"
)

// TestModEditorOpenURLRoundTrip 检查这边拼出的协议 URL 能被编辑器那侧的解析方式还原成原路径
// 编辑器用 url.Parse + Query().Get("path") 取值，这里用同样的方式往返，等价于跨进程对接测试
// TestModEditorOpenURLRoundTrip checks the protocol URL built here survives the parsing the editor side performs
// The editor reads it with url.Parse + Query().Get("path"), so round-tripping the same way is equivalent to an integration check
func TestModEditorOpenURLRoundTrip(t *testing.T) {
	paths := []string{
		`D:\Code\mods\parts.menuassets`,
		`D:\Code\my mods\with space\body.model`,
		`D:\Code\日本語のフォルダ\部品.materialassets`,
		`D:\Code\mods\hand.ikcol.bytes`,
		`D:\Code\mods\a&b=c\odd#name.preset`,
		`/home/user/mods/parts.menuassets`,
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			target := modEditorOpenURL(path)
			if !strings.HasPrefix(target, modEditorScheme+"://open?path=") {
				t.Fatalf("URL %q does not use the agreed shape", target)
			}
			parsed, err := url.Parse(target)
			if err != nil {
				t.Fatalf("parse %q: %v", target, err)
			}
			if parsed.Scheme != modEditorScheme {
				t.Errorf("scheme = %q, want %q", parsed.Scheme, modEditorScheme)
			}
			if got := parsed.Query().Get("path"); got != path {
				t.Errorf("round-tripped path = %q, want %q", got, path)
			}
		})
	}
}

// TestCanOpenInModEditor 检查按钮的可用性判断只对编辑器真正支持的格式为真
// TestCanOpenInModEditor checks the button-enablement test only accepts formats the editor really supports
func TestCanOpenInModEditor(t *testing.T) {
	app := &App{}

	editable := []string{
		"parts.menuassets", "parts.materialassets", "parts.pmatassets", "body.model",
		"skirt.dsbconf", "hand.ikcol.bytes", "arm.limbcol",
		"girl.preset", "girl.perset", "attach.sad", "check.hitcheck",
		"data.nson", "undress.undressdat", "parts.undresspdat", "table.psk", "table.nei",
	}
	for _, name := range editable {
		if !app.CanOpenInModEditor(`D:\mods\` + name) {
			t.Errorf("CanOpenInModEditor(%q) = false, want true", name)
		}
	}

	// 解包产物里的原始 Unity 对象与图片没有编辑页面，裸 .bytes 也不能当成 maid_collider
	rejected := []string{
		"texture.texture2d", "mesh.mmesh", "clip.anm", "sprite.sprite",
		"object.bytes", "image.png", "sound.wav", "notes.txt", "app.exe",
		"container.aba", "catalog.ct", ".menuassets",
	}
	for _, name := range rejected {
		if app.CanOpenInModEditor(`D:\mods\` + name) {
			t.Errorf("CanOpenInModEditor(%q) = true, want false", name)
		}
	}
}
