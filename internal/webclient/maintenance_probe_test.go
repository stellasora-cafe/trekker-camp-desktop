// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"

	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webfrontend"
)

func TestMaintenanceRecoveryRequiresRedirectedTargets(t *testing.T) {
	var loginMaintenance, signupMaintenance, manifestMaintenance, homeMaintenance atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.URL.Path == "/" && homeMaintenance.Load()) ||
			(r.URL.Path == "/desktop-auth" && r.URL.Query().Get("mode") == "signin" && loginMaintenance.Load()) ||
			(r.URL.Path == "/desktop-auth" && r.URL.Query().Get("mode") == "signup" && signupMaintenance.Load()) ||
			(r.URL.Path == webfrontend.UpdatePath && manifestMaintenance.Load()) {
			http.Redirect(w, r, "/__camp-maintenance/v1.html#/", 302)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	c := &client{origin: server.URL}
	c.maintenanceRetries.Store("login", server.URL+"/desktop-auth?mode=signin")
	c.maintenanceRetries.Store("signup", server.URL+"/desktop-auth?mode=signup")
	c.maintenanceProbeTargets.Store(server.URL+webfrontend.UpdatePath, struct{}{})
	assertActive := func(want bool) {
		t.Helper()
		active, err := c.probeMaintenance(context.Background())
		if err != nil || active != want {
			t.Fatalf("active=%v err=%v want=%v", active, err, want)
		}
	}
	homeMaintenance.Store(true)
	assertActive(true)
	homeMaintenance.Store(false)
	loginMaintenance.Store(true)
	assertActive(true)
	loginMaintenance.Store(false)
	signupMaintenance.Store(true)
	assertActive(true)
	signupMaintenance.Store(false)
	manifestMaintenance.Store(true)
	assertActive(true)
	manifestMaintenance.Store(false)
	assertActive(false)
	server.Close()
	if active, err := c.probeMaintenance(context.Background()); active || err == nil {
		t.Fatalf("offline must not signal recovery: %v %v", active, err)
	}
}

func TestMaintenanceProbeRecognizesServiceResponseAndRecovery(t *testing.T) {
	var maintenance atomic.Bool
	maintenance.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maintenance.Load() {
			w.Header().Set("X-Camp-Page", "maintenance")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":"CAMP_MAINTENANCE"}}`))
			return
		}
		w.Header().Set("X-Camp-Page", "app")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	c := &client{origin: server.URL}
	for _, want := range []bool{true, false} {
		maintenance.Store(want)
		active, err := c.probeMaintenance(context.Background())
		if active != want || err != nil {
			t.Fatalf("maintenance=%v: active=%v err=%v", want, active, err)
		}
	}
}

func TestMaintenanceTargetsFollowCurrentWindowWithoutStartupCache(t *testing.T) {
	main, tool := &mygo.Window{}, &mygo.Window{}
	c := &client{window: main, tools: map[*mygo.Window]struct{}{tool: {}}}
	// The bootstrap window is hidden and must never receive a maintenance view.
	guest, windows := c.maintenanceTargets()
	if guest || len(windows) != 1 || windows[0] != tool {
		t.Fatal("bootstrap was treated as a visible frontend")
	}
	// First login can display the online frontend before any bundle is cached.
	c.mainFrontend = &frontendWindow{}
	c.mainFrontend.online.Store(true)
	guest, windows = c.maintenanceTargets()
	if guest || len(windows) != 2 || windows[0] != main || windows[1] != tool {
		t.Fatal("logged-in online frontend did not receive in-window maintenance")
	}
	c.mainFrontend.online.Store(false)
	guest, windows = c.maintenanceTargets()
	if guest || len(windows) != 2 || windows[0] != main {
		t.Fatal("cached frontend did not receive in-window maintenance")
	}
	c.guest, c.mainFrontend = true, nil
	guest, windows = c.maintenanceTargets()
	if !guest || len(windows) != 1 || windows[0] != tool {
		t.Fatal("native welcome was treated as a web frontend")
	}
}

func TestMainMaintenanceDocumentBeforeFrontendBoot(t *testing.T) {
	main, tool := &mygo.Window{}, &mygo.Window{}
	c := &client{window: main, mainFrontend: &frontendWindow{}}
	c.mainFrontend.online.Store(true)
	if !c.mainNeedsMaintenanceDocument(main) {
		t.Fatal("uncached main must show the maintenance document")
	}
	if c.mainNeedsMaintenanceDocument(tool) {
		t.Fatal("tool must not replace the main document")
	}
	c.mainFrontend.ready.Store(true)
	if c.mainNeedsMaintenanceDocument(main) {
		t.Fatal("mounted frontend must preserve its editors and show inline maintenance")
	}
	c.mainFrontend = nil
	if c.mainNeedsMaintenanceDocument(main) {
		t.Fatal("bootstrap must finish session recovery first")
	}
}
