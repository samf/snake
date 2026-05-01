package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Server string `json:"server"`
	CanID  string `json:"can_id"`
	Token  string `json:"token"`
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "snake", "config.json"), nil
}

func saveConfig(updates Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// Merge into existing config so we don't wipe unrelated fields.
	existing, _ := loadConfig()
	if existing == nil {
		existing = &Config{}
	}
	if updates.Server != "" {
		existing.Server = updates.Server
	}
	if updates.CanID != "" {
		existing.CanID = updates.CanID
	}
	if updates.Token != "" {
		existing.Token = updates.Token
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func loadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("snake is not configured — create %s with server, can_id, and token", path)
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config at %s: %w", path, err)
	}
	if cfg.Server == "" || cfg.Token == "" {
		return nil, fmt.Errorf("config at %s is incomplete — run 'snake login'", path)
	}
	return &cfg, nil
}
