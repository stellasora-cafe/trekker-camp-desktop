// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webassets

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// LoadCached is a read-only first-paint path. It never waits on the network,
// creates directories, or rewrites the index. Prepare can refresh it later.
func LoadCached(origin, directory string, installed ...string) (*Bundle, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("webassets: expected an HTTP origin")
	}
	origin = u.Scheme + "://" + u.Host
	b := &Bundle{origin: origin, directory: directory, resources: map[string]cachedResource{}, rewrites: map[string]string{}}
	b.findInstalledFonts(installed)
	data, err := os.ReadFile(filepath.Join(directory, "index.json"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var index cacheIndex
	if json.Unmarshal(data, &index) != nil || index.Origin != origin {
		index = cacheIndex{Origin: origin}
	}
	var entries map[string]struct {
		File string `json:"file"`
	}
	if manifest, err := os.ReadFile(b.filename("public/manifest.json")); err == nil {
		_ = json.Unmarshal(manifest, &entries)
	}
	for _, spec := range resourceSpecs(entries) {
		if r, ok := b.installedFont(spec); ok {
			b.addResource(spec, r)
			b.report.Resources = append(b.report.Resources, ResourceReport{spec.path, spec.kind, r.Bytes, "installed", true})
			continue
		}
		r, ok := index.Resources[spec.path]
		if !ok || r.Bytes <= 0 || r.Bytes > spec.limit || r.MIME != spec.mime {
			continue
		}
		remoteAllowed := r.Remote == spec.remote
		if strings.HasPrefix(spec.path, "public/icons/") {
			remoteAllowed = remoteAllowed || (strings.HasPrefix(r.Remote, "/vite/") && manifestAsset.MatchString(strings.TrimPrefix(r.Remote, "/vite/")) && filepath.Ext(r.Remote) == filepath.Ext(spec.path))
		}
		if !remoteAllowed {
			continue
		}
		info, err := os.Stat(b.filename(spec.path))
		if err != nil || !info.Mode().IsRegular() || info.Size() != r.Bytes {
			continue
		}
		b.addResource(spec, r)
		b.report.Resources = append(b.report.Resources, ResourceReport{spec.path, spec.kind, r.Bytes, "cache", true})
	}
	return b, nil
}

// CachedURL resolves an original site URL to a prepared local resource URL.
// An empty result means the original request must use its normal source.
func (b *Bundle) CachedURL(raw string) string { return b.rewriteURL(raw) }
