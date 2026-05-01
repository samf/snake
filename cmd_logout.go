package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type LogoutCmd struct{}

func (l *LogoutCmd) Run(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("not logged in")
	}

	// Best-effort server-side revocation.
	req, err := http.NewRequest(http.MethodPost, cfg.Server+"/auth/revoke", nil)
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}

	// Write config back with token and can_id cleared, preserving server.
	path, err := configPath()
	if err != nil {
		return fmt.Errorf("could not locate config: %w", err)
	}
	data, err := json.MarshalIndent(Config{Server: cfg.Server}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("could not update config: %w", err)
	}

	fmt.Println("Logged out.")
	return nil
}
