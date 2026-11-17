// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"context"
	"errors"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestPermissionViewKeyboardAndChoices(t *testing.T) {
	model := permissionContent(mygo.PermissionRequest{Origin: "https://camp.stellasora.cafe", Permissions: []mygo.Permission{mygo.PermissionNotifications}})
	for _, tc := range []struct {
		name  string
		key   ui.Key
		click string
		allow bool
	}{
		{"enter-default-denies", ui.KeyEnter, "", false},
		{"escape-denies", ui.KeyEscape, "", false},
		{"decline", 0, "暂不允许", false},
		{"allow", 0, "允许通知", true},
		{"close", 0, "关闭权限请求", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, allowed := 0, false
			decision := &permissionDecision{respond: func(value bool) { calls++; allowed = value }}
			tester := ui.NewTester(func(c *ui.Context) { permissionView(c, model, decision.resolve) }, 440, 300)
			if !tester.Focused("暂不允许") || !tester.HasText(model.origin) || !tester.HasText("开启桌面通知？") {
				t.Fatal("prompt must show its site and focus the decline action")
			}
			if tc.click != "" {
				if err := tester.Click(tc.click); err != nil {
					t.Fatal(err)
				}
			} else {
				tester.Key(0, tc.key)
			}
			decision.resolve(false) // Native window closing follows the button.
			if calls != 1 || allowed != tc.allow {
				t.Fatalf("decision = %v (%d callbacks), want %v once", allowed, calls, tc.allow)
			}
		})
	}
}

func TestPermissionViewThemesAndTab(t *testing.T) {
	if os.Getenv("CAMP_PERMISSION_SNAPSHOTS") != "" {
		for _, path := range permissionFontCandidates("") {
			if data, err := os.ReadFile(path); err == nil && ui.RegisterFont(data, "MiSans") == nil {
				break
			}
		}
	}
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) {
			allowed := false
			model := permissionContent(mygo.PermissionRequest{Origin: "https://camp.stellasora.cafe", Permissions: []mygo.Permission{mygo.PermissionNotifications}})
			tester := ui.NewTester(func(c *ui.Context) {
				if dark {
					c.SetTheme(ui.DarkTheme())
				}
				permissionView(c, model, func(value bool) { allowed = value })
			}, 440, 300)
			for _, text := range []string{model.title, model.description, "暂不允许", "允许通知"} {
				bounds, found := tester.Find(text)
				if !found || bounds.X < 0 || bounds.Y < 0 || bounds.X+bounds.W > 440 || bounds.Y+bounds.H > 300 {
					t.Fatalf("prompt content is clipped: %s", text)
				}
			}
			if directory := os.Getenv("CAMP_PERMISSION_SNAPSHOTS"); directory != "" {
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				file, err := os.Create(filepath.Join(directory, "permission-"+name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(file, tester.Image()); err != nil {
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
			}
			tester.Key(0, ui.KeyTab)
			if !tester.Focused("允许通知") {
				t.Fatal("Tab must move from decline to allow")
			}
			tester.Key(0, ui.KeyEnter)
			if !allowed {
				t.Fatal("keyboard activation must grant the pending permission")
			}
		})
	}
}

func TestPermissionFontLoadsOnlyOneAnonymousFaceAndCaches(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/client-assets/camp/fonts/misans-v1/MiSans-Regular.ttf" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("native permission font must use its anonymous same-origin Regular face")
		}
		_, _ = w.Write([]byte("font-fixture"))
	}))
	defer server.Close()
	register := func(data []byte) error {
		if string(data) != "font-fixture" {
			return errors.New("invalid font")
		}
		return nil
	}
	cache := filepath.Join(t.TempDir(), "fonts", "MiSans-Regular.ttf")
	for range 2 {
		if err := preparePermissionFont(context.Background(), server.URL, cache, []string{cache}, register); err != nil {
			t.Fatal(err)
		}
	}
	if requests.Load() != 1 {
		t.Fatal("subsequent native prompts must reuse the cached face")
	}
	data, err := os.ReadFile(cache)
	if err != nil || string(data) != "font-fixture" {
		t.Fatal("font cache must contain the validated original bytes")
	}
}

func TestPermissionFontRejectsRedirectAndInvalidData(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		var destinationHits atomic.Int32
		destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationHits.Add(1) }))
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if redirect {
				http.Redirect(w, r, destination.URL, http.StatusFound)
				return
			}
			_, _ = w.Write([]byte("invalid"))
		}))
		cache := filepath.Join(t.TempDir(), "font.ttf")
		err := preparePermissionFont(context.Background(), server.URL, cache, nil, func([]byte) error { return errors.New("invalid font") })
		server.Close()
		destination.Close()
		if err == nil || destinationHits.Load() != 0 {
			t.Fatal("redirects and invalid font bytes must be rejected")
		}
		if _, err := os.Stat(cache); !os.IsNotExist(err) {
			t.Fatal("invalid font must not enter the cache")
		}
	}
}
