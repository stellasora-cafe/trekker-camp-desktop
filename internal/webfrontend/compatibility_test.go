// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webfrontend

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCheckCompatibilityOnlyRequestsManifest(t *testing.T) {
	for _, mode := range []string{"compatible", "upgrade", "host-api", "maintenance", "maintenance-json", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != UpdatePath {
					t.Errorf("unexpected resource request %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if r.Header.Get("X-Camp-Desktop-Version") != "0.2.0" || r.Header.Get("X-Camp-Host-Api") != "2" {
					t.Error("missing compatibility headers")
				}
				switch mode {
				case "upgrade":
					w.WriteHeader(http.StatusUpgradeRequired)
					_, _ = w.Write([]byte(`{"minimumVersion":"0.3.0","hostApi":3,"serverVersion":"next"}`))
					return
				case "maintenance":
					http.Redirect(w, r, "/__camp-maintenance/v1.html#/", http.StatusFound)
					return
				case "maintenance-json":
					w.Header().Set("X-Camp-Page", "maintenance")
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte(`{"error":{"code":"CAMP_MAINTENANCE"}}`))
					return
				case "malformed":
					_, _ = w.Write([]byte(`not a manifest`))
					return
				}
				revision := strings.Repeat("a", 64)
				release := Release{revision, revision, 123, HostAPI, "/_desktop/frontend/" + revision + ".zip"}
				if mode == "host-api" {
					release.HostAPI = HostAPI + 1
				}
				_ = json.NewEncoder(w).Encode(release)
			}))
			defer server.Close()
			u, err := OpenUpdates(server.URL, t.TempDir(), "0.2.0")
			if err != nil {
				t.Fatal(err)
			}
			err = u.CheckCompatibility(context.Background())
			switch mode {
			case "compatible":
				if err != nil {
					t.Fatal(err)
				}
			case "upgrade", "host-api":
				var required *UpdateRequiredError
				if !errors.As(err, &required) {
					t.Fatalf("missing upgrade gate: %v", err)
				}
				if mode == "upgrade" && (required.MinimumVersion != "0.3.0" || required.ServerVersion != "next") {
					t.Fatalf("lost version requirements: %+v", required)
				}
			case "maintenance", "maintenance-json":
				if !errors.Is(err, ErrMaintenance) {
					t.Fatalf("missing maintenance gate: %v", err)
				}
			default:
				if err == nil {
					t.Fatal("invalid manifest accepted")
				}
			}
			if requests.Load() != 1 {
				t.Fatalf("expected single manifest request, got %d", requests.Load())
			}
			files, err := os.ReadDir(u.directory)
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range files {
				if file.Name() != "state.json" {
					t.Fatalf("compatibility downloaded resource %s", file.Name())
				}
			}
			if current, revision := u.Current(); current != nil || revision != "" {
				t.Fatal("compatibility check changed active frontend")
			}
		})
	}
}
