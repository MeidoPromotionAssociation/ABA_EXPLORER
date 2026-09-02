package internal

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ProtocolScheme 是外部工具唤起本程序使用的 URL scheme
// 完整形式为 aba-explorer://open?path=<URL 编码的绝对路径>，由安装器写入注册表
// ProtocolScheme is the URL scheme external tools use to invoke this application
// The full form is aba-explorer://open?path=<URL-encoded absolute path> and the installer writes the registration
const ProtocolScheme = "aba-explorer"

// ProtocolOpenEvent 是收到打开请求后发给前端的事件名
// 单实例转交过来的文件关联双击也走这个事件：对前端来说两者都是"另一次启动要求打开这个路径"
// ProtocolOpenEvent is the event emitted to the frontend after an open request
// Association double-clicks handed over by single instance use it too, since to the frontend both mean
// "another launch asked to open this path"
const ProtocolOpenEvent = "explorer:protocol-open"

// protocolOpenableExtensions 是协议允许打开的扩展名，与前端 utils/consts.ts 的 AnyKcesFilter 对应
// 协议 URL 可以由任意程序甚至网页触发，没有白名单就等于把"打开任意本地文件并显示内容"暴露出去，
// 因此这里只放本程序真正有浏览页面的容器与内容表格式
// 目录也故意不接受：拖放解包目录是本地操作，而让一个网页能要求程序去枚举任意目录是完全另一回事
// protocolOpenableExtensions lists the extensions the protocol may open, mirroring AnyKcesFilter in utils/consts.ts
// Any program or even a web page can trigger a protocol URL, so without an allow list this would expose
// "open and display an arbitrary local file"; only the container and catalog formats with a real browser page belong here
// Directories are deliberately refused as well: dropping an unpacked folder is a local action, while letting a web page
// ask the application to enumerate an arbitrary directory is something else entirely
var protocolOpenableExtensions = map[string]struct{}{
	".aba":         {},
	".asset_bg":    {},
	".asset_scene": {},
	".ct":          {},
}

// ParseProtocolURL 从一个命令行参数中解出协议请求指向的文件路径
// 不是协议 URL、缺少 path、路径不是允许的扩展名或文件不存在时都返回空字符串
// ParseProtocolURL extracts the file path a protocol request points at from one command-line argument
// It returns an empty string when the argument is not a protocol URL, lacks path, has a disallowed extension, or is missing
func ParseProtocolURL(argument string) string {
	if !strings.HasPrefix(strings.ToLower(argument), ProtocolScheme+":") {
		return ""
	}
	parsed, err := url.Parse(argument)
	if err != nil {
		return ""
	}
	candidate := strings.TrimSpace(parsed.Query().Get("path"))
	if candidate == "" {
		return ""
	}
	// 相对路径没有意义：协议由系统转发，本程序的工作目录和发起方毫无关系
	// A relative path is meaningless because the system forwards the protocol and our working directory
	// has nothing to do with the caller's
	if !filepath.IsAbs(candidate) {
		return ""
	}
	if !isProtocolOpenableExtension(candidate) {
		return ""
	}
	info, err := os.Stat(candidate)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return candidate
}

// isProtocolOpenableExtension 判断路径的扩展名是否在协议白名单里
// isProtocolOpenableExtension reports whether a path's extension is on the protocol allow list
func isProtocolOpenableExtension(path string) bool {
	lower := strings.ToLower(filepath.Base(path))
	for extension := range protocolOpenableExtensions {
		// 要求扩展名前面还有文件名，避免把 ".ct" 这种纯扩展名文件当成合法输入
		// The extension still needs a stem, so a file named just ".ct" is not accepted
		if strings.HasSuffix(lower, extension) && len(lower) > len(extension) {
			return true
		}
	}
	return false
}

// ProtocolFileFromArgs 扫描命令行参数并返回第一个有效的协议目标路径
// ProtocolFileFromArgs scans command-line arguments and returns the first valid protocol target
func ProtocolFileFromArgs(args []string) string {
	for _, argument := range args {
		if path := ParseProtocolURL(argument); path != "" {
			return path
		}
	}
	return ""
}

// OpenTargetFromArgs 从一组不含程序名的命令行参数中判断该打开哪个路径
// 先按协议 URL 解析，没有再取第一个看起来是本地路径的参数（文件关联双击与拖到图标上走这条，
// 因此这条分支不限扩展名也接受目录，它的来源是用户的本机操作而不是外部 URL）
// OpenTargetFromArgs decides which path a set of command-line arguments asks to open, program name excluded
// Protocol URLs come first, then the first argument that looks like a local path; that is how association
// double-clicks and drops onto the icon arrive, so this branch accepts any extension and directories too
// because it originates from the user's own machine rather than an external URL
func OpenTargetFromArgs(args []string) string {
	if path := ProtocolFileFromArgs(args); path != "" {
		return path
	}
	for _, argument := range args {
		if argument == "" || strings.HasPrefix(argument, "-") || strings.Contains(argument, "://") {
			continue
		}
		return argument
	}
	return ""
}

// SecondInstanceTarget 从另一个实例的启动参数里判断该打开哪个路径
// SecondInstanceData.Args 直接来自那个进程的 os.Args，第一个元素是可执行文件自身，必须先去掉：
// 否则文件关联双击唤起时，第一个非选项参数就是 exe 的路径，会被当成用户想打开的文件
// SecondInstanceTarget decides which path another instance's launch arguments ask to open
// SecondInstanceData.Args is that process's os.Args verbatim and its first element is the executable itself,
// which has to be dropped: otherwise on an association double-click the first non-flag argument is the exe path
// and would be taken for the file the user wanted to open
func SecondInstanceTarget(args []string) string {
	if len(args) > 0 {
		args = args[1:]
	}
	return OpenTargetFromArgs(args)
}

// ProtocolStatus 是自定义协议的当前状态，设置页用它说明协议现在能不能用
// ProtocolStatus is the current state of the custom protocol so the settings page can explain whether it works
type ProtocolStatus struct {
	Scheme     string `json:"scheme"`     // 协议名，不含 :// / Scheme name without ://
	Registered bool   `json:"registered"` // 系统上是否注册过 / Whether it is registered on this system
}

// ProtocolStatus 返回协议名与注册状态
// 协议由安装器写入，直接解压的绿色版不会注册，此时外部工具唤起不会有任何反应，需要在设置页说清楚
// ProtocolStatus returns the scheme name and its registration state
// The installer writes the registration, so a portable copy has none and an external invocation would silently
// do nothing, which the settings page has to spell out
func (a *App) ProtocolStatus() ProtocolStatus {
	return ProtocolStatus{Scheme: ProtocolScheme, Registered: protocolRegistered(ProtocolScheme)}
}
