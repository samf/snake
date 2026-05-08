package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFormatLsSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1_048_575, "1024.0 KB"},
		{1_048_576, "1.0 MB"},
		{10_485_760, "10.0 MB"},
		{1_073_741_824, "1.0 GB"},
		{2_147_483_648, "2.0 GB"},
	}
	for _, tt := range tests {
		got := formatLsSize(tt.bytes)
		if got != tt.want {
			t.Errorf("formatLsSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestFetchFilesByPath_Single(t *testing.T) {
	path := strPtr("/home/user/docs")
	files := []lsFile{
		{UUID: "uuid-1", Name: "a.txt", Size: 100, Path: path},
		{UUID: "uuid-2", Name: "b.txt", Size: 200, Path: path},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("path") == "" {
			t.Error("missing path query param")
		}
		json.NewEncoder(w).Encode(lsAPIResponse{Files: files})
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	got, err := fetchFilesByPath(cfg, "/home/user/docs", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d files, want 2", len(got))
	}
	if got[0].UUID != "uuid-1" || got[1].UUID != "uuid-2" {
		t.Errorf("unexpected file order: %v", got)
	}
}

func TestFetchFilesByPath_Paginated(t *testing.T) {
	page1 := []lsFile{{UUID: "uuid-1", Name: "a.txt"}}
	page2 := []lsFile{{UUID: "uuid-2", Name: "b.txt"}}
	cursor := "cursor-abc"

	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			next := cursor
			json.NewEncoder(w).Encode(lsAPIResponse{Files: page1, NextCursor: &next})
		} else {
			if r.URL.Query().Get("cursor") != cursor {
				t.Errorf("expected cursor %q, got %q", cursor, r.URL.Query().Get("cursor"))
			}
			json.NewEncoder(w).Encode(lsAPIResponse{Files: page2})
		}
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	got, err := fetchFilesByPath(cfg, "/docs", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d files, want 2", len(got))
	}
	if calls != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", calls)
	}
}

func lsTestServer(t *testing.T, files []lsFile) (*httptest.Server, *Config) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, lsAPIResponse{Files: files})
	}))
	t.Cleanup(ts.Close)
	return ts, &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
}

func TestLsCmd_TerseOutput(t *testing.T) {
	dir := "/test/docs"
	uploaded := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC).UnixMilli()
	expires := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC).UnixMilli()
	files := []lsFile{
		{UUID: "u1", Name: "report.pdf", Size: 2048, Path: strPtr(dir), Uploaded: uploaded, Expires: expires},
		{UUID: "u2", Name: "notes.txt", Size: 512, Path: strPtr(dir), Uploaded: uploaded, Expires: expires},
	}
	_, cfg := lsTestServer(t, files)

	cmd := &LsCmd{Dir: dir, Long: false}
	out := captureStdout(t, func() { cmd.Run(cfg) })

	if !strings.Contains(out, "report.pdf") {
		t.Errorf("terse output missing filename: %q", out)
	}
	if !strings.Contains(out, "notes.txt") {
		t.Errorf("terse output missing filename: %q", out)
	}
	// Terse mode must not include size or date columns.
	if strings.Contains(out, "KB") || strings.Contains(out, "MB") {
		t.Errorf("terse output should not contain size: %q", out)
	}
	if strings.Contains(out, "2025") || strings.Contains(out, "2026") {
		t.Errorf("terse output should not contain dates: %q", out)
	}
}

func TestLsCmd_LongOutput(t *testing.T) {
	dir := "/test/docs"
	uploaded := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC).UnixMilli()
	expires := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC).UnixMilli()
	files := []lsFile{
		{UUID: "u1", Name: "report.pdf", Size: 2 * 1024 * 1024, Path: strPtr(dir), Uploaded: uploaded, Expires: expires},
	}
	_, cfg := lsTestServer(t, files)

	cmd := &LsCmd{Dir: dir, Long: true}
	out := captureStdout(t, func() { cmd.Run(cfg) })

	checks := []string{"report.pdf", "2.0 MB", "Jan 15, 2025", "Mar 20, 2026"}
	for _, s := range checks {
		if !strings.Contains(out, s) {
			t.Errorf("long output missing %q in: %q", s, out)
		}
	}
}

func TestFetchFilesByPath_RecursiveParam(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query().Get("recursive")
		if got != "true" {
			fmt.Fprintf(w, `{"error":"expected recursive=true, got %q"}`, got)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(lsAPIResponse{Files: []lsFile{}})
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	if _, err := fetchFilesByPath(cfg, "/docs", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
