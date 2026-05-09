package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

type LsCmd struct {
	Dir       string `arg:"" optional:"" help:"Directory to list (default: current directory)."`
	Recursive bool   `short:"r" help:"Include files in subdirectories."`
	Long      bool   `short:"l" help:"Show size, uploaded date, and expiration date."`
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

	// Always fetch recursively so we can detect immediate subdirectories.
	files, err := fetchFilesByPath(cfg, dir, true)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		if l.Recursive {
			fmt.Printf("no files under %s\n", dir)
		} else {
			fmt.Printf("no files in %s\n", dir)
		}
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if l.Recursive {
		// Group by path and show each group with a subdir header.
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

		if l.Long {
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", "Name", "Size", "Uploaded", "Expires", "UUID")
		}
		for i, key := range keys {
			if key != dir {
				if i > 0 {
					fmt.Fprintln(w)
				}
				label := key
				if rel, err := filepath.Rel(dir, key); err == nil {
					label = rel
				}
				if label == "" {
					label = "(no path)"
				}
				fmt.Fprintln(w, label)
			}
			seen := map[string]bool{}
			for _, f := range grouped[key] {
				if l.Long {
					uploaded := time.UnixMilli(f.Uploaded).Format("Jan 2, 2006")
					expires := time.UnixMilli(f.Expires).Format("Jan 2, 2006")
					fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", f.Name, formatLsSize(f.Size), uploaded, expires, f.UUID)
				} else {
					if seen[f.Name] {
						continue
					}
					seen[f.Name] = true
					fmt.Fprintf(w, "  %s\n", f.Name)
				}
			}
		}
		w.Flush()
		return nil
	}

	// Non-recursive: separate files in this exact directory from files deeper
	// in the tree. For deeper files, expose only the immediate child directory.
	dirPrefix := dir + string(filepath.Separator)
	var directFiles []lsFile
	seenDirs := map[string]bool{}
	var subdirNames []string

	for _, f := range files {
		fPath := ""
		if f.Path != nil {
			fPath = *f.Path
		}
		if fPath == dir {
			directFiles = append(directFiles, f)
		} else if strings.HasPrefix(fPath, dirPrefix) {
			child := strings.SplitN(fPath[len(dirPrefix):], string(filepath.Separator), 2)[0]
			if !seenDirs[child] {
				seenDirs[child] = true
				subdirNames = append(subdirNames, child)
			}
		}
	}
	sort.Strings(subdirNames)

	if l.Long {
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", "Name", "Size", "Uploaded", "Expires", "UUID")
		for _, f := range directFiles {
			uploaded := time.UnixMilli(f.Uploaded).Format("Jan 2, 2006")
			expires := time.UnixMilli(f.Expires).Format("Jan 2, 2006")
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", f.Name, formatLsSize(f.Size), uploaded, expires, f.UUID)
		}
		for _, d := range subdirNames {
			fmt.Fprintf(w, "  %s/\t—\t—\t—\t—\n", d)
		}
	} else {
		type entry struct{ name string }
		var entries []entry
		seen := map[string]bool{}
		for _, f := range directFiles {
			if !seen[f.Name] {
				seen[f.Name] = true
				entries = append(entries, entry{f.Name})
			}
		}
		for _, d := range subdirNames {
			entries = append(entries, entry{d + "/"})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
		for _, e := range entries {
			fmt.Fprintf(w, "  %s\n", e.name)
		}
	}
	w.Flush()
	return nil
}

func fetchFilesByPath(cfg *Config, dir string, recursive bool) ([]lsFile, error) {
	var all []lsFile
	cursor := ""
	recursiveParam := ""
	if recursive {
		recursiveParam = "&recursive=true"
	}
	for {
		u := fmt.Sprintf("%s/can/%s/files?path=%s&limit=200%s",
			cfg.Server, cfg.CanID, url.QueryEscape(dir), recursiveParam)
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

