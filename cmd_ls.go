package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"
)

type LsCmd struct {
	Dir string `arg:"" optional:"" help:"Directory to list (default: current directory)."`
}

type lsFile struct {
	UUID     string  `json:"uuid"`
	Name     string  `json:"name"`
	Uploaded int64   `json:"uploaded"`
	Expires  int64   `json:"expires"`
	Mime     string  `json:"mime"`
	Size     int64   `json:"size"`
	Client   *string `json:"client"`
	Path     *string `json:"path"`
}

type lsAPIResponse struct {
	Files      []lsFile `json:"files"`
	NextCursor *string  `json:"nextCursor"`
}

func (l *LsCmd) Run(cfg *Config) error {
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

	dir := l.Dir
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

	files, err := fetchFilesByPath(cfg, dir)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Printf("no files under %s\n", dir)
		return nil
	}

	// Group by path for display.
	grouped := map[string][]lsFile{}
	var keys []string
	for _, f := range files {
		p := ""
		if f.Path != nil {
			p = *f.Path
		}
		if _, seen := grouped[p]; !seen {
			keys = append(keys, p)
		}
		grouped[p] = append(grouped[p], f)
	}
	sort.Strings(keys)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for i, key := range keys {
		if i > 0 {
			fmt.Fprintln(w)
		}
		if key == "" {
			fmt.Fprintln(w, "(no path)")
		} else {
			fmt.Fprintln(w, key)
		}
		for _, f := range grouped[key] {
			expires := time.UnixMilli(f.Expires).Format("Jan 2, 2006")
			fmt.Fprintf(w, "  %s\t%s\t%s\n", f.Name, formatLsSize(f.Size), expires)
		}
	}
	w.Flush()
	return nil
}

func fetchFilesByPath(cfg *Config, dir string) ([]lsFile, error) {
	var all []lsFile
	cursor := ""
	for {
		u := fmt.Sprintf("%s/can/%s/files?path=%s&limit=200",
			cfg.Server, cfg.CanID, url.QueryEscape(dir))
		if cursor != "" {
			u += "&cursor=" + url.QueryEscape(cursor)
		}
		req, err := http.NewRequest(http.MethodGet, u, nil)
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

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("server returned %d", resp.StatusCode)
		}

		var page lsAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			return nil, fmt.Errorf("unexpected response: %w", err)
		}
		all = append(all, page.Files...)
		if page.NextCursor == nil || *page.NextCursor == "" {
			break
		}
		cursor = *page.NextCursor
	}
	return all, nil
}

func formatLsSize(b int64) string {
	switch {
	case b < 1024:
		return fmt.Sprintf("%d B", b)
	case b < 1_048_576:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	case b < 1_073_741_824:
		return fmt.Sprintf("%.1f MB", float64(b)/1_048_576)
	default:
		return fmt.Sprintf("%.1f GB", float64(b)/1_073_741_824)
	}
}

