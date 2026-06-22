package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadCmd_RecursiveKeepsFiles(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	os.Mkdir(subdir, 0755)

	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0600)
	os.WriteFile(filepath.Join(subdir, "b.txt"), []byte("bbb"), 0600)

	uploaded := map[string]bool{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		fh := r.MultipartForm.File["file"]
		if len(fh) > 0 {
			uploaded[fh[0].Filename] = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	cmd := &UploadCmd{Files: []string{dir}, Recursive: true}
	if err := cmd.Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, name := range []string{"a.txt", "b.txt"} {
		if !uploaded[name] {
			t.Errorf("expected %s to be uploaded", name)
		}
	}
	// Files and the directory tree should remain in place.
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Errorf("a.txt should still exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(subdir, "b.txt")); err != nil {
		t.Errorf("b.txt should still exist: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("directory should still exist: %v", err)
	}
}

func TestUploadCmd_ValidateRejectsDirWithoutFlag(t *testing.T) {
	dir := t.TempDir()
	cmd := &UploadCmd{Files: []string{dir}, Recursive: false}
	if err := cmd.Validate(); err == nil {
		t.Fatal("expected error for directory without -r, got nil")
	}
}
