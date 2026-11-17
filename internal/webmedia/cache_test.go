// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webmedia

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var imageBytes = []byte("\x89PNG\r\n\x1a\ncached-image-fixture")

func request(c *Cache, method, target string, headers http.Header) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	if headers != nil {
		r.Header = headers
	}
	w := httptest.NewRecorder()
	c.ServeHTTP(w, r)
	return w
}

func imageServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("ETag", "\"public-image\"")
		w.Header().Set("Last-Modified", "Sat, 14 Nov 2026 01:00:00 GMT")
		if r.Header.Get("If-None-Match") == "\"public-image\"" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write(imageBytes)
	}))
}

func TestImageSurvivesRestartAndOfflineRangeAndHead(t *testing.T) {
	server := imageServer()
	defer server.Close()
	directory := t.TempDir()
	target := server.URL + "/files/photo.png"
	c := New(server.URL, directory)
	first := request(c, http.MethodGet, target, nil)
	if first.Code != http.StatusOK || !bytes.Equal(first.Body.Bytes(), imageBytes) {
		t.Fatal("online image was not forwarded")
	}
	server.Close()
	restored := New(server.URL, directory)
	offline := request(restored, http.MethodGet, target, nil)
	if offline.Code != http.StatusOK || !bytes.Equal(offline.Body.Bytes(), imageBytes) || offline.Header().Get("Content-Type") != "image/png" {
		t.Fatal("offline image was not restored")
	}
	partial := request(restored, http.MethodGet, target, http.Header{"Range": {"bytes=2-5"}})
	if partial.Code != http.StatusPartialContent || !bytes.Equal(partial.Body.Bytes(), imageBytes[2:6]) {
		t.Fatal("offline range was not served")
	}
	head := request(restored, http.MethodHead, target, nil)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != fmt.Sprint(len(imageBytes)) {
		t.Fatal("offline HEAD did not retain the image length")
	}
}

func TestOfflineImagePreservesEncodingAndDocumentRestrictions(t *testing.T) {
	var body bytes.Buffer
	compressed := gzip.NewWriter(&body)
	_, _ = compressed.Write([]byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>"))
	_ = compressed.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Disposition", "attachment; filename=image.svg")
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(body.Bytes())
	}))
	defer server.Close()
	c := New(server.URL, t.TempDir())
	target := server.URL + "/files/image.svg"
	headers := http.Header{"Accept-Encoding": {"gzip"}}
	request(c, http.MethodGet, target, headers)
	server.Close()
	offline := request(c, http.MethodGet, target, headers)
	if offline.Code != http.StatusOK || !bytes.Equal(offline.Body.Bytes(), body.Bytes()) ||
		offline.Header().Get("Content-Encoding") != "gzip" ||
		offline.Header().Get("Content-Disposition") != "attachment; filename=image.svg" ||
		offline.Header().Get("Content-Security-Policy") != "sandbox; default-src 'none'" ||
		offline.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("cached image changed its encoding or document security")
	}
}

func TestImageRevalidatesAndAtomicallyReplacesOldBytes(t *testing.T) {
	var version atomic.Int32
	var conditional atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		etag := fmt.Sprintf("\"%d\"", version.Load())
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			conditional.Store(true)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write(append(bytes.Clone(imageBytes), byte(version.Load())))
	}))
	defer server.Close()
	c := New(server.URL, t.TempDir())
	target := server.URL + "/avatar/user"
	request(c, http.MethodGet, target, nil)
	again := request(c, http.MethodGet, target, nil)
	if !conditional.Load() || again.Code != http.StatusOK || again.Body.Len() != len(imageBytes)+1 {
		t.Fatal("cached image was not revalidated")
	}
	clientConditional := request(c, http.MethodGet, target, http.Header{"If-None-Match": {"\"0\""}})
	if clientConditional.Code != http.StatusNotModified || clientConditional.Body.Len() != 0 {
		t.Fatal("browser conditional request semantics changed")
	}
	version.Store(1)
	fresh := request(c, http.MethodGet, target, nil)
	server.Close()
	offline := request(c, http.MethodGet, target, nil)
	if fresh.Code != http.StatusOK || !bytes.Equal(offline.Body.Bytes(), append(bytes.Clone(imageBytes), byte(1))) {
		t.Fatal("replacement was not persisted atomically")
	}
}

func TestOnlyTemporaryServerFailuresUseCachedImages(t *testing.T) {
	var status atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		if code := status.Load(); code != 0 {
			w.WriteHeader(int(code))
			return
		}
		_, _ = w.Write(imageBytes)
	}))
	defer server.Close()
	c := New(server.URL, t.TempDir())
	target := server.URL + "/proxy/photo.webp"
	request(c, http.MethodGet, target, nil)
	for _, code := range []int{502, 503, 504} {
		status.Store(int32(code))
		w := request(c, http.MethodGet, target, nil)
		if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), imageBytes) {
			t.Errorf("temporary status %d did not retain cached content", code)
		}
	}
	status.Store(500)
	if w := request(c, http.MethodGet, target, nil); w.Code != 500 {
		t.Fatal("a real non-transient response was hidden")
	}
}

func TestAccessDenialsAndDeletionInvalidateOldImages(t *testing.T) {
	for _, code := range []int{401, 403, 404} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			var rejected atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if rejected.Load() {
					w.WriteHeader(code)
					return
				}
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(imageBytes)
			}))
			defer server.Close()
			c := New(server.URL, t.TempDir())
			target := server.URL + "/files/photo.png"
			request(c, http.MethodGet, target, nil)
			rejected.Store(true)
			if w := request(c, http.MethodGet, target, nil); w.Code != code || w.Body.Len() != 0 {
				t.Fatal("access denial was replaced with old content")
			}
			server.Close()
			if w := request(c, http.MethodGet, target, nil); w.Code != http.StatusBadGateway {
				t.Fatal("rejected content remained available offline")
			}
		})
	}
}

func TestCredentialsNeverReadOrPopulateTheCache(t *testing.T) {
	for _, header := range []string{"Authorization", "Cookie", "authorization", "cookie"} {
		t.Run(header, func(t *testing.T) {
			server := imageServer()
			defer server.Close()
			c := New(server.URL, t.TempDir())
			public := server.URL + "/files/public.png"
			private := server.URL + "/files/private.png"
			request(c, http.MethodGet, public, nil)
			headers := http.Header{header: {"test-credential"}}
			if w := request(c, http.MethodGet, private, headers); w.Code != http.StatusOK {
				t.Fatal("authenticated request was not forwarded")
			}
			server.Close()
			if w := request(c, http.MethodGet, public, headers); w.Code != http.StatusBadGateway {
				t.Fatal("authenticated request received anonymous cached bytes")
			}
			if w := request(c, http.MethodGet, private, nil); w.Code != http.StatusBadGateway {
				t.Fatal("authenticated response entered the public cache")
			}
			if w := request(c, http.MethodGet, public, nil); w.Code != http.StatusOK {
				t.Fatal("public cache was unexpectedly removed")
			}
		})
	}
}

func TestNonPublicNonImageAndOversizedResponsesAreNotCached(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		body    []byte
	}{
		{"private", http.Header{"Content-Type": {"image/png"}, "Cache-Control": {"private, max-age=30"}}, imageBytes},
		{"no-store", http.Header{"Content-Type": {"image/png"}, "Cache-Control": {"no-store"}}, imageBytes},
		{"set-cookie", http.Header{"Content-Type": {"image/png"}, "Set-Cookie": {"session=test"}}, imageBytes},
		{"video", http.Header{"Content-Type": {"video/mp4"}}, []byte("video")},
		{"html", http.Header{"Content-Type": {"text/html"}}, []byte("<html>")},
		{"oversized", http.Header{"Content-Type": {"image/png"}}, bytes.Repeat([]byte("x"), int(maxImageBytes)+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for key, values := range tt.headers {
					w.Header()[key] = values
				}
				_, _ = w.Write(tt.body)
			}))
			defer server.Close()
			c := New(server.URL, t.TempDir())
			target := server.URL + "/files/media"
			w := request(c, http.MethodGet, target, nil)
			if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), tt.body) {
				t.Fatal("uncached response was not forwarded intact")
			}
			server.Close()
			if w := request(c, http.MethodGet, target, nil); w.Code != http.StatusBadGateway {
				t.Fatal("non-public or unsupported content entered the cache")
			}
		})
	}
}

func TestOriginIsolationAndPathRejection(t *testing.T) {
	first, second := imageServer(), imageServer()
	defer first.Close()
	defer second.Close()
	directory := t.TempDir()
	a, b := New(first.URL, directory), New(second.URL, directory)
	request(a, http.MethodGet, first.URL+"/files/same.png", nil)
	first.Close()
	second.Close()
	if w := request(b, http.MethodGet, second.URL+"/files/same.png", nil); w.Code != http.StatusBadGateway {
		t.Fatal("another origin reused cached image bytes")
	}
	if w := request(a, http.MethodGet, first.URL+"/files/same.png", nil); w.Code != http.StatusOK {
		t.Fatal("original server cache disappeared")
	}
	for _, target := range []string{
		first.URL + "/files/../api/i", first.URL + "/files/%2e%2e/api/i",
		first.URL + "/files/%2f..%2fapi/i", first.URL + "/files//image",
		first.URL + "/files/..\\api", first.URL + "/files/image%00",
		first.URL + "/files-elsewhere/image", first.URL + "/api/identicon",
		first.URL + "/files/image#fragment", second.URL + "/files/image",
		"/files/image", "file:///files/image",
	} {
		if a.Matches(target) {
			t.Errorf("unsafe or unrelated path matched: %s", target)
		}
	}
	if New("https://user:password@example.com", directory).Matches("https://example.com/files/image") {
		t.Fatal("credential-bearing origin was accepted")
	}
}

func TestRedirectsStayWithWebViewAndWritesAreBlocked(t *testing.T) {
	var followed, writes atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Add(1) }))
	defer destination.Close()
	tlsDestination := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Add(1) }))
	defer tlsDestination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes.Add(1)
		}
		target := destination.URL + "/files/image"
		if r.URL.Path == "/identicon/https" {
			target = tlsDestination.URL + "/public/avatar.png"
		}
		http.Redirect(w, r, target, http.StatusFound)
	}))
	defer server.Close()
	c := New(server.URL, t.TempDir())
	for _, tc := range []struct{ path, location string }{
		{"/identicon/http", destination.URL + "/files/image"},
		{"/identicon/https", tlsDestination.URL + "/public/avatar.png"},
	} {
		for _, headers := range []http.Header{nil, {"Authorization": {"Bearer local-session"}, "Cookie": {"session=local-only"}}} {
			w := request(c, http.MethodGet, server.URL+tc.path, headers)
			if w.Code != http.StatusFound || w.Header().Get("Location") != tc.location {
				t.Fatalf("HTTP(S) redirect was not preserved: status %d, location %q", w.Code, w.Header().Get("Location"))
			}
		}
	}
	if followed.Load() != 0 {
		t.Fatal("proxy followed a redirect instead of leaving it to the WebView")
	}
	if files, _ := os.ReadDir(c.directory); len(files) != 0 {
		t.Fatal("redirect response was written to the image cache")
	}
	if w := request(c, http.MethodPost, server.URL+"/files/image", nil); w.Code != http.StatusMethodNotAllowed || writes.Load() != 0 {
		t.Fatal("write request was handled as an image")
	}
	server.Close()
	if w := request(c, http.MethodGet, server.URL+"/identicon/http", nil); w.Code != http.StatusBadGateway {
		t.Fatal("redirect was returned from the offline image cache")
	}
}

func TestNonHTTPRedirectsAreBlocked(t *testing.T) {
	for _, location := range []string{"file:///tmp/avatar.png", "javascript:alert(1)", "https://user:password@example.com/avatar.png"} {
		t.Run(location, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", location)
				w.WriteHeader(http.StatusFound)
			}))
			defer server.Close()
			c := New(server.URL, t.TempDir())
			w := request(c, http.MethodGet, server.URL+"/identicon/user", nil)
			if w.Code != http.StatusBadGateway || w.Header().Get("Location") != "" {
				t.Fatal("invalid image redirect was forwarded")
			}
		})
	}
}

func TestBudgetEvictsLeastRecentlyUsedAndStoresNoURL(t *testing.T) {
	server := imageServer()
	defer server.Close()
	c := New(server.URL, t.TempDir())
	first := server.URL + "/files/first?token=private-url-value"
	second := server.URL + "/files/second"
	third := server.URL + "/files/third"
	request(c, http.MethodGet, first, nil)
	firstURL, _ := url.Parse(first)
	info, err := os.Stat(c.filename(firstURL))
	if err != nil {
		t.Fatal(err)
	}
	c.budget = info.Size() * 2
	request(c, http.MethodGet, second, nil)
	secondURL, _ := url.Parse(second)
	old := time.Unix(1000, 0)
	if err := os.Chtimes(c.filename(secondURL), old, old); err != nil {
		t.Fatal(err)
	}
	request(c, http.MethodGet, first, nil)
	request(c, http.MethodGet, third, nil)
	server.Close()
	if request(c, http.MethodGet, second, nil).Code != http.StatusBadGateway {
		t.Fatal("least recently used image was not evicted")
	}
	if request(c, http.MethodGet, first, nil).Code != http.StatusOK || request(c, http.MethodGet, third, nil).Code != http.StatusOK {
		t.Fatal("recent images were evicted")
	}
	files, _ := os.ReadDir(c.directory)
	var total int64
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(c.directory, file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		total += int64(len(data))
		if strings.Contains(string(data), "private-url-value") || strings.Contains(file.Name(), "private-url-value") {
			t.Fatal("URL credentials appeared in cache metadata")
		}
	}
	if total > c.budget {
		t.Fatal("cache exceeded its disk budget")
	}
}
