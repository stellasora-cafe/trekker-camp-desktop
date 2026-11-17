// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAboutOfflineDocumentAndResources(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "about"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "about", "organization.png"), []byte("local-avatar"), 0600); err != nil {
		t.Fatal(err)
	}
	handler := aboutHandler(root, "1.2.3<script>")
	for _, test := range []struct {
		path    string
		status  int
		content string
	}{
		{"/", 200, "1.2.3&lt;script&gt;"},
		{"/about.css", 200, "font-weight:400"},
		{"/about.js", 200, "window-control"},
		{"/images/organization.png", 200, "local-avatar"},
		{"/images/camp-icon.png", 404, ""},
		{"/images/../release.json", 404, ""},
		{"/api/i", 404, ""},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", aboutOrigin+test.path, nil))
			if response.Code != test.status {
				t.Fatalf("status %d", response.Code)
			}
			if !strings.Contains(response.Body.String(), test.content) {
				t.Fatalf("missing expected content %q", test.content)
			}
			if !strings.Contains(response.Header().Get("Content-Security-Policy"), "connect-src 'none'") {
				t.Fatal("network access not disabled")
			}
		})
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "https://example.com/", nil))
	if response.Code != 404 {
		t.Fatal("foreign origin accepted")
	}
}

func TestAboutCaptionUsesSharedNativeAssets(t *testing.T) {
	response := httptest.NewRecorder()
	aboutHandler(t.TempDir(), "1.0.0").ServeHTTP(response, httptest.NewRequest("GET", aboutOrigin+"/", nil))
	for _, name := range []string{"minimize", "maximize", "restore", "close"} {
		if !strings.Contains(response.Body.String(), string(captionAsset(name))) {
			t.Fatalf("caption %s does not use shared SVG", name)
		}
	}
	script, err := aboutFiles.ReadFile("about/about.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "camp-window-state-change") {
		t.Fatal("caption not observing maximize/restore state")
	}
}
