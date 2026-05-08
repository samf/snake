package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// snakeConfigPath returns the path that configPath() will resolve to under the given HOME.
func snakeConfigPath(t *testing.T, home string) string {
	t.Helper()
	// On Darwin: $HOME/Library/Application Support/snake/config.json
	// On Unix:   $HOME/.config/snake/config.json  (via XDG_CONFIG_HOME or HOME)
	// We derive it by calling configPath() after pointing HOME at the temp dir.
	t.Setenv("HOME", home)
	p, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	return p
}

func writeConfig(t *testing.T, path string, cfg Config) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestLoadConfig_Missing(t *testing.T) {
	home := t.TempDir()
	snakeConfigPath(t, home) // just sets HOME
	_, err := loadConfig()
	if err == nil {
		t.Fatal("expected error for missing config, got nil")
	}
}

func TestLoadConfig_Incomplete_NoToken(t *testing.T) {
	home := t.TempDir()
	p := snakeConfigPath(t, home)
	writeConfig(t, p, Config{Server: "https://example.com"})
	_, err := loadConfig()
	if err == nil {
		t.Fatal("expected error for config without token, got nil")
	}
}

func TestLoadConfig_Incomplete_NoServer(t *testing.T) {
	home := t.TempDir()
	p := snakeConfigPath(t, home)
	writeConfig(t, p, Config{Token: "tok"})
	_, err := loadConfig()
	if err == nil {
		t.Fatal("expected error for config without server, got nil")
	}
}

func TestLoadConfig_Valid(t *testing.T) {
	home := t.TempDir()
	p := snakeConfigPath(t, home)
	want := Config{Server: "https://example.com", Token: "tok", CanID: "can-1"}
	writeConfig(t, p, want)
	got, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Server != want.Server || got.Token != want.Token || got.CanID != want.CanID {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSaveConfig_CreatesDirectory(t *testing.T) {
	home := t.TempDir()
	p := snakeConfigPath(t, home)
	if err := saveConfig(Config{Server: "https://example.com", Token: "tok"}); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}

func TestSaveConfig_MergesFields(t *testing.T) {
	home := t.TempDir()
	p := snakeConfigPath(t, home)
	writeConfig(t, p, Config{Server: "https://example.com", Token: "tok1", CanID: "can-1"})

	// Update only the token — server and can_id should be preserved.
	if err := saveConfig(Config{Token: "tok2"}); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}
	got, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if got.Server != "https://example.com" {
		t.Errorf("server changed: got %q", got.Server)
	}
	if got.Token != "tok2" {
		t.Errorf("token not updated: got %q", got.Token)
	}
	if got.CanID != "can-1" {
		t.Errorf("can_id changed: got %q", got.CanID)
	}
}
