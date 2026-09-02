package internal

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSettingsRoundTrip 检查开关写下去再读回来还是同一个值，且落在预期的路径上
// TestSettingsRoundTrip checks a toggle written and read back keeps its value and lands on the expected path
func TestSettingsRoundTrip(t *testing.T) {
	isolatedConfigDir(t)
	app := &App{}

	// 还没有配置文件时必须给出默认值而不是报错，这条路径在启动时先跑
	// With no settings file yet the defaults must come back instead of an error, since this runs at startup
	if got := app.GetSettings(); got != DefaultSettings() {
		t.Errorf("GetSettings() without a file = %+v, want %+v", got, DefaultSettings())
	}

	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	// 配置要落在用户配置目录下的应用子目录里，而不是可执行文件旁边或工作目录
	// The settings must land in the application folder of the user config directory, not next to the exe or in the working directory
	if filepath.Base(path) != "settings.json" || filepath.Base(filepath.Dir(path)) != "ABA_EXPLORER" {
		t.Errorf("settingsPath() = %q, want ABA_EXPLORER/settings.json inside the user config directory", path)
	}
	if _, err := os.Stat(path); err == nil {
		t.Errorf("GetSettings() created %q, reading must not write", path)
	}

	for _, enabled := range []bool{true, false, true} {
		if err := app.SetSingleInstance(enabled); err != nil {
			t.Fatalf("SetSingleInstance(%v): %v", enabled, err)
		}
		if got := app.GetSettings().SingleInstance; got != enabled {
			t.Errorf("SingleInstance after SetSingleInstance(%v) = %v", enabled, got)
		}
		// 启动路径读的是 LoadSettings，它必须看到同一个值
		// The startup path reads LoadSettings, which has to see the same value
		if got := LoadSettings().SingleInstance; got != enabled {
			t.Errorf("LoadSettings().SingleInstance after SetSingleInstance(%v) = %v", enabled, got)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("stat the written settings: %v", err)
	}
}

// TestLoadSettingsFallsBackOnDamagedFile 检查配置文件损坏时应用还能起来
// 启动路径上的读取失败不该让程序打不开，只能退回默认值
// TestLoadSettingsFallsBackOnDamagedFile checks a damaged settings file still lets the application launch
// A read failure on the startup path must not block startup, so it can only fall back to the defaults
func TestLoadSettingsFallsBackOnDamagedFile(t *testing.T) {
	isolatedConfigDir(t)

	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("create the settings directory: %v", err)
	}

	// 半个 JSON、空文件与写反了类型的字段，都要退回默认值而不是恐慌
	// Half a JSON document, an empty file, and a wrongly typed field must all fall back instead of panicking
	for _, content := range []string{"", "{", `{"singleInstance": true`, `{"singleInstance": "yes"}`, "\x00\x01\x02"} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write the damaged settings: %v", err)
		}
		if got := LoadSettings(); got != DefaultSettings() {
			t.Errorf("LoadSettings() with content %q = %+v, want %+v", content, got, DefaultSettings())
		}
	}

	// 未知字段应当被忽略而不是让整份配置作废，否则将来加了字段的版本回退就读不了
	// An unknown field must be ignored rather than voiding the file, otherwise downgrading past a new field breaks reads
	if err := os.WriteFile(path, []byte(`{"singleInstance": true, "somethingElse": 1}`), 0644); err != nil {
		t.Fatalf("write the settings: %v", err)
	}
	if !LoadSettings().SingleInstance {
		t.Error("LoadSettings() dropped a valid value because of an unknown field")
	}
}
