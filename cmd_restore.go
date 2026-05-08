package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type RestoreCmd struct {
	Dir       string `arg:"" optional:"" help:"Directory to restore from (default: current directory)."`
	Recursive bool   `short:"r" help:"Include files in subdirectories."`
}

func (r *RestoreCmd) Run(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("not authenticated — run 'snake login'")
	}
	if cfg.CanID == "" {
		id, name, err := resolveCanID(cfg.Server, cfg.Token)
		if err != nil {
			return err
		}
		cfg.CanID = id
		fmt.Fprintf(os.Stderr, "using can: %s (%s)\n", name, id)
	}

	dir := r.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("could not determine current directory: %w", err)
		}
	} else {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		dir = abs
	}

	files, err := fetchFilesByPath(cfg, dir, r.Recursive)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		if r.Recursive {
			fmt.Printf("no files under %s\n", dir)
		} else {
			fmt.Printf("no files in %s\n", dir)
		}
		return nil
	}

	for _, f := range files {
		destDir := dir
		if f.Path != nil && *f.Path != "" {
			destDir = *f.Path
		}
		dest := filepath.Join(destDir, f.Name)
		fmt.Printf("restoring %s... ", dest)
		if err := downloadFile(cfg, f.UUID, dest); err != nil {
			fmt.Println("failed")
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		fmt.Println("done")
	}
	return nil
}

func downloadFile(cfg *Config, uuid, dest string) error {
	url := fmt.Sprintf("%s/can/%s/file/%s", cfg.Server, cfg.CanID, uuid)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("file not found on server")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return nil
}
