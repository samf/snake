package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

var uuidRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isUUID(s string) bool { return uuidRe.MatchString(s) }

type RestoreCmd struct {
	Path      string `arg:"" optional:"" help:"File or directory to restore (default: current directory)."`
	Recursive bool   `short:"r" help:"Include files in subdirectories."`
	Force     bool   `short:"f" help:"Overwrite existing local files."`
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

	if r.Path == "" {
		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not determine current directory: %w", err)
		}
		return r.restoreDir(cfg, dir)
	}

	if isUUID(r.Path) {
		return r.restoreByUUID(cfg, r.Path)
	}

	abs, err := filepath.Abs(r.Path)
	if err != nil {
		return fmt.Errorf("%s: %w", r.Path, err)
	}

	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		return r.restoreDir(cfg, abs)
	}
	return r.restoreFile(cfg, filepath.Dir(abs), filepath.Base(abs))
}

func (r *RestoreCmd) restoreDir(cfg *Config, dir string) error {
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

	if !r.Force {
		var conflicts []string
		for _, f := range files {
			dest := destForFile(f, dir)
			if _, err := os.Stat(dest); err == nil {
				conflicts = append(conflicts, dest)
			}
		}
		if len(conflicts) > 0 {
			for _, c := range conflicts {
				fmt.Fprintf(os.Stderr, "exists: %s\n", c)
			}
			return fmt.Errorf("refusing to overwrite existing files — use -f to force")
		}
	}

	for _, f := range files {
		dest := destForFile(f, dir)
		fmt.Printf("restoring %s... ", dest)
		if err := downloadFile(cfg, f.UUID, dest); err != nil {
			fmt.Println("failed")
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		fmt.Println("done")
	}
	return nil
}

func destForFile(f lsFile, fallbackDir string) string {
	dir := fallbackDir
	if f.Path != nil && *f.Path != "" {
		dir = *f.Path
	}
	return filepath.Join(dir, f.Name)
}

func (r *RestoreCmd) restoreFile(cfg *Config, dir, name string) error {
	dest := filepath.Join(dir, name)
	if !r.Force {
		if _, err := os.Stat(dest); err == nil {
			return fmt.Errorf("%s: already exists — use -f to force", dest)
		}
	}

	files, err := fetchFilesByPath(cfg, dir, false)
	if err != nil {
		return err
	}
	var match *lsFile
	for i, f := range files {
		if f.Name == name {
			match = &files[i]
			break
		}
	}
	if match == nil {
		return fmt.Errorf("%s: not found in the can", name)
	}
	fmt.Printf("restoring %s... ", dest)
	if err := downloadFile(cfg, match.UUID, dest); err != nil {
		fmt.Println("failed")
		return fmt.Errorf("%s: %w", name, err)
	}
	fmt.Println("done")
	return nil
}

func (r *RestoreCmd) restoreByUUID(cfg *Config, uuid string) error {
	meta, err := fetchFileMeta(cfg, uuid)
	if err != nil {
		return err
	}

	destDir := ""
	if meta.Path != nil && *meta.Path != "" {
		destDir = *meta.Path
	} else {
		destDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("could not determine current directory: %w", err)
		}
	}

	dest := filepath.Join(destDir, meta.Name)
	if !r.Force {
		if _, err := os.Stat(dest); err == nil {
			return fmt.Errorf("%s: already exists — use -f to force", dest)
		}
	}

	fmt.Printf("restoring %s... ", dest)
	if err := downloadFile(cfg, uuid, dest); err != nil {
		fmt.Println("failed")
		return fmt.Errorf("%s: %w", meta.Name, err)
	}
	fmt.Println("done")
	return nil
}

func fetchFileMeta(cfg *Config, uuid string) (*lsFile, error) {
	url := fmt.Sprintf("%s/can/%s/file/%s/info", cfg.Server, cfg.CanID, uuid)
	req, err := http.NewRequest(http.MethodGet, url, nil)
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

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%s: not found in the can", uuid)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var meta lsFile
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("unexpected response: %w", err)
	}
	return &meta, nil
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
