// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webassets

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestLoadCachedNeverConnectsOrRewritesIndex(t *testing.T) {
	var requests atomic.Int32
	server := fixtureServer(t, func(http.ResponseWriter, *http.Request) bool { requests.Add(1); return false })
	defer server.Close()
	directory := t.TempDir()
	prepared, err := Prepare(context.Background(), server.URL, directory)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(directory, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	requests.Store(0)
	bundle, err := LoadCached(server.URL, directory)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatal("offline load made a network request")
	}
	after, err := os.ReadFile(filepath.Join(directory, "index.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("offline load rewrote the index")
	}
	if bundle.Report().Bytes != prepared.Report().Bytes || len(bundle.Report().Resources) != 12 {
		t.Fatalf("incomplete offline bundle: %+v", bundle.Report())
	}
	for _, name := range []string{"apology-v1.webp", "apology-still-v1.webp"} {
		local := bundle.CachedURL(server.URL + "/client-assets/camp/maintenance/" + name)
		if local != AssetBaseURL+"private/maintenance/"+name {
			t.Fatalf("maintenance rewrite: %s", local)
		}
		w := httptest.NewRecorder()
		bundle.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, local, nil))
		if w.Code != 200 || w.Header().Get("Content-Type") != "image/webp" || !strings.HasPrefix(w.Body.String(), "RIFF") {
			t.Errorf("offline image %s: %d", name, w.Code)
		}
	}
}

func TestLoadCachedSeparatesOriginsAndLeavesNewDirectoryAbsent(t *testing.T) {
	server := fixtureServer(t, nil)
	defer server.Close()
	directory := t.TempDir()
	if _, err := Prepare(context.Background(), server.URL, directory); err != nil {
		t.Fatal(err)
	}
	bundle, err := LoadCached("https://another.invalid", directory)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Report().Bytes != 0 || bundle.CachedURL("/client-assets/camp/maintenance/apology-v1.webp") != "" {
		t.Fatal("a different origin received cached resources")
	}
	missing := filepath.Join(directory, "not-created")
	bundle, err = LoadCached(server.URL, missing)
	if err != nil || bundle.Report().Bytes != 0 {
		t.Fatalf("new offline profile: %v", err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("offline load created a directory")
	}
}
