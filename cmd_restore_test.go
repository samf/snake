package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDestForFile(t *testing.T) {
	storedPath := "/home/user/docs"
	tests := []struct {
		name        string
		file        lsFile
		fallbackDir string
		want        string
	}{
		{
			name:        "uses stored path",
			file:        lsFile{Name: "a.txt", Path: strPtr(storedPath)},
			fallbackDir: "/fallback",
			want:        "/home/user/docs/a.txt",
		},
		{
			name:        "falls back when path is nil",
			file:        lsFile{Name: "a.txt", Path: nil},
			fallbackDir: "/fallback",
			want:        "/fallback/a.txt",
		},
		{
			name:        "falls back when path is empty string",
			file:        lsFile{Name: "a.txt", Path: strPtr("")},
			fallbackDir: "/fallback",
			want:        "/fallback/a.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := destForFile(tt.file, tt.fallbackDir)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDownloadFile_Success(t *testing.T) {
	content := []byte("restored file content")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing Authorization header")
		}
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer ts.Close()

	dest := filepath.Join(t.TempDir(), "out.txt")
	cfg := &Config{Server: ts.URL, Token: "test-token", CanID: "can-1"}
	if err := downloadFile(cfg, "uuid-abc", dest); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestDownloadFile_CreatesParentDir(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer ts.Close()

	dest := filepath.Join(t.TempDir(), "subdir", "nested", "out.txt")
	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	if err := downloadFile(cfg, "uuid-abc", dest); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}

func TestDownloadFile_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	dest := filepath.Join(t.TempDir(), "out.txt")
	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	err := downloadFile(cfg, "uuid-missing", dest)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestRestoreFile_BlocksExistingFile(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "foo.txt")
	os.WriteFile(existing, []byte("old"), 0600)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should never reach download — the conflict check fires first.
		t.Error("unexpected HTTP call during conflict check")
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	cmd := &RestoreCmd{}
	err := cmd.restoreFile(cfg, dir, "foo.txt")
	if err == nil {
		t.Fatal("expected error when file already exists, got nil")
	}
}

func TestRestoreFile_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "foo.txt")
	os.WriteFile(existing, []byte("old"), 0600)

	newContent := []byte("new content")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a file listing on /can/.../files, then file content on /can/.../file/...
		if r.URL.Path == "/can/can-1/files" {
			p := strPtr(dir)
			files := []lsFile{{UUID: "uuid-1", Name: "foo.txt", Path: p}}
			writeJSON(w, lsAPIResponse{Files: files})
		} else {
			w.Write(newContent)
		}
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	cmd := &RestoreCmd{Force: true}
	if err := cmd.restoreFile(cfg, dir, "foo.txt"); err != nil {
		t.Fatalf("restoreFile with force: %v", err)
	}
	got, _ := os.ReadFile(existing)
	if string(got) != string(newContent) {
		t.Errorf("got %q, want %q", got, newContent)
	}
}

func TestRestoreFile_NotFoundInCan(t *testing.T) {
	dir := t.TempDir()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, lsAPIResponse{Files: []lsFile{}})
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	cmd := &RestoreCmd{}
	err := cmd.restoreFile(cfg, dir, "missing.txt")
	if err == nil {
		t.Fatal("expected error for file not in can, got nil")
	}
}
