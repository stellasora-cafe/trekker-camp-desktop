// SPDX-FileCopyrightText: stellasora-cafe contributors
// SPDX-License-Identifier: Apache-2.0

package webassets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestInstalledFontsWorkBeforeFirstConnectionAndSkipDownloads(t *testing.T) {
	installed, cache := t.TempDir(), t.TempDir()
	fontDir := filepath.Join(installed, "fonts", "misans-v1")
	if err := os.MkdirAll(fontDir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, face := range faces {
		if err := os.WriteFile(filepath.Join(fontDir, "MiSans-"+face.name+".woff2"), []byte("wOF2-local-font"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var fontRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/fonts/") {
			fontRequests.Add(1)
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	first, err := LoadCached(server.URL, cache, installed)
	if err != nil {
		t.Fatal(err)
	}
	for _, face := range faces {
		remote := server.URL + "/client-assets/camp/fonts/misans-v1/MiSans-" + face.name + ".woff2"
		local := first.CachedURL(remote)
		if local == "" {
			t.Fatalf("font unavailable before first connection: %s", face.name)
		}
		w := httptest.NewRecorder()
		first.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, local, nil))
		if w.Code != 200 || w.Body.String() != "wOF2-local-font" {
			t.Fatalf("installed font response: %d", w.Code)
		}
	}
	updated, err := Prepare(context.Background(), server.URL, cache, installed)
	if err != nil {
		t.Fatal(err)
	}
	if fontRequests.Load() != 0 {
		t.Fatal("packaged fonts requested from server")
	}
	if _, err := os.Stat(filepath.Join(cache, "private", "fonts")); !os.IsNotExist(err) {
		t.Fatal("packaged fonts duplicated in cache")
	}
	if !strings.Contains(updated.Style(), "MiSans-Bold.woff2") {
		t.Fatal("installed weight mapping lost after refresh")
	}
	server.Close()
	restarted, err := LoadCached(server.URL, cache, installed)
	if err != nil || restarted.Report().PrivateBytes != first.Report().PrivateBytes {
		t.Fatalf("installed fonts lost on offline restart: %v", err)
	}
}
