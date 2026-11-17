// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

// Package webassets prepares a small, explicitly listed runtime resource cache.
// It does not include application JavaScript, user media, or account credentials.
package webassets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const Scheme = "camp-assets"

// AssetBaseURL is MyGo's Windows mapping of camp-assets://localhost/.
const AssetBaseURL = "http://camp-assets.localhost/"

type ResourceReport struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Bytes  int64  `json:"bytes"`
	Source string `json:"source"`
	Ready  bool   `json:"ready"`
}

type Report struct {
	Resources    []ResourceReport `json:"resources"`
	Bytes        int64            `json:"bytes"`
	PublicBytes  int64            `json:"publicBytes"`
	PrivateBytes int64            `json:"privateBytes"`
	Warnings     []string         `json:"warnings,omitempty"`
}

type Bundle struct {
	origin    string
	directory string
	resources map[string]cachedResource
	rewrites  map[string]string
	report    Report
	installed map[string]string
}

type resourceSpec struct {
	path, remote, kind, mime string
	limit                    int64
}

type cachedResource struct {
	Remote       string `json:"remote"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	Bytes        int64  `json:"bytes"`
	MIME         string `json:"mime"`
}

type cacheIndex struct {
	Origin    string                    `json:"origin"`
	Resources map[string]cachedResource `json:"resources"`
}

// The desktop family uses CSS weight semantics independently of the website's
// OS/2 nearest-face ranges: normal text uses Regular and bold still uses Bold.
var faces = []struct{ name, weight string }{
	{"Regular", "100 449"}, {"Medium", "450 549"},
	{"Semibold", "550 649"}, {"Bold", "650 900"},
}

var manifestAsset = regexp.MustCompile(`^assets/[A-Za-z0-9_-]+\.(css|woff2)$`)

// Prepare validates cached files with the server and retains usable files on a
// network failure. Resource failures are reported individually; they leave the
// corresponding website URLs intact. The cache directory is runtime data and
// must remain outside the application's public source/assets distribution.
func Prepare(ctx context.Context, origin, directory string, installed ...string) (*Bundle, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("webassets: expected an HTTP origin")
	}
	origin = u.Scheme + "://" + u.Host
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	b := &Bundle{origin: origin, directory: directory, resources: map[string]cachedResource{}, rewrites: map[string]string{}}
	b.findInstalledFonts(installed)
	index := cacheIndex{Origin: origin, Resources: map[string]cachedResource{}}
	if data, err := os.ReadFile(filepath.Join(directory, "index.json")); err == nil {
		var previous cacheIndex
		if json.Unmarshal(data, &previous) == nil && previous.Origin == origin && previous.Resources != nil {
			index = previous
		}
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// Production Vite names these three public assets with a content hash.
	// Only their exact manifest keys and simple assets/ filenames are accepted.
	manifestSpec := resourceSpec{"public/manifest.json", "/vite/manifest.json", "public", "application/json", 2 << 20}
	manifest, _, manifestErr := b.fetch(ctx, client, manifestSpec, index.Resources[manifestSpec.path])
	if manifestErr == nil {
		index.Resources[manifestSpec.path] = manifest
	}
	var entries map[string]struct {
		File string `json:"file"`
	}
	if manifest.Bytes > 0 {
		data, _ := os.ReadFile(b.filename(manifestSpec.path))
		_ = json.Unmarshal(data, &entries)
	}
	specs := resourceSpecs(entries)
	type result struct {
		resource cachedResource
		source   string
		err      error
	}
	results := make(chan struct {
		index  int
		result result
	}, len(specs))
	for i, spec := range specs {
		previous := index.Resources[spec.path]
		go func() {
			r, source, err := b.fetch(ctx, client, spec, previous)
			results <- struct {
				index  int
				result result
			}{i, result{r, source, err}}
		}()
	}
	ordered := make([]result, len(specs))
	for range specs {
		result := <-results
		ordered[result.index] = result.result
	}
	for i, spec := range specs {
		result := ordered[i]
		r := result.resource
		ready := r.Bytes > 0
		b.report.Resources = append(b.report.Resources, ResourceReport{spec.path, spec.kind, r.Bytes, result.source, ready})
		if result.err != nil {
			b.report.Warnings = append(b.report.Warnings, spec.path+": "+result.err.Error())
		}
		if !ready {
			continue
		}
		index.Resources[spec.path] = r
		b.addResource(spec, r)
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err == nil {
		err = writeAtomic(filepath.Join(directory, "index.json"), data)
	}
	if err != nil {
		return nil, fmt.Errorf("webassets: save cache index: %w", err)
	}
	return b, nil
}

func resourceSpecs(entries map[string]struct {
	File string `json:"file"`
}) []resourceSpec {
	var specs []resourceSpec
	for _, face := range faces {
		name := "MiSans-" + face.name + ".woff2"
		specs = append(specs, resourceSpec{"private/fonts/" + name, "/client-assets/camp/fonts/misans-v1/" + name, "private", "font/woff2", 12 << 20})
	}
	for _, name := range []string{"tabler-icons-frontend.css", "tabler-icons-frontend.woff2", "tabler-icons.woff2"} {
		remote := "/vite/node_modules/icons-subsetter/built/" + name
		mime := "font/woff2"
		if strings.HasSuffix(name, ".css") {
			remote += "?direct"
			mime = "text/css; charset=utf-8"
		}
		if item, ok := entries["../icons-subsetter/built/"+name]; ok && manifestAsset.MatchString(item.File) && filepath.Ext(item.File) == filepath.Ext(name) {
			remote = "/vite/" + item.File
		}
		specs = append(specs, resourceSpec{"public/icons/" + name, remote, "public", mime, 2 << 20})
	}
	for _, name := range []string{"icon-v1.svg", "app-icon-192-v1.png", "splash-v1.png"} {
		mime := "image/png"
		if strings.HasSuffix(name, ".svg") {
			mime = "image/svg+xml"
		}
		specs = append(specs, resourceSpec{"public/branding/" + name, "/client-assets/trekker-camp/branding/" + name, "public", mime, 2 << 20})
	}
	for _, name := range []string{"apology-v1.webp", "apology-still-v1.webp"} {
		specs = append(specs, resourceSpec{"private/maintenance/" + name, "/client-assets/camp/maintenance/" + name, "private", "image/webp", 8 << 20})
	}
	return specs
}

func (b *Bundle) addResource(spec resourceSpec, r cachedResource) {
	b.resources["/"+spec.path] = r
	b.rewrites[spec.remote] = AssetBaseURL + spec.path
	b.rewrites[r.Remote] = AssetBaseURL + spec.path
	b.report.Bytes += r.Bytes
	if spec.kind == "private" {
		b.report.PrivateBytes += r.Bytes
	} else {
		b.report.PublicBytes += r.Bytes
	}
}

func (b *Bundle) filename(path string) string {
	if installed := b.installed[path]; installed != "" {
		return installed
	}
	return filepath.Join(b.directory, filepath.FromSlash(path))
}

func (b *Bundle) Report() Report {
	r := b.report
	r.Resources = append([]ResourceReport(nil), r.Resources...)
	r.Warnings = append([]string(nil), r.Warnings...)
	return r
}

// Handler exposes only prepared entries; it never serves the cache index or a
// directory listing. MyGo calls it with the original custom-scheme URL.
func (b *Bundle) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Del("Access-Control-Allow-Origin")
		w.Header().Del("Access-Control-Allow-Credentials")
		w.Header().Set("Vary", "Origin")
		if host := r.URL.Host; host != "localhost" && host != "camp-assets.localhost" {
			http.NotFound(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if origin != b.origin {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", b.origin)
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resource, ok := b.resources[r.URL.Path]
		if !ok || r.URL.RawQuery != "" || r.URL.RawPath != "" {
			http.NotFound(w, r)
			return
		}
		file, err := os.Open(b.filename(strings.TrimPrefix(r.URL.Path, "/")))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", resource.MIME)
		w.Header().Set("Cache-Control", "private, max-age=3600")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasSuffix(r.URL.Path, ".css") {
			data, err := os.ReadFile(b.filename(strings.TrimPrefix(r.URL.Path, "/")))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			css := b.iconCSS(string(data), resource.Remote)
			http.ServeContent(w, r, filepath.Base(r.URL.Path), info.ModTime(), strings.NewReader(css))
			return
		}
		if resource.ETag != "" {
			w.Header().Set("ETag", resource.ETag)
		}
		http.ServeContent(w, r, filepath.Base(r.URL.Path), info.ModTime(), file)
	})
}
