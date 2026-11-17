// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webfrontend

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, extra map[string]string) []byte {
	t.Helper()
	files := map[string]string{
		"bundle.json":           `{"version":"2026.11.0","entry":"scripts/app.js","css":["assets/app.css"],"langs":["zh-CN","zh-TW"]}`,
		"vite/scripts/app.js":   "import('./detail.js');",
		"vite/zh-CN/app.js":     "console.log('中文');",
		"vite/zh-TW/app.js":     "console.log('中文');",
		"vite/assets/app.css":   "body{color:red}",
		"vite/assets/worker.js": "postMessage('ready')",
		"vite/loader/boot.js":   "import(CLIENT_ENTRY)",
		"vite/loader/style.css": "#splash{display:block}",
	}
	for name, content := range extra {
		files[name] = content
	}
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	for name, content := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func TestOfflineDocumentAndAssets(t *testing.T) {
	// Previously downloaded archives can render without the upstream server.
	b, err := FromArchive(fixture(t, nil), "https://offline.invalid")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`content="https://offline.invalid"`, `const CLIENT_ENTRY="scripts/app.js"`, `const LANGS=["zh-CN","zh-TW"]`, `/vite/assets/app.css`, `/vite/loader/boot.js`} {
		if !strings.Contains(b.HTML(), expected) {
			t.Errorf("document missing %s", expected)
		}
	}
	for _, pathname := range []string{"/", "/settings/preferences", "/notes/local-note", "/@user.name"} {
		w := httptest.NewRecorder()
		b.ServeHTTP(w, httptest.NewRequest(http.MethodGet, pathname, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "<!doctype html>") {
			t.Errorf("document %s: %d", pathname, w.Code)
		}
	}
	for _, pathname := range []string{"/vite/scripts/app.js", "/vite/zh-CN/app.js", "/vite/assets/worker.js"} {
		w := httptest.NewRecorder()
		b.ServeHTTP(w, httptest.NewRequest(http.MethodGet, pathname, nil))
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
			t.Errorf("module %s: %d %s", pathname, w.Code, w.Header())
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/vite/assets/worker.js", nil)
	r.Header.Set("Range", "bytes=0-3")
	b.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "post" {
		t.Fatalf("range response: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	b.ServeHTTP(w, httptest.NewRequest(http.MethodHead, "/vite/loader/boot.js", nil))
	if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("boot HEAD: %d %s", w.Code, w.Header())
	}
}

func TestNoAPIMediaOrPrivateFallback(t *testing.T) {
	b, err := FromArchive(fixture(t, nil), "https://offline.invalid")
	if err != nil {
		t.Fatal(err)
	}
	for _, pathname := range []string{"/api/meta", "/api/client/i", "/client-streaming", "/streaming", "/files/file", "/proxy/image", "/client-assets/camp/private.png", "/vite/missing.js", "/bundle.json", "/vite/../bundle.json", "/vite//scripts/app.js"} {
		w := httptest.NewRecorder()
		b.ServeHTTP(w, httptest.NewRequest(http.MethodGet, pathname, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s returned %d", pathname, w.Code)
		}
	}
	w := httptest.NewRecorder()
	b.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/settings", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST returned %d", w.Code)
	}
}

func TestRejectIncompleteOrPrivateArchive(t *testing.T) {
	for _, files := range []map[string]string{
		{"client-assets/camp/secret.png": "private"},
		{"../escape.js": "bad"},
		{"bundle.json": `{"version":"1","entry":"scripts/missing.js","langs":["zh-CN"]}`},
	} {
		if _, err := FromArchive(fixture(t, files), "https://offline.invalid"); err == nil {
			t.Fatal("accepted invalid archive")
		}
	}
	for _, origin := range []string{"file:///tmp/site", "https://user:pass@camp.invalid", "https://camp.invalid/path"} {
		if _, err := FromArchive(fixture(t, nil), origin); err == nil {
			t.Errorf("accepted origin %s", origin)
		}
	}
}

func TestServerManifestIsComplete(t *testing.T) {
	archive := os.Getenv("CAMP_TEST_FRONTEND_ARCHIVE")
	if archive == "" {
		t.Skip("set CAMP_TEST_FRONTEND_ARCHIVE to validate a server release")
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	b, err := FromArchive(data, "https://camp.stellasora.cafe")
	if err != nil {
		t.Fatal(err)
	}
	data, err = fs.ReadFile(b.files, "vite/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]struct {
		File           string   `json:"file"`
		CSS            []string `json:"css"`
		Assets         []string `json:"assets"`
		Imports        []string `json:"imports"`
		DynamicImports []string `json:"dynamicImports"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for key, chunk := range manifest {
		for _, file := range append(append([]string{chunk.File}, chunk.CSS...), chunk.Assets...) {
			if !b.Contains("/vite/" + file) {
				t.Errorf("%s refers to absent file %s", key, file)
			}
		}
		for _, imported := range append(chunk.Imports, chunk.DynamicImports...) {
			if _, ok := manifest[imported]; !ok {
				t.Errorf("%s refers to absent chunk %s", key, imported)
			}
		}
	}
	if len(manifest) < 100 {
		t.Errorf("incomplete frontend: %d chunks", len(manifest))
	}
}
