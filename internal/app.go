package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	COM3D2Service "github.com/MeidoPromotionAssociation/MeidoSerialization/v2/service/COM3D2"
	KCESService "github.com/MeidoPromotionAssociation/MeidoSerialization/v2/service/KCES"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	// GitHubApiURL 是版本检查使用的 GitHub API 地址 / GitHubApiURL is the GitHub API endpoint used for update checks
	GitHubApiURL = "https://api.github.com/repos/MeidoPromotionAssociation/ABA_EXPLORER/releases/latest"
	// CurrentVersion 是当前应用版本 / CurrentVersion is the current application version
	CurrentVersion = "v0.1.1"
)

// maxTextFileBytes 是以文本方式读取文件的大小上限 / maxTextFileBytes caps the size of files read as text
const maxTextFileBytes int64 = 1 << 30

// maxPreviewBytes 是十六进制/文本预览读取的字节上限 / maxPreviewBytes caps the bytes read for hex and text previews
const maxPreviewBytes int64 = 1 << 20

// App 提供应用级能力：文件对话框、文件类型识别、文件信息、预览与版本检查
// App provides application-level capabilities: file dialogs, file type detection, file info, previews, and update checks
type App struct {
	app         *application.App
	fileType    *KCESService.FileTypeService
	startupFile string
}

// NewApp 创建 App 服务 / NewApp creates the App service
func NewApp() *App {
	return &App{
		fileType:    &KCESService.FileTypeService{},
		startupFile: OpenTargetFromArgs(os.Args[1:]),
	}
}

// SetApplication 注入 Wails 应用实例，对话框需要它
// 只在 main 里调用，不暴露给前端
// SetApplication injects the Wails application instance required by dialogs
// It is called from main only and is not exposed to the frontend
//
//wails:ignore
func (a *App) SetApplication(app *application.App) {
	a.app = app
}

// StartupFile 返回启动时要求打开的路径，可能来自文件关联双击或协议唤起
// StartupFile returns the path the launch asked to open, either from an association double-click or a protocol invocation
func (a *App) StartupFile() string {
	return a.startupFile
}

// SelectFile 打开文件选择对话框，filetype 形如 "*.aba;*.ct"，用户取消时返回空字符串且错误为 nil
// SelectFile opens a file-selection dialog where filetype looks like "*.aba;*.ct" and returns an empty string and nil error when cancelled
func (a *App) SelectFile(filetype string, fileDisplayName string) (string, error) {
	if a.app == nil {
		return "", errors.New("application is not initialized")
	}
	dialog := a.app.Dialog.OpenFile().
		SetTitle("Choose a file").
		AddFilter(fileDisplayName, filetype)
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		if strings.Contains(err.Error(), "by user") {
			return "", nil
		}
		return "", fmt.Errorf("open file dialog: %w", err)
	}
	return path, nil
}

// SelectPathToSave 打开保存对话框，suggestedName 预填文件名，用户取消时返回空字符串且错误为 nil
// 预填名很重要：导出对象时用户不该被要求手打 crc_nt008_line.png 这种名字
// SelectPathToSave opens a save dialog with suggestedName prefilled and returns an empty string and nil error when cancelled
// Prefilling matters: exporting an object should not ask the user to type a name like crc_nt008_line.png by hand
func (a *App) SelectPathToSave(filetype string, fileDisplayName string, suggestedName string) (string, error) {
	if a.app == nil {
		return "", errors.New("application is not initialized")
	}
	dialog := a.app.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Title:    "Save file",
		Filename: suggestedName,
	})
	dialog.AddFilter(fileDisplayName, filetype)
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		if strings.Contains(err.Error(), "by user") {
			return "", nil
		}
		return "", fmt.Errorf("open save dialog: %w", err)
	}
	return path, nil
}

// SelectDirectory 打开目录选择对话框，用户取消时返回空字符串且错误为 nil
// SelectDirectory opens a directory-selection dialog and returns an empty string and nil error when cancelled
func (a *App) SelectDirectory(title string) (string, error) {
	if a.app == nil {
		return "", errors.New("application is not initialized")
	}
	if title == "" {
		title = "Choose a directory"
	}
	dialog := a.app.Dialog.OpenFile().
		SetTitle(title).
		CanChooseDirectories(true).
		CanChooseFiles(false)
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		if strings.Contains(err.Error(), "by user") {
			return "", nil
		}
		return "", fmt.Errorf("open directory dialog: %w", err)
	}
	return path, nil
}

// DetermineFileType 判定 KCES 文件类型，无法识别时 FileType 为 Unknown 供前端按扩展名回退
// DetermineFileType detects the KCES file type and reports Unknown so the frontend can fall back to the extension
func (a *App) DetermineFileType(path string) (COM3D2Service.FileInfo, error) {
	info, matched, err := a.fileType.TryFileTypeDetermine(path)
	if err != nil {
		return info, err
	}
	if !matched {
		info.FileType = COM3D2Service.UnknownFileType
	}
	return info, nil
}

// PathInfo 是一个路径的基础属性 / PathInfo holds the basic attributes of one path
type PathInfo struct {
	Path     string `json:"path"`     // 查询的路径 / Queried path
	Exists   bool   `json:"exists"`   // 路径是否存在 / Whether the path exists
	IsDir    bool   `json:"isDir"`    // 是否为目录 / Whether the path is a directory
	Size     int64  `json:"size"`     // 文件字节数，目录为 0 / File byte count, zero for directories
	Modified string `json:"modified"` // 最后修改时间的 RFC3339 表示 / Last modification time in RFC3339
}

// StatPath 查询路径属性，不存在时 Exists 为 false 且不返回错误
// StatPath queries path attributes and reports Exists as false without an error when the path is missing
func (a *App) StatPath(path string) (PathInfo, error) {
	result := PathInfo{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, err
	}
	result.Exists = true
	result.IsDir = info.IsDir()
	if !info.IsDir() {
		result.Size = info.Size()
	}
	result.Modified = info.ModTime().Format(time.RFC3339)
	return result, nil
}

// ReadTextFile 以 UTF-8 文本读取文件并去掉 BOM / ReadTextFile reads a file as UTF-8 text and strips the BOM
func (a *App) ReadTextFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%q is not a regular file", path)
	}
	if info.Size() > maxTextFileBytes {
		return "", fmt.Errorf("file size %d exceeds limit %d", info.Size(), maxTextFileBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})), nil
}

// WriteTextFile 以 UTF-8 写入文本文件 / WriteTextFile writes a UTF-8 text file
func (a *App) WriteTextFile(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// FilePreview 是一个文件的预览结果
// 二进制内容不做十六进制转储：那对排查 KCES 资源没有帮助，需要看内部结构时应该先转换成 JSON
// FilePreview is the preview result of one file
// Binary content gets no hex dump because that does not help with KCES assets; convert to JSON to inspect the structure
type FilePreview struct {
	Path      string `json:"path"`      // 文件路径 / File path
	Size      int64  `json:"size"`      // 文件总字节数 / Total file byte count
	IsText    bool   `json:"isText"`    // 内容是否可按 UTF-8 文本展示 / Whether the content displays as UTF-8 text
	Text      string `json:"text"`      // 文本内容，IsText 为 false 时为空 / Text content, empty when IsText is false
	Truncated bool   `json:"truncated"` // 是否因超出上限被截断 / Whether the preview was truncated at the limit
}

// PreviewFile 读取文件开头用于预览，纯文本内容按 UTF-8 返回，二进制内容只报告大小
// PreviewFile reads the start of a file for preview, returning UTF-8 text for textual content and only size for binary content
func (a *App) PreviewFile(path string) (*FilePreview, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data := make([]byte, maxPreviewBytes)
	read, err := file.Read(data)
	if err != nil && read == 0 && info.Size() != 0 {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	data = data[:read]

	preview := &FilePreview{Path: path, Size: info.Size(), Truncated: info.Size() > int64(read)}
	if looksLikeText(data) {
		preview.IsText = true
		preview.Text = string(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	}
	return preview, nil
}

// looksLikeText 判断字节是否可按文本展示：无 NUL 字节、合法 UTF-8、控制字符占比低
// looksLikeText reports whether bytes display as text: no NUL bytes, valid UTF-8, and few control characters
func looksLikeText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	if !utf8.Valid(data) {
		return false
	}
	control := 0
	for _, character := range data {
		if character < 0x20 && character != '\t' && character != '\n' && character != '\r' {
			control++
		}
	}
	return control*100 <= len(data)
}

// Reveal 在系统文件管理器中定位路径，参数以数组形式传给进程，不经过 shell 解析
// Reveal locates a path in the system file manager, passing arguments as an array without shell parsing
func (a *App) Reveal(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve %q: %w", path, err)
	}
	if _, err := os.Stat(absolute); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "windows":
		// explorer 对 /select 只接受紧跟逗号的单个参数，且成功时也可能返回非零退出码
		// explorer accepts a single comma-joined argument for /select and may exit non-zero even on success
		_ = exec.Command("explorer", "/select,"+absolute).Run()
		return nil
	case "darwin":
		return exec.Command("open", "-R", absolute).Run()
	default:
		target := absolute
		if info, statErr := os.Stat(absolute); statErr == nil && !info.IsDir() {
			target = filepath.Dir(absolute)
		}
		return exec.Command("xdg-open", target).Run()
	}
}

// GetAppVersion 返回应用版本 / GetAppVersion returns the application version
func (a *App) GetAppVersion() string {
	return CurrentVersion
}

// VersionCheckResult 是版本检查结果 / VersionCheckResult is the outcome of an update check
type VersionCheckResult struct {
	CurrentVersion string `json:"currentVersion"` // 当前版本 / Current version
	LatestVersion  string `json:"latestVersion"`  // 最新发布版本 / Latest released version
	IsNewer        bool   `json:"isNewer"`        // 最新版本是否更高 / Whether the latest version is higher
}

// CheckLatestVersion 查询 GitHub 上的最新发布版本并与当前版本比较
// CheckLatestVersion queries the latest GitHub release and compares it with the current version
func (a *App) CheckLatestVersion() (VersionCheckResult, error) {
	result := VersionCheckResult{CurrentVersion: CurrentVersion}
	latest, err := fetchLatestVersion()
	if err != nil {
		return result, err
	}
	result.LatestVersion = latest
	isNewer, err := compareVersions(CurrentVersion, latest)
	if err != nil {
		return result, err
	}
	result.IsNewer = isNewer
	return result, nil
}

// fetchLatestVersion 从 GitHub API 读取最新 release 的 tag
// fetchLatestVersion reads the latest release tag from the GitHub API
func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Get(GitHubApiURL)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API request failed with status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", err
	}
	return release.TagName, nil
}

// compareVersions 返回 local 是否低于 latest / compareVersions reports whether local is lower than latest
func compareVersions(local string, latest string) (bool, error) {
	localParts, err := parseSemver(local)
	if err != nil {
		return false, fmt.Errorf("invalid local version: %w", err)
	}
	latestParts, err := parseSemver(latest)
	if err != nil {
		return false, fmt.Errorf("invalid remote version: %w", err)
	}
	for index := 0; index < 3; index++ {
		if latestParts[index] != localParts[index] {
			return latestParts[index] > localParts[index], nil
		}
	}
	return false, nil
}

// parseSemver 解析 v1.2.3 形式的版本号 / parseSemver parses a version of the form v1.2.3
func parseSemver(version string) ([3]int, error) {
	var result [3]int
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(strings.TrimPrefix(version, "v"), "V")
	if index := strings.IndexAny(version, "-+"); index >= 0 {
		version = version[:index]
	}
	parts := strings.Split(version, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return result, fmt.Errorf("unrecognized version %q", version)
	}
	for index, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return result, fmt.Errorf("unrecognized version %q: %w", version, err)
		}
		result[index] = value
	}
	return result, nil
}
