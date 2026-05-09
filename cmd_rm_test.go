package main

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileMD5(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "md5test")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("hello world")
	f.Close()

	// echo -n "hello world" | md5
	const want = "5eb63bbbe01eeed093cb22bb8f5acdc3"
	got, err := fileMD5(f.Name())
	if err != nil {
		t.Fatalf("fileMD5: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeQuotes(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{`no special chars`, `no special chars`},
		{`say "hello"`, `say \"hello\"`},
		{`back\slash`, `back\\slash`},
		{`both "and" \slash`, `both \"and\" \\slash`},
	}
	for _, tt := range tests {
		if got := escapeQuotes(tt.in); got != tt.want {
			t.Errorf("escapeQuotes(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDetectMIME_ByExtension(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".html", "text/html; charset=utf-8"},
		{".json", "application/json"},
		{".png",  "image/png"},
		{".pdf",  "application/pdf"},
	}
	dir := t.TempDir()
	for _, tt := range tests {
		path := filepath.Join(dir, "file"+tt.ext)
		os.WriteFile(path, []byte("data"), 0600)
		got := detectMIME(path)
		// Use mime package to normalise for comparison.
		gotBase, _, _ := mime.ParseMediaType(got)
		wantBase, _, _ := mime.ParseMediaType(tt.want)
		if gotBase != wantBase {
			t.Errorf("detectMIME(%q): got %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestUploadFile_Success(t *testing.T) {
	content := []byte("test file content")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method %q", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing Authorization header")
		}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("not multipart: %v", err)
		}
		fields := map[string]string{}
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("multipart error: %v", err)
			}
			data, _ := io.ReadAll(part)
			fields[part.FormName()] = string(data)
		}
		if fields["path"] == "" {
			t.Error("missing path field")
		}
		if fields["checksum"] == "" {
			t.Error("missing checksum field")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"uuid": "abc"})
	}))
	defer ts.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, content, 0600)

	checksum, _ := fileMD5(path)
	cfg := &Config{Server: ts.URL, Token: "test-token", CanID: "can-1"}
	if err := uploadFile(cfg, path, "test.txt", dir, checksum); err != nil {
		t.Fatalf("uploadFile: %v", err)
	}
}

func TestUploadFile_Duplicate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Consume body to avoid broken pipe.
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "file already in the can (unchanged)"})
	}))
	defer ts.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "dup.txt")
	os.WriteFile(path, []byte("data"), 0600)

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	err := uploadFile(cfg, path, "dup.txt", dir, "somechecksum")
	if err == nil {
		t.Fatal("expected error for 409, got nil")
	}
}

func TestRmCmd_ValidateRejectsDirWithoutFlag(t *testing.T) {
	dir := t.TempDir()
	cmd := &RmCmd{Files: []string{dir}, Recursive: false}
	if err := cmd.Validate(); err == nil {
		t.Fatal("expected error for directory without -r, got nil")
	}
}

func TestRmCmd_RecursiveUploadsAndDeletes(t *testing.T) {
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
	cmd := &RmCmd{Files: []string{dir}, Recursive: true}
	if err := cmd.Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, name := range []string{"a.txt", "b.txt"} {
		if !uploaded[name] {
			t.Errorf("expected %s to be uploaded", name)
		}
	}
	// Files and the directory tree should be gone.
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err == nil {
		t.Error("a.txt should have been deleted")
	}
	if _, err := os.Stat(filepath.Join(subdir, "b.txt")); err == nil {
		t.Error("b.txt should have been deleted")
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("directory should have been removed")
	}
}

func TestRmCmd_RecursiveEmptyDir(t *testing.T) {
	dir := t.TempDir()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("unexpected upload call for empty directory")
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	cmd := &RmCmd{Files: []string{dir}, Recursive: true}
	out := captureStdout(t, func() { cmd.Run(cfg) })

	if !strings.Contains(out, "no files") {
		t.Errorf("expected 'no files' notice, got: %q", out)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("empty directory should have been removed")
	}
}

func TestUploadFile_QuotaExceeded(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Consume the multipart body.
		mr, _ := r.MultipartReader()
		if mr != nil {
			for {
				p, err := mr.NextPart()
				if err != nil {
					break
				}
				io.Copy(io.Discard, p)
			}
		}
		w.WriteHeader(http.StatusInsufficientStorage)
		json.NewEncoder(w).Encode(map[string]string{"error": "storage quota exceeded"})
	}))
	defer ts.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	os.WriteFile(path, []byte("data"), 0600)

	cfg := &Config{Server: ts.URL, Token: "tok", CanID: "can-1"}
	err := uploadFile(cfg, path, "big.txt", dir, "checksum")
	if err == nil {
		t.Fatal("expected error for 507, got nil")
	}
}

