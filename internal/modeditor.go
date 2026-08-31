package internal

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// modEditorScheme 是 KCES MOD EDITOR 注册的自定义 URL 协议
// 请求形式为 kces-mod-editor://open?path=<URL 编码的绝对路径>，由安装器写入注册表
// modEditorScheme is the custom URL protocol KCES MOD EDITOR registers
// A request looks like kces-mod-editor://open?path=<URL-encoded absolute path> and the installer writes the registry entry
const modEditorScheme = "kces-mod-editor"

// modEditorExtensions 是 KCES MOD EDITOR 有编辑页面的格式，与那边 internal/protocol.go 的白名单一一对应
// 两侧各存一份是跨进程协议的代价：这边的清单只决定按钮是否可点，真正的准入判断在编辑器那侧
// modEditorExtensions lists the formats KCES MOD EDITOR has an editor page for, mirroring the allow list in its internal/protocol.go
// Keeping a copy on each side is the cost of a cross-process protocol: this list only decides whether the button is enabled,
// while the actual admission check lives in the editor
var modEditorExtensions = []string{
	// 服装部件 / Parts
	".menuassets", ".materialassets", ".pmatassets", ".model",
	// 物理 / Physics
	".dbconf", ".dbcol", ".db2conf", ".dsbconf", ".dsb2conf", ".dslconf", ".dsl2conf",
	".dslcol", ".ikcol", ".ikcol.bytes", ".limbcol",
	// 角色 / Character
	".preset", ".perset", ".sad", ".hitcheck",
	// 数据 / Data
	".nson", ".undressdat", ".undresspdat", ".psk", ".nei", ".csv",
}

// CanOpenInModEditor 判断文件是否为 KCES MOD EDITOR 能编辑的格式
// CanOpenInModEditor reports whether a file is in a format KCES MOD EDITOR can edit
func (a *App) CanOpenInModEditor(path string) bool {
	lower := strings.ToLower(filepath.Base(path))
	for _, extension := range modEditorExtensions {
		// 复合扩展名如 .ikcol.bytes 要求前面还有文件名，避免把纯扩展名文件当成合法输入
		if strings.HasSuffix(lower, extension) && len(lower) > len(extension) {
			return true
		}
	}
	return false
}

// IsModEditorAvailable 判断系统上是否注册过 KCES MOD EDITOR 的协议
// 协议由安装器写入，直接解压的绿色版不会注册，此时按钮点了不会有任何反应，需要提前告诉用户
// IsModEditorAvailable reports whether the KCES MOD EDITOR protocol is registered on this system
// The installer writes the registration, so a portable copy has none and the button would silently do nothing
func (a *App) IsModEditorAvailable() bool {
	return modEditorProtocolRegistered(modEditorScheme)
}

// modEditorOpenURL 拼出请求编辑器打开某个绝对路径的协议 URL
// 路径经 QueryEscape 编码，因此 Windows 的反斜杠、空格与中文都能安全穿过命令行
// modEditorOpenURL builds the protocol URL that asks the editor to open one absolute path
// The path is QueryEscape'd so Windows backslashes, spaces, and CJK characters survive the command line
func modEditorOpenURL(absolutePath string) string {
	return modEditorScheme + "://open?path=" + url.QueryEscape(absolutePath)
}

// OpenInModEditor 通过自定义协议请求 KCES MOD EDITOR 打开一个文件
// 编辑器已在运行时由它的单实例机制转交给现有窗口，不会每次都启新进程
// OpenInModEditor asks KCES MOD EDITOR to open a file through the custom protocol
// A running editor receives it through its single-instance handover instead of starting another process
func (a *App) OpenInModEditor(path string) error {
	if a.app == nil {
		return errors.New("application is not initialized")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve %q: %w", path, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q is not a regular file", absolute)
	}
	if !a.CanOpenInModEditor(absolute) {
		return fmt.Errorf("KCES MOD EDITOR has no editor for %s files", strings.ToLower(filepath.Ext(absolute)))
	}
	return a.app.Browser.OpenURL(modEditorOpenURL(absolute))
}
