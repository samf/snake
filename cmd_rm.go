package main

import (
	"bytes"
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
	Files []string `arg:"" name:"file" help:"Files to send to the Snake Can." min:"1"`
}

func (r *RmCmd) Validate() error {
	for _, path := range r.Files {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s: not a regular file", path)
		}
	}
	return nil
}

func (r *RmCmd) Run(cfg *Config) error {
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
	for _, path := range r.Files {
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		name := filepath.Base(abs)
		dir := filepath.Dir(abs)
		fmt.Printf("uploading %s... ", name)
		if err := uploadFile(cfg, abs, name, dir); err != nil {
			fmt.Println("failed")
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := os.Remove(abs); err != nil {
			fmt.Println("uploaded, but could not remove local file")
			return fmt.Errorf("%s: %w", path, err)
		}
		fmt.Println("done")
	}
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

func uploadFile(cfg *Config, path, name, dir string) error {
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
