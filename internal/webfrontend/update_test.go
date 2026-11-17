// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webfrontend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFirstDownloadAndCachedRestart(t *testing.T) {
	archive := fixture(t, nil)
	sum := sha256.Sum256(archive)
	revision := hex.EncodeToString(sum[:])
	var incompatible atomic.Bool
	var downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Camp-Desktop-Version") != "0.2.0" || r.Header.Get("X-Camp-Host-Api") != "2" {
			t.Error("missing compatibility headers")
		}
		if r.URL.Path == UpdatePath {
			if incompatible.Load() {
				w.WriteHeader(http.StatusUpgradeRequired)
				_, _ = w.Write([]byte(`{"error":{"code":"CLIENT_UPDATE_REQUIRED"},"minimumVersion":"0.3.0","hostApi":3,"serverVersion":"next"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(Release{revision, revision, int64(len(archive)), HostAPI, "/_desktop/frontend/" + revision + ".zip"})
			return
		}
		downloads.Add(1)
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	directory := t.TempDir()
	u, err := OpenUpdates(server.URL, directory, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := u.Current(); b != nil {
		t.Fatal("first launch already has a frontend")
	}
	got, err := u.Check(context.Background())
	if err != nil || got != revision {
		t.Fatalf("first download: %s %v", got, err)
	}
	if err := u.Activate(got); err != nil {
		t.Fatal(err)
	}
	if err := u.Confirm(got); err != nil {
		t.Fatal(err)
	}
	u, err = OpenUpdates(server.URL, directory, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := u.Current(); b == nil {
		t.Fatal("confirmed cache lost")
	}
	if got, err := u.Check(context.Background()); err != nil || got != "" {
		t.Fatalf("cache check: %s %v", got, err)
	}
	if downloads.Load() != 1 {
		t.Fatal("unchanged cache downloaded again")
	}
	incompatible.Store(true)
	_, err = u.Check(context.Background())
	var required *UpdateRequiredError
	if !errors.As(err, &required) || required.MinimumVersion != "0.3.0" || required.HostAPI != 3 {
		t.Fatalf("compatibility gate: %v", err)
	}
	if downloads.Load() != 1 {
		t.Fatal("incompatible frontend downloaded")
	}
	server.Close()
	if _, err := u.Check(context.Background()); err == nil {
		t.Fatal("offline check succeeded")
	}
	if b, _ := u.Current(); b == nil {
		t.Fatal("network failure discarded cached frontend")
	}
}

func TestUpdateSkipsIdenticalBundledFrontend(t *testing.T) {
	archive := fixture(t, nil)
	sum := sha256.Sum256(archive)
	revision := hex.EncodeToString(sum[:])
	var downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == UpdatePath {
			_ = json.NewEncoder(w).Encode(Release{revision, revision, int64(len(archive)), HostAPI, "/_desktop/frontend/" + revision + ".zip"})
			return
		}
		downloads.Add(1)
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	b, err := FromArchive(archive, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	u, err := newUpdates(server.URL, t.TempDir(), b)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if revision, err := u.Check(context.Background()); err != nil || revision != "" {
			t.Fatalf("identical bundle offered as update: %q, %v", revision, err)
		}
	}
	if downloads.Load() != 0 {
		t.Fatal("identical embedded archive downloaded")
	}
	if current, revision := u.Current(); current != b || revision != "" || u.state.Staged != "" {
		t.Fatal("unchanged bundle altered activation state")
	}
}

func TestUpdateLifecycle(t *testing.T) {
	archive := fixture(t, map[string]string{"vite/scripts/new.js": "new version"})
	sum := sha256.Sum256(archive)
	revision := hex.EncodeToString(sum[:])
	release := Release{revision, revision, int64(len(archive)), HostAPI, "/_desktop/frontend/" + revision + ".zip"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == UpdatePath {
			_ = json.NewEncoder(w).Encode(release)
		} else {
			_, _ = w.Write(archive)
		}
	}))
	defer server.Close()
	b, err := FromArchive(fixture(t, nil), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	u, err := newUpdates(server.URL, directory, b)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := u.Check(context.Background())
	if err != nil || staged != revision {
		t.Fatalf("stage: %s %v", staged, err)
	}
	if got, rev := u.Current(); got != b || rev != "" {
		t.Fatal("download replaced a running document")
	}
	if err := u.Activate(staged); err != nil {
		t.Fatal(err)
	}
	if got, _ := u.Current(); !got.Contains("/vite/scripts/new.js") || !strings.Contains(got.HTML(), revision) {
		t.Fatal("new bundle unavailable")
	}
	if !u.Pending(revision) {
		t.Fatal("missing boot acknowledgement")
	}
	// A crash before the renderer acknowledges restores the bundled version.
	restarted, err := newUpdates(server.URL, directory, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, rev := restarted.Current(); rev != "" {
		t.Fatal("failed boot survived restart")
	}
	if rev, err := restarted.Check(context.Background()); err != nil || rev != "" {
		t.Fatal("rejected boot retried")
	}
	// A healthy boot persists and works after the server goes away.
	if err := u.Confirm(revision); err != nil {
		t.Fatal(err)
	}
	server.Close()
	restarted, err = newUpdates(server.URL, directory, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, rev := restarted.Current(); rev != revision || restarted.Pending(rev) {
		t.Fatal("confirmed bundle lost")
	}
	if _, err := restarted.Check(context.Background()); err == nil {
		t.Fatal("offline request succeeded")
	}
	if _, rev := restarted.Current(); rev != revision {
		t.Fatal("network error replaced cache")
	}
	other, err := newUpdates("https://other.invalid", directory, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, rev := other.Current(); rev != "" {
		t.Fatal("cache crossed origins")
	}
}

func TestUpdateRejectsIncompleteAndIncompatibleDownloads(t *testing.T) {
	archive := fixture(t, nil)
	sum := sha256.Sum256(archive)
	revision := hex.EncodeToString(sum[:])
	for _, mode := range []string{"truncated", "checksum", "host", "path", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == UpdatePath {
					release := Release{revision, revision, int64(len(archive)), HostAPI, "/_desktop/frontend/" + revision + ".zip"}
					if mode == "host" {
						release.HostAPI = 1
					}
					if mode == "path" {
						release.Archive = "https://outside.invalid/archive.zip"
					}
					_ = json.NewEncoder(w).Encode(release)
					return
				}
				data := append([]byte{}, archive...)
				switch mode {
				case "redirect":
					http.Redirect(w, r, "https://outside.invalid/archive.zip", 302)
					return
				case "truncated":
					data = data[:len(data)/2]
				case "checksum":
					data[len(data)-1] ^= 1
				}
				_, _ = w.Write(data)
			}))
			defer server.Close()
			// Keep the installed bundle distinct so integrity checks still exercise
			// the download path, rather than the already-installed fast path.
			b, _ := FromArchive(fixture(t, map[string]string{"vite/scripts/old.js": "installed"}), server.URL)
			u, err := newUpdates(server.URL, t.TempDir(), b)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := u.Check(context.Background()); err == nil {
				t.Fatal("bad release accepted")
			}
			if _, rev := u.Current(); rev != "" {
				t.Fatal("failed download changed current")
			}
			if _, err := os.Stat(filepath.Join(u.directory, revision+".zip")); !os.IsNotExist(err) {
				t.Fatal("bad archive retained")
			}
		})
	}
}

func TestUpdateRollbackKeepsPrevious(t *testing.T) {
	b, _ := FromArchive(fixture(t, nil), "https://camp.invalid")
	u, err := newUpdates("https://camp.invalid", t.TempDir(), b)
	if err != nil {
		t.Fatal(err)
	}
	install := func(marker string) string {
		data := fixture(t, map[string]string{"vite/scripts/version.js": marker})
		hash := sha256.Sum256(data)
		revision := hex.EncodeToString(hash[:])
		if err := os.WriteFile(filepath.Join(u.directory, revision+".zip"), data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := u.Activate(revision); err != nil {
			t.Fatal(err)
		}
		return revision
	}
	first := install("first")
	if err := u.Confirm(first); err != nil {
		t.Fatal(err)
	}
	second := install("second")
	if err := u.Confirm(first); err != nil {
		t.Fatal(err)
	}
	if !u.Pending(second) {
		t.Fatal("old document acknowledged new version")
	}
	if err := u.Rollback(second); err != nil {
		t.Fatal(err)
	}
	if _, rev := u.Current(); rev != first {
		t.Fatal("rollback lost working version")
	}
	if err := u.Activate(second); err == nil {
		t.Fatal("failed version reactivated")
	}
}

func TestStagedUpdateAppliesOnNextLaunch(t *testing.T) {
	origin, directory := "https://camp.invalid", t.TempDir()
	b, _ := FromArchive(fixture(t, nil), origin)
	u, err := newUpdates(origin, directory, b)
	if err != nil {
		t.Fatal(err)
	}
	data := fixture(t, map[string]string{"vite/scripts/next.js": "next"})
	hash := sha256.Sum256(data)
	revision := hex.EncodeToString(hash[:])
	if err := os.WriteFile(filepath.Join(u.directory, revision+".zip"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := u.Stage(revision); err != nil {
		t.Fatal(err)
	}
	if _, current := u.Current(); current != "" {
		t.Fatal("background download changed running bundle")
	}
	next, err := newUpdates(origin, directory, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, current := next.Current(); current != revision || !next.Pending(revision) {
		t.Fatal("next launch did not activate staged bundle")
	}
	failed, err := newUpdates(origin, directory, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, current := failed.Current(); current != "" {
		t.Fatal("failed next-launch bundle did not roll back")
	}
}
