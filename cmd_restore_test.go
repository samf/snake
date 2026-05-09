package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestIsUUID(t *testing.T) {
	valid := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"550E8400-E29B-41D4-A716-446655440000",
	}
	invalid := []string{
		"foo.txt",
		"subdir/foo.txt",
		"550e8400-e29b-41d4-a716",          // too short
		"550e8400-e29b-41d4-a716-44665544000g", // invalid char
		"",
	}
	for _, s := range valid {
		if !isUUID(s) {
			t.Errorf("isUUID(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if isUUID(s) {
			t.Errorf("isUUID(%q) = true, want false", s)
		}
	}
}

func TestRestoreByUUID_Success(t *testing.T) {
	dir := t.TempDir()
	uuid := "550e8400-e29b-41d4-a716-446655440000"
	content := []byte("file content by uuid")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/info") {
			writeJSON(w, lsFile{UUID: uuid, Name: "report.pdf", Path: strPtr(dir)})
		} else {
			w.Write(content)
		}
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	cmd := &RestoreCmd{}
	if err := cmd.restoreByUUID(cfg, uuid); err != nil {
		t.Fatalf("restoreByUUID: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "report.pdf"))
	if string(got) != string(content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestRestoreByUUID_BlocksExisting(t *testing.T) {
	dir := t.TempDir()
	uuid := "550e8400-e29b-41d4-a716-446655440000"
	os.WriteFile(filepath.Join(dir, "report.pdf"), []byte("old"), 0600)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/info") {
			writeJSON(w, lsFile{UUID: uuid, Name: "report.pdf", Path: strPtr(dir)})
		} else {
			t.Error("unexpected download call")
		}
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	cmd := &RestoreCmd{}
	if err := cmd.restoreByUUID(cfg, uuid); err == nil {
		t.Fatal("expected error for existing file, got nil")
	}
}

func TestRestoreByUUID_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	uuid := "550e8400-e29b-41d4-a716-446655440000"
	os.WriteFile(filepath.Join(dir, "report.pdf"), []byte("old"), 0600)
	newContent := []byte("new content")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/info") {
			writeJSON(w, lsFile{UUID: uuid, Name: "report.pdf", Path: strPtr(dir)})
		} else {
			w.Write(newContent)
		}
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	cmd := &RestoreCmd{Force: true}
	if err := cmd.restoreByUUID(cfg, uuid); err != nil {
		t.Fatalf("restoreByUUID with force: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "report.pdf"))
	if string(got) != string(newContent) {
		t.Errorf("got %q, want %q", got, newContent)
	}
}

func TestRestoreByUUID_NotFound(t *testing.T) {
	uuid := "550e8400-e29b-41d4-a716-446655440000"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	cmd := &RestoreCmd{}
	if err := cmd.restoreByUUID(cfg, uuid); err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestRestoreFile_PicksMostRecent(t *testing.T) {
	dir := t.TempDir()

	// Server returns two versions of foo.txt ordered newest-first (as the real
	// server does: uploaded DESC). Each version has distinct content so we can
	// tell which one was actually downloaded.
	newerUUID := "uuid-newer"
	olderUUID := "uuid-older"
	newerContent := []byte("newer version")
	olderContent := []byte("older version")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/can/can-1/files" {
			p := strPtr(dir)
			files := []lsFile{
				{UUID: newerUUID, Name: "foo.txt", Path: p, Uploaded: 2000},
				{UUID: olderUUID, Name: "foo.txt", Path: p, Uploaded: 1000},
			}
			writeJSON(w, lsAPIResponse{Files: files})
			return
		}
		// Download by UUID — return distinct content per version.
		if strings.HasSuffix(r.URL.Path, newerUUID) {
			w.Write(newerContent)
		} else {
			w.Write(olderContent)
		}
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	cmd := &RestoreCmd{}
	if err := cmd.restoreFile(cfg, dir, "foo.txt"); err != nil {
		t.Fatalf("restoreFile: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "foo.txt"))
	if string(got) != string(newerContent) {
		t.Errorf("got %q, want newer version %q", got, newerContent)
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

func TestRestoreDir_RestoresImmediateSubdirs(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")

	directContent := []byte("direct file")
	subdirContent := []byte("subdir file")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/can/can-1/files" {
			files := []lsFile{
				{UUID: "uuid-direct", Name: "a.txt", Path: strPtr(dir)},
				{UUID: "uuid-sub", Name: "b.txt", Path: strPtr(subdir)},
			}
			writeJSON(w, lsAPIResponse{Files: files})
			return
		}
		if strings.HasSuffix(r.URL.Path, "uuid-direct") {
			w.Write(directContent)
		} else {
			w.Write(subdirContent)
		}
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	cmd := &RestoreCmd{}
	if err := cmd.restoreDir(cfg, dir); err != nil {
		t.Fatalf("restoreDir: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(got) != string(directContent) {
		t.Errorf("a.txt: got %q, want %q", got, directContent)
	}
	got, _ = os.ReadFile(filepath.Join(subdir, "b.txt"))
	if string(got) != string(subdirContent) {
		t.Errorf("b.txt: got %q, want %q", got, subdirContent)
	}
}

func TestRestoreDir_SkipsTwoLevelsDeep(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "sub", "subsub")

	downloadCalled := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/can/can-1/files" {
			files := []lsFile{
				{UUID: "uuid-deep", Name: "c.txt", Path: strPtr(deep)},
			}
			writeJSON(w, lsAPIResponse{Files: files})
			return
		}
		downloadCalled = true
		w.Write([]byte("data"))
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	cmd := &RestoreCmd{}
	if err := cmd.restoreDir(cfg, dir); err != nil {
		t.Fatalf("restoreDir: %v", err)
	}
	if downloadCalled {
		t.Error("should not download a file two levels deep in non-recursive mode")
	}
}

func TestRestoreDir_RecursiveIncludesAllDepths(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "sub", "subsub")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/can/can-1/files" {
			files := []lsFile{
				{UUID: "uuid-deep", Name: "c.txt", Path: strPtr(deep)},
			}
			writeJSON(w, lsAPIResponse{Files: files})
			return
		}
		w.Write([]byte("deep content"))
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	cmd := &RestoreCmd{Recursive: true}
	if err := cmd.restoreDir(cfg, dir); err != nil {
		t.Fatalf("restoreDir -r: %v", err)
	}
	if _, err := os.Stat(filepath.Join(deep, "c.txt")); err != nil {
		t.Errorf("expected deep file to be restored: %v", err)
	}
}
