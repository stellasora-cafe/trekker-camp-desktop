// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webassets"
	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webfrontend"
	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webmedia"
)

func resourceClient(t *testing.T, origin string) *client {
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
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
	for name, data := range files {
		file, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	frontend, err := webfrontend.FromArchive(archive.Bytes(), origin)
	if err != nil {
		t.Fatal(err)
	}
	return &client{origin: origin, frontend: frontend}
}

func localResponse(c *client, method, raw string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c.serveLocalResource(w, httptest.NewRequest(method, raw, nil))
	return w
}

func TestLocalResourceFrontendPreservesOriginOffline(t *testing.T) {
	c := resourceClient(t, "https://offline.invalid")
	for _, path := range []string{"/", "/settings/preferences", "/notes/cached-note", "/@user.name"} {
		raw := c.origin + path
		if !c.localResource(raw) {
			t.Fatalf("bundled document not selected: %s", path)
		}
		w := localResponse(c, http.MethodGet, raw)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `content="https://offline.invalid"`) {
			t.Fatalf("bundled document changed origin or failed: %s %d", path, w.Code)
		}
	}
	raw := c.origin + "/vite/assets/worker.js?version=1"
	if !c.localResource(raw) {
		t.Fatal("bundled worker not selected")
	}
	w := localResponse(c, http.MethodGet, raw)
	if w.Code != http.StatusOK || w.Body.String() != "postMessage('ready')" || !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("bundled worker response: %d %s", w.Code, w.Header())
	}
	w = localResponse(c, http.MethodHead, raw)
	if w.Code != http.StatusOK || w.Body.Len() != 0 {
		t.Fatal("bundled HEAD response changed")
	}
	for _, raw := range []string{
		"https://another.invalid/settings", "https://offline.invalid:444/settings",
		"http://offline.invalid/settings", c.origin + "/api/client/i", c.origin + "/client-streaming",
		c.origin + "/files/photo.png", c.origin + "/client-assets/camp/unknown.png", c.origin + "/vite/missing.js",
	} {
		if c.localResource(raw) {
			t.Errorf("unexpected local interception: %s", raw)
		}
		if w := localResponse(c, http.MethodGet, raw); w.Code != http.StatusNotFound {
			t.Errorf("unexpected local response: %s %d", raw, w.Code)
		}
	}
	if w := localResponse(c, http.MethodPost, c.origin+"/settings"); w.Code != http.StatusMethodNotAllowed {
		t.Fatal("document interception must remain read-only")
	}
}

func TestWindowsPinFrontendUntilNextDocument(t *testing.T) {
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for name, content := range map[string]string{
		"bundle.json":         `{"version":"next","entry":"scripts/app.js","css":[],"langs":["zh-CN"]}`,
		"vite/scripts/app.js": "new", "vite/zh-CN/app.js": "new",
		"vite/loader/boot.js": "new", "vite/loader/style.css": "new",
	} {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data.Bytes())
	revision := hex.EncodeToString(hash[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == webfrontend.UpdatePath {
			_ = json.NewEncoder(w).Encode(webfrontend.Release{Revision: revision, SHA256: revision, Size: int64(data.Len()), HostAPI: webfrontend.HostAPI, Archive: "/_desktop/frontend/" + revision + ".zip"})
		} else {
			_, _ = w.Write(data.Bytes())
		}
	}))
	defer server.Close()
	c := resourceClient(t, server.URL)
	var err error
	c.updates, err = webfrontend.OpenUpdates(server.URL, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	oldWindow := c.pageOptions(false, false, func() bool { return false })
	liveRevision, err := c.updates.AvailableRevision(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	live := &frontendWindow{onlineRevision: liveRevision}
	live.online.Store(true)
	c.mainFrontend = live
	onlineMain := c.pageOptions(false, false, live.online.Load)
	staged, err := c.updates.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ready, err := c.stageFrontend(staged); err != nil || ready || live.pending != "" {
		t.Fatalf("downloaded copy of the online frontend must not offer a refresh: ready=%v, err=%v", ready, err)
	}
	if err := c.updates.Activate(staged); err != nil {
		t.Fatal(err)
	}
	get := func(path string) string {
		w := httptest.NewRecorder()
		oldWindow.ResourceHandler.ServeHTTP(w, httptest.NewRequest("GET", server.URL+path, nil))
		return w.Body.String()
	}
	if body := get("/vite/scripts/app.js"); body != "import('./detail.js');" {
		t.Fatal("live window mixed frontend versions")
	}
	if body := get("/settings"); !strings.Contains(body, revision) {
		t.Fatal("new document did not select updated bundle")
	}
	if body := get("/vite/scripts/app.js"); body != "new" {
		t.Fatal("new document kept old script")
	}
	if oldWindow.ResourceFilter(server.URL + webfrontend.UpdatePath) {
		t.Fatal("update endpoint intercepted by SPA")
	}
	for _, path := range []string{"/", "/vite/scripts/app.js"} {
		if onlineMain.ResourceFilter(server.URL + path) {
			t.Fatalf("main window after sign-in must still load %s from the server even with a full cache", path)
		}
	}
	live.online.Store(false)
	if !onlineMain.ResourceFilter(server.URL + "/") {
		t.Fatal("explicit frontend apply must let the running window load the activated cache")
	}
	// Opening sign-in or registration after the main window has used a cached
	// bundle must still hit the server, including its maintenance redirects.
	for _, mode := range []string{"signin", "signup"} {
		target := server.URL + "/desktop-auth?mode=" + mode
		authWindow := c.pageOptions(true, false, func() bool { return isAuthTool(target) })
		for _, raw := range []string{target, server.URL + "/vite/scripts/app.js"} {
			if authWindow.ResourceFilter(raw) {
				t.Fatalf("auth window intercepted %s with a cached frontend", raw)
			}
		}
	}
}

func TestLocalResourcePrivateCacheAfterServerStops(t *testing.T) {
	font := "wOF2cached-font-fixture"
	artwork := "RIFF0000WEBPcached-maintenance-fixture"
	fontPath := "/client-assets/camp/fonts/misans-v1/MiSans-Regular.woff2"
	artworkPath := "/client-assets/camp/maintenance/apology-v1.webp"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case fontPath:
			w.Header().Set("Content-Type", "font/woff2")
			_, _ = w.Write([]byte(font))
		case artworkPath:
			w.Header().Set("Content-Type", "image/webp")
			_, _ = w.Write([]byte(artwork))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	directory := t.TempDir()
	if _, err := webassets.Prepare(context.Background(), server.URL, directory); err != nil {
		t.Fatal(err)
	}
	server.Close()
	c := resourceClient(t, server.URL)
	cached, err := webassets.LoadCached(server.URL, directory)
	if err != nil {
		t.Fatal(err)
	}
	c.assets.Store(cached)
	for path, body := range map[string]string{fontPath: font, artworkPath: artwork} {
		raw := server.URL + path
		if !c.localResource(raw) {
			t.Fatalf("cached private resource not selected: %s", path)
		}
		w := localResponse(c, http.MethodGet, raw)
		if w.Code != http.StatusOK || w.Body.String() != body {
			t.Fatalf("private resource did not survive offline restart: %s %d", path, w.Code)
		}
		if w := localResponse(c, http.MethodHead, raw); w.Code != http.StatusOK || w.Body.Len() != 0 {
			t.Fatalf("private resource HEAD: %s %d", path, w.Code)
		}
		if w := localResponse(c, http.MethodPost, raw); w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("private resource POST: %s %d", path, w.Code)
		}
	}
	if c.localResource("https://outside.invalid" + artworkPath) {
		t.Fatal("private cache crossed origin")
	}
}

func TestLocalResourceMediaCacheKeepsOriginalURL(t *testing.T) {
	body := "\x89PNG\r\n\x1a\ncached-image-fixture"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/files/photo.png?size=small" {
			t.Errorf("image URL changed: %s", r.URL.RequestURI())
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	directory := t.TempDir()
	c := resourceClient(t, server.URL)
	c.media = webmedia.New(server.URL, directory)
	raw := server.URL + "/files/photo.png?size=small"
	if !c.localResource(raw) {
		t.Fatal("same-origin media was not selected")
	}
	if w := localResponse(c, http.MethodGet, raw); w.Code != http.StatusOK || w.Body.String() != body {
		t.Fatal("media was not forwarded intact")
	}
	server.Close()
	c.media = webmedia.New(server.URL, directory)
	if w := localResponse(c, http.MethodGet, raw); w.Code != http.StatusOK || w.Body.String() != body || w.Header().Get("Content-Type") != "image/png" {
		t.Fatal("cached media was not returned after restart without the server")
	}
	if c.localResource("https://outside.invalid/files/photo.png?size=small") || c.localResource(server.URL+"/api/client/i") {
		t.Fatal("media cache selected another origin or an API request")
	}
}
