package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type StatusCmd struct{}

type meResponse struct {
	Name  *string `json:"name"`
	Email *string `json:"email"`
}

func (s *StatusCmd) Run(cfg *Config) error {
	fmt.Printf("snake %s\n\n", version)

	path, _ := configPath()
	fmt.Printf("config   %s\n", path)

	server := defaultServer
	if cfg != nil && cfg.Server != "" {
		server = cfg.Server
	}
	fmt.Printf("server   %s\n", server)

	if cfg == nil || cfg.Token == "" {
		fmt.Printf("login    not logged in\n")
		fmt.Printf("can      —\n")
		return nil
	}

	// Fetch identity.
	me, err := fetchMe(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not reach server: %v\n", err)
		fmt.Printf("login    token present (server unreachable)\n")
	} else if me == nil {
		fmt.Printf("login    token invalid or expired — run 'snake login'\n")
	} else {
		ident := formatIdent(me)
		fmt.Printf("login    %s\n", ident)
	}

	// Show default can.
	if cfg.CanID != "" {
		fmt.Printf("can      %s\n", cfg.CanID)
	} else {
		fmt.Printf("can      not set\n")
	}

	return nil
}

func fetchMe(cfg *Config) (*meResponse, error) {
	req, err := http.NewRequest(http.MethodGet, cfg.Server+"/auth/me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var me meResponse
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return nil, err
	}
	return &me, nil
}

func formatIdent(me *meResponse) string {
	name := ""
	email := ""
	if me.Name != nil {
		name = *me.Name
	}
	if me.Email != nil {
		email = *me.Email
	}
	if name != "" && email != "" {
		return fmt.Sprintf("%s <%s>", name, email)
	}
	if email != "" {
		return email
	}
	if name != "" {
		return name
	}
	return "logged in"
}
