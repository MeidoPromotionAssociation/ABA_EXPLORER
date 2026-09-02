package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Settings 是必须在应用启动前就读到的配置
// 单实例要在 application.New 时就决定，那时前端还没起来、localStorage 读不到，所以这类设置只能存文件
// Settings holds configuration that must be known before the application starts
// Single instance is decided at application.New, before the frontend exists and localStorage is reachable,
// so settings of this kind have to live in a file
type Settings struct {
	// SingleInstance 为 true 时只允许一个实例运行，协议唤起与文件关联都转交给已有窗口
	// 关闭后每次唤起都会开新窗口，冷启动仍会打开目标文件
	// When true only one instance runs and protocol or association opens are handed to the existing window
	// When off each invocation opens another window, which still opens the target file on the cold-start path
	SingleInstance bool `json:"singleInstance"`
}

// DefaultSettings 返回默认配置
// 单实例默认关闭：本程序是浏览器式的工具，并排开两个窗口对比两个容器是常见用法，
// 默认转交给已有窗口会让第二次双击变成覆盖当前视图
// DefaultSettings returns the default configuration
// Single instance defaults to off because this is a browsing tool and comparing two containers in two windows
// is a normal thing to do; handing over by default would turn the second double-click into replacing the current view
func DefaultSettings() Settings {
	return Settings{SingleInstance: false}
}

// settingsPath 返回配置文件路径，位于用户配置目录下的应用子目录
// settingsPath returns the settings file path inside the application folder of the user config directory
func settingsPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate the user config directory: %w", err)
	}
	return filepath.Join(base, "ABA_EXPLORER", "settings.json"), nil
}

// LoadSettings 读取配置，文件缺失或损坏时回落到默认值
// 启动路径上的读取失败不该让应用起不来，因此这里不返回错误
// LoadSettings reads the settings and falls back to defaults when the file is missing or damaged
// A read failure on the startup path must not stop the app from launching, so no error is returned
func LoadSettings() Settings {
	settings := DefaultSettings()
	path, err := settingsPath()
	if err != nil {
		return settings
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return settings
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		// 半路解析失败可能已经写坏了部分字段，整体退回默认值比留下混合状态可靠
		// A partial decode may already have written some fields, so falling back wholesale beats a mixed state
		return DefaultSettings()
	}
	return settings
}

// SaveSettings 写入配置，必要时创建应用配置目录
// SaveSettings writes the settings, creating the application config directory when needed
func SaveSettings(settings Settings) error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create the settings directory: %w", err)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the settings: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write the settings %q: %w", path, err)
	}
	return nil
}

// GetSettings 返回当前的启动期配置，供设置页展示
// GetSettings returns the current startup configuration for the settings page
func (a *App) GetSettings() Settings {
	return LoadSettings()
}

// SetSingleInstance 切换单实例开关，下次启动生效
// SetSingleInstance toggles single instance and takes effect on the next launch
func (a *App) SetSingleInstance(enabled bool) error {
	settings := LoadSettings()
	settings.SingleInstance = enabled
	return SaveSettings(settings)
}
