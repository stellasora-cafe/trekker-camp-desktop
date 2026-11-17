// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webassets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

const testIconCSS = `@font-face{font-family:"tabler-icons";src:url("./tabler-icons.woff2")}@font-face{font-family:"tabler-icons";src:url("./tabler-icons-frontend.woff2");unicode-range:U+ea02}.ti{font-family:"tabler-icons" !important}.ti-ad:before{content:"\ea02"}`

func fixtureResource(path string) []byte {
	switch {
	case strings.HasSuffix(path, ".woff2"):
		return []byte("wOF2fixture-font")
	case strings.HasSuffix(path, ".css"):
		return []byte(testIconCSS)
	case strings.HasSuffix(path, ".svg"):
		return []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)
	case strings.HasSuffix(path, ".png"):
		return []byte("\x89PNG\r\n\x1a\nfixture")
	case strings.HasSuffix(path, ".webp"):
		return []byte("RIFF0000WEBPfixture")
	}
	return nil
}

func fixtureServer(t *testing.T, handler func(http.ResponseWriter, *http.Request) bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Camp-Client") != "" {
			t.Errorf("resource request included unexpected method or account headers")
		}
		if handler != nil && handler(w, r) {
			return
		}
		data := fixtureResource(r.URL.Path)
		if data == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("ETag", `"fixture-v1"`)
		w.Header().Set("Last-Modified", "Sat, 14 Nov 2026 01:00:00 GMT")
		if r.Header.Get("If-None-Match") == `"fixture-v1"` {
			if r.Header.Get("If-Modified-Since") == "" {
				t.Error("Last-Modified validator missing")
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write(data)
	}))
}

func TestPrepareRevalidatesAndUsesOfflineCache(t *testing.T) {
	server := fixtureServer(t, nil)
	directory := t.TempDir()
	first, err := Prepare(context.Background(), server.URL, directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Report().Resources) != 12 || len(first.Report().Warnings) != 0 {
		t.Fatalf("initial report: %+v", first.Report())
	}
	for _, r := range first.Report().Resources {
		if !r.Ready || r.Source != "download" {
			t.Errorf("first resource: %+v", r)
		}
	}
	second, err := Prepare(context.Background(), server.URL, directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range second.Report().Resources {
		if r.Source != "validated" {
			t.Errorf("revalidated resource: %+v", r)
		}
	}
	server.Close()
	third, err := Prepare(context.Background(), server.URL, directory)
	if err != nil {
		t.Fatal(err)
	}
	if third.Report().Bytes != first.Report().Bytes {
		t.Fatal("offline cache lost bytes")
	}
	for _, r := range third.Report().Resources {
		if !r.Ready || r.Source != "cache" {
			t.Errorf("offline resource: %+v", r)
		}
		if !strings.HasPrefix(r.Path, r.Kind+"/") {
			t.Errorf("resource crossed public/private partition: %+v", r)
		}
	}
	if third.Report().Bytes != third.Report().PublicBytes+third.Report().PrivateBytes {
		t.Fatal("reported byte totals differ")
	}
}

func TestPrepareProductionManifestOnlyUsesKnownAssetEntries(t *testing.T) {
	var unexpected atomic.Bool
	server := fixtureServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/vite/manifest.json" {
			_, _ = fmt.Fprint(w, `{"../icons-subsetter/built/tabler-icons-frontend.css":{"file":"assets/icons-123.css"},"../icons-subsetter/built/tabler-icons-frontend.woff2":{"file":"assets/subset-123.woff2"},"../icons-subsetter/built/tabler-icons.woff2":{"file":"assets/full-123.woff2"},"unrelated":{"file":"https://invalid.example/tracker"}}`)
			return true
		}
		if strings.Contains(r.URL.Path, "node_modules") {
			unexpected.Store(true)
		}
		if r.URL.Path == "/vite/assets/icons-123.css" {
			_, _ = fmt.Fprint(w, `@font-face{font-family:"tabler-icons";src:url('/vite/assets/full-123.woff2')}@font-face{font-family:"tabler-icons";src:url('/vite/assets/subset-123.woff2');unicode-range:U+ea02}`)
			return true
		}
		return false
	})
	defer server.Close()
	bundle, err := Prepare(context.Background(), server.URL, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if unexpected.Load() {
		t.Fatal("production ignored manifest")
	}
	css := bundle.Style()
	for _, name := range []string{"tabler-icons.woff2", "tabler-icons-frontend.woff2"} {
		if !strings.Contains(css, AssetBaseURL+"public/icons/"+name) {
			t.Errorf("missing local %s", name)
		}
	}
	if !strings.Contains(css, "unicode-range:U+ea02") {
		t.Fatal("icon subset range lost")
	}
}

func TestPrepareKeepsUpdatedDataAndRefusesRedirects(t *testing.T) {
	var second atomic.Bool
	var redirectHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirectHits.Add(1) }))
	defer other.Close()
	server := fixtureServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "MiSans-Regular.woff2") && second.Load() {
			if r.Header.Get("If-None-Match") == "" {
				t.Error("updated resource omitted validator")
			}
			w.Header().Set("ETag", `"v2"`)
			_, _ = fmt.Fprint(w, "wOF2updated-font-longer")
			return true
		}
		if strings.HasSuffix(r.URL.Path, "MiSans-Bold.woff2") {
			http.Redirect(w, r, other.URL+"/font.woff2", http.StatusFound)
			return true
		}
		return false
	})
	defer server.Close()
	directory := t.TempDir()
	if _, err := Prepare(context.Background(), server.URL, directory); err != nil {
		t.Fatal(err)
	}
	second.Store(true)
	bundle, err := Prepare(context.Background(), server.URL, directory)
	if err != nil {
		t.Fatal(err)
	}
	if redirectHits.Load() != 0 {
		t.Fatal("followed resource redirect")
	}
	data, err := os.ReadFile(filepath.Join(directory, "private", "fonts", "MiSans-Regular.woff2"))
	if err != nil || string(data) != "wOF2updated-font-longer" {
		t.Fatalf("resource update: %q, %v", data, err)
	}
	if !strings.Contains(bundle.Style(), server.URL+"/client-assets/camp/fonts/misans-v1/MiSans-Bold.woff2") {
		t.Fatal("missing font did not fall back to original URL")
	}
}

func TestHandlerAllowlistAndCORS(t *testing.T) {
	server := fixtureServer(t, nil)
	defer server.Close()
	bundle, err := Prepare(context.Background(), server.URL, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, url, origin string
		status              int
	}{
		{"GET", AssetBaseURL + "private/fonts/MiSans-Regular.woff2", server.URL, 200},
		{"HEAD", "camp-assets://localhost/public/branding/icon-v1.svg", server.URL, 200},
		{"GET", AssetBaseURL + "public/icons/tabler-icons-frontend.css", server.URL, 200},
		{"GET", AssetBaseURL + "index.json", server.URL, 404},
		{"GET", AssetBaseURL + "private/fonts/../../index.json", server.URL, 404},
		{"GET", AssetBaseURL + "private/fonts/%4diSans-Regular.woff2", server.URL, 404},
		{"GET", AssetBaseURL + "private/fonts/MiSans-Regular.woff2?extra=1", server.URL, 404},
		{"GET", "camp-assets://other/private/fonts/MiSans-Regular.woff2", server.URL, 404},
		{"POST", AssetBaseURL + "private/fonts/MiSans-Regular.woff2", server.URL, 405},
		{"GET", AssetBaseURL + "private/fonts/MiSans-Regular.woff2", "https://foreign.example", 403},
	} {
		r := httptest.NewRequest(tc.method, tc.url, nil)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		bundle.Handler().ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s %s: got %d want %d", tc.method, tc.url, w.Code, tc.status)
		}
		if tc.status == 200 && w.Header().Get("Access-Control-Allow-Origin") != server.URL {
			t.Error("font CORS origin differs")
		}
		if tc.status == 403 && w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("foreign origin allowed")
		}
		if strings.Contains(tc.url, "frontend.css") && !strings.Contains(w.Body.String(), "Camp Cached Tabler") {
			t.Error("served CSS omitted family alias")
		}
	}
}

func TestHTMLKeepsApplicationURLsAndAddsReusableStyles(t *testing.T) {
	server := fixtureServer(t, nil)
	defer server.Close()
	bundle, err := Prepare(context.Background(), server.URL, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := `<!doctype html><html><head><link rel="stylesheet" href="/vite/node_modules/icons-subsetter/built/tabler-icons-frontend.css?direct"><link rel="icon" href="/client-assets/trekker-camp/branding/icon-v1.svg"><script type="module" src="/vite/src/_boot_.ts"></script><script>const worker='/vite/worker.js';</script></head><body><img src="https://other.example/client-assets/trekker-camp/branding/icon-v1.svg"></body></html>`
	output, err := bundle.HTML(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{AssetBaseURL + "public/icons/tabler-icons-frontend.css", AssetBaseURL + "public/branding/icon-v1.svg", `src="/vite/src/_boot_.ts"`, `const worker='/vite/worker.js';`, `https://other.example/client-assets/trekker-camp/branding/icon-v1.svg`, `id="camp-local-assets"`, "Camp Cached Tabler", "Camp Cached MiSans"} {
		if !strings.Contains(output, expected) {
			t.Errorf("missing %q", expected)
		}
	}
	if !strings.Contains(output, bundle.Style()) {
		t.Fatal("HTML and navigation CSS differ")
	}
	if !strings.Contains(bundle.Style(), ":root .ti{font-family:'Camp Cached Tabler','tabler-icons' !important}") {
		t.Fatal("local icon font must outrank the website's later important rule")
	}
	second, err := bundle.HTML(output)
	if err != nil || strings.Count(second, `id="camp-local-assets"`) != 1 {
		t.Fatal("HTML rewrite is not idempotent")
	}
	if _, err := bundle.HTML("<body>missing head</body>"); err == nil {
		t.Fatal("missing head accepted")
	}
	encoded, err := json.Marshal(bundle.Report())
	if err != nil || strings.Contains(string(encoded), "token") {
		t.Fatal("report serialization invalid")
	}
}

func TestPrepareRejectsNonOrigin(t *testing.T) {
	for _, origin := range []string{"file:///tmp", "https://user:password@example.com", "https://example.com/path", "https://example.com/?token=secret"} {
		if _, err := Prepare(context.Background(), origin, t.TempDir()); err == nil {
			t.Errorf("accepted %q", origin)
		}
	}
}

func TestDesktopFontWeightSelectionWithMissingCache(t *testing.T) {
	faceRule := regexp.MustCompile(`src:url\('([^']+)'\).*?font-weight:(\d+) (\d+)`)
	names := []string{"Regular", "Medium", "Semibold", "Bold"}
	for _, cached := range []int{4, 1, 0} {
		t.Run(fmt.Sprintf("cached-%d", cached), func(t *testing.T) {
			bundle := &Bundle{origin: "http://localhost:3000", resources: map[string]cachedResource{}}
			for _, name := range names[:cached] {
				bundle.resources["/private/fonts/MiSans-"+name+".woff2"] = cachedResource{}
			}
			css := bundle.Style()
			rules := faceRule.FindAllStringSubmatch(css, -1)
			if len(rules) != 4 {
				t.Fatalf("got %d faces, want all four even without cached files", len(rules))
			}
			for weight := 100; weight <= 900; weight++ {
				matches := 0
				for _, rule := range rules {
					lower, _ := strconv.Atoi(rule[2])
					upper, _ := strconv.Atoi(rule[3])
					if weight >= lower && weight <= upper {
						matches++
					}
				}
				if matches != 1 {
					t.Fatalf("weight %d matches %d faces", weight, matches)
				}
			}
			for i, weight := range []int{400, 500, 600, 700} {
				want := bundle.origin + "/client-assets/camp/fonts/misans-v1/MiSans-" + names[i] + ".woff2"
				if i < cached {
					want = AssetBaseURL + "private/fonts/MiSans-" + names[i] + ".woff2"
				}
				for _, rule := range rules {
					lower, _ := strconv.Atoi(rule[2])
					upper, _ := strconv.Atoi(rule[3])
					if weight >= lower && weight <= upper && rule[1] != want {
						t.Errorf("weight %d selects %s, want %s", weight, rule[1], want)
					}
				}
			}
			if strings.Contains(css, "font-size:") || strings.Contains(css, "line-height:") {
				t.Fatal("font mapping changed text geometry")
			}
		})
	}
}
