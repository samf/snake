package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
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
