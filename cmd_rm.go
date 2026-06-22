package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
)

type RmCmd struct {
	Files     []string `arg:"" name:"file" help:"Files or directories to send to the Snake Can." min:"1"`
	Recursive bool     `short:"r" help:"Recursively process directories."`
}

func (r *RmCmd) Validate() error {
	return validatePaths(r.Files, r.Recursive)
}

func (r *RmCmd) Run(cfg *Config) error {
	return uploadPaths(cfg, r.Files, r.Recursive, true)
}

// validatePaths checks that each path exists and is a regular file, or a
// directory when recursion is allowed.
func validatePaths(files []string, recursive bool) error {
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if info.IsDir() {
			if !recursive {
				return fmt.Errorf("%s: is a directory (use -r to recurse)", path)
			}
		} else if !info.Mode().IsRegular() {
			return fmt.Errorf("%s: not a regular file", path)
		}
	}
	return nil
}

// uploadPaths uploads each of the given files or directories. When remove is
// true the local copies are deleted after a successful upload.
func uploadPaths(cfg *Config, files []string, recursive, remove bool) error {
	if cfg == nil {
		return fmt.Errorf("not authenticated — run 'snake login'")
	}
	if cfg.CanID == "" {
		id, name, err := resolveCanID(cfg.Server, cfg.Token)
		if err != nil {
			return err
		}
		cfg.CanID = id
		fmt.Printf("Using can: %s (%s)\n", name, id)
	}
	for _, path := range files {
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if info.IsDir() {
			if err := uploadDir(cfg, abs, remove); err != nil {
				return err
			}
		} else {
			if err := uploadOne(cfg, abs, remove); err != nil {
				return err
			}
		}
	}
	return nil
}

func uploadDir(cfg *Config, dir string, remove bool) error {
	fileCount := 0
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		fileCount++
		return uploadOne(cfg, path, remove)
	}); err != nil {
		return err
	}
	if fileCount == 0 {
		fmt.Printf("no files in %s\n", dir)
	}
	if remove {
		return os.RemoveAll(dir)
	}
	return nil
}

func uploadOne(cfg *Config, abs string, remove bool) error {
	name := filepath.Base(abs)
	dir := filepath.Dir(abs)
	checksum, err := fileMD5(abs)
	if err != nil {
		return fmt.Errorf("%s: %w", abs, err)
	}
	fmt.Printf("uploading %s... ", name)
	if err := uploadFile(cfg, abs, name, dir, checksum); err != nil {
		fmt.Println("failed")
		return fmt.Errorf("%s: %w", abs, err)
	}
	if remove {
		if err := os.Remove(abs); err != nil {
			fmt.Println("uploaded, but could not remove local file")
			return fmt.Errorf("%s: %w", abs, err)
		}
	}
	fmt.Println("done")
	return nil
}

func detectMIME(path string) string {
	if ext := filepath.Ext(path); ext != "" {
		if t := mime.TypeByExtension(strings.ToLower(ext)); t != "" {
			return t
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	return http.DetectContentType(buf[:n])
}

func fileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func uploadFile(cfg *Config, path, name, dir, checksum string) error {
	contentType := detectMIME(path)

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	// File part — set Content-Type so the server reads the correct MIME.
	fh := make(textproto.MIMEHeader)
	fh.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeQuotes(name)))
	fh.Set("Content-Type", contentType)
	part, err := mw.CreatePart(fh)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}

	if err := mw.WriteField("path", dir); err != nil {
		return err
	}
	if err := mw.WriteField("client", "snake"); err != nil {
		return err
	}
	if err := mw.WriteField("checksum", checksum); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/can/%s/upload", cfg.Server, cfg.CanID)
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("already in the can (unchanged)")
	}
	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		raw, _ := io.ReadAll(resp.Body)
		if json.Unmarshal(raw, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("%s", errResp.Error)
		}
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

func escapeQuotes(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}
