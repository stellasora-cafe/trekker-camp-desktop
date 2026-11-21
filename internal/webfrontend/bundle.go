// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

// Package webfrontend serves the complete frontend downloaded from the server.
package webfrontend

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type Manifest struct {
	Version string   `json:"version"`
	Entry   string   `json:"entry"`
	CSS     []string `json:"css"`
	Langs   []string `json:"langs"`
}

type Bundle struct {
	files    *zip.Reader
	manifest Manifest
	document string
	revision string
}

// FromArchive loads a validated server frontend archive.
func FromArchive(data []byte, origin string) (*Bundle, error) {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("webfrontend: invalid site origin")
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("webfrontend: archive: %w", err)
	}
	var total uint64
	names := make(map[string]bool)
	for _, file := range z.File {
		total += file.UncompressedSize64
		if total > 512<<20 || len(z.File) > 20000 || names[file.Name] {
			return nil, errors.New("webfrontend: invalid archive size or duplicate file")
		}
		names[file.Name] = true
		if !fs.ValidPath(file.Name) || strings.Contains(file.Name, "\\") || file.Mode()&fs.ModeSymlink != 0 || strings.HasPrefix(file.Name, "client-assets/camp/") {
			return nil, fmt.Errorf("webfrontend: forbidden asset path %q", file.Name)
		}
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(io.Discard, reader)
		_ = reader.Close()
		if err != nil {
			return nil, fmt.Errorf("webfrontend: corrupt asset %s: %w", file.Name, err)
		}
	}
	manifestJSON, err := fs.ReadFile(z, "bundle.json")
	if err != nil {
		return nil, fmt.Errorf("webfrontend: missing manifest: %w", err)
	}
	b := &Bundle{files: z, revision: fmt.Sprintf("%x", sha256.Sum256(data))}
	if err := json.Unmarshal(manifestJSON, &b.manifest); err != nil {
		return nil, fmt.Errorf("webfrontend: manifest: %w", err)
	}
	if b.manifest.Version == "" || !strings.HasPrefix(b.manifest.Entry, "scripts/") || len(b.manifest.Langs) == 0 {
		return nil, errors.New("webfrontend: incomplete manifest")
	}
	required := []string{"vite/loader/boot.js", "vite/loader/style.css", "vite/" + b.manifest.Entry}
	for _, lang := range b.manifest.Langs {
		if lang != "zh-CN" && lang != "zh-TW" {
			return nil, errors.New("webfrontend: unsupported language")
		}
		required = append(required, "vite/"+strings.Replace(b.manifest.Entry, "scripts/", lang+"/", 1))
	}
	for _, css := range b.manifest.CSS {
		required = append(required, "vite/"+css)
	}
	for _, name := range required {
		if !b.Contains("/" + name) {
			return nil, fmt.Errorf("webfrontend: missing required asset %s", name)
		}
	}
	var document bytes.Buffer
	err = documentTemplate.Execute(&document, struct {
		Origin   string
		Manifest Manifest
	}{strings.TrimSuffix(origin, "/"), b.manifest})
	if err != nil {
		return nil, err
	}
	b.document = document.String()
	return b, nil
}

func (b *Bundle) HTML() string { return b.document }

func (b *Bundle) Contains(rawPath string) bool {
	name, ok := assetPath(rawPath)
	if !ok || name == "bundle.json" {
		return false
	}
	info, err := fs.Stat(b.files, name)
	return err == nil && !info.IsDir()
}

func assetPath(rawPath string) (string, bool) {
	if !strings.HasPrefix(rawPath, "/") || strings.Contains(rawPath, "\\") || strings.Contains(rawPath, "\x00") || path.Clean(rawPath) != rawPath {
		return "", false
	}
	name := strings.TrimPrefix(rawPath, "/")
	return name, fs.ValidPath(name)
}

// IsDocumentPath excludes API, media and asset requests from the SPA fallback.
func IsDocumentPath(rawPath string) bool {
	if rawPath == "/" {
		return true
	}
	if _, ok := assetPath(strings.TrimSuffix(rawPath, "/")); !ok {
		return false
	}
	for _, prefix := range []string{"/api", "/url", "/streaming", "/client-streaming", "/vite", "/assets", "/client-assets", "/static-assets", "/files", "/proxy", "/avatar", "/identicon", "/emoji", "/fluent-emoji", "/.well-known", "/__camp", "/_desktop"} {
		if rawPath == prefix || strings.HasPrefix(rawPath, prefix+"/") {
			return false
		}
	}
	if strings.HasPrefix(rawPath, "/@") {
		return true
	}
	return path.Ext(rawPath) == ""
}

func (b *Bundle) Handler() http.Handler { return b }

func (b *Bundle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if b.Contains(r.URL.Path) {
		name, _ := assetPath(r.URL.Path)
		data, err := fs.ReadFile(b.files, name)
		if err != nil {
			http.Error(w, "asset unavailable", http.StatusInternalServerError)
			return
		}
		contentType := mime.TypeByExtension(path.Ext(name))
		switch path.Ext(name) {
		case ".js", ".mjs":
			contentType = "text/javascript; charset=utf-8"
		case ".wasm":
			contentType = "application/wasm"
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.Header().Set("Cache-Control", "no-cache")
		if strings.HasPrefix(name, "vite/scripts/") || strings.HasPrefix(name, "vite/assets/") || strings.HasPrefix(name, "vite/zh-") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
		return
	}
	if !IsDocumentPath(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		_, _ = io.WriteString(w, b.document)
	}
}

var documentTemplate = template.Must(template.New("desktop").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<meta name="theme-color" content="#edf2f8"><meta name="theme-color-orig" content="#edf2f8">
<meta property="instance_url" content="{{.Origin}}"><meta property="og:site_name" content="旅人营地">
<meta name="format-detection" content="telephone=no,date=no,address=no,email=no,url=no">
<title>旅人营地</title><link rel="icon" href="/favicon.ico">
<link rel="stylesheet" href="/vite/loader/style.css">
{{range .Manifest.CSS}}<link rel="stylesheet" href="/vite/{{.}}">{{end}}
<script>const VERSION={{.Manifest.Version}};const CLIENT_ENTRY={{.Manifest.Entry}};const LANGS={{.Manifest.Langs}};</script>
<script src="/vite/loader/boot.js"></script></head><body>
<div id="splash"><img id="splashIcon" alt="" src="/client-assets/trekker-camp/branding/icon-v1.svg"></div>
</body></html>`))
