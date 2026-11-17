// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCloseBehaviorBridgePersistsLocalChoice(t *testing.T) {
	cipher, err := newCredentialCipher("https://camp.example", make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "close-behavior")
	c := &client{origin: cipher.origin, credentials: cipher, closeBehavior: &closeBehaviorStore{filename: filename}}
	target := c.origin + closeBehaviorPath
	check := func(method, body, origin, challenge string, status int, value string) {
		t.Helper()
		request := httptest.NewRequest(method, target, strings.NewReader(body))
		request.Header.Set("Origin", origin)
		request.Header.Set("X-Camp-Close-Behavior", challenge)
		response := httptest.NewRecorder()
		c.serveCloseBehavior(response, request)
		if response.Code != status || (value != "" && strings.TrimSpace(response.Body.String()) != value) {
			t.Fatalf("%s %s: %d %q", method, body, response.Code, response.Body.String())
		}
	}
	check(http.MethodGet, "", c.origin, cipher.challenge, http.StatusOK, `"ask"`)
	check(http.MethodPut, `"tray"`, "https://other.example", cipher.challenge, http.StatusForbidden, "")
	check(http.MethodPut, `"tray"`, c.origin, "", http.StatusForbidden, "")
	check(http.MethodPut, `"tray"`, c.origin, cipher.challenge, http.StatusOK, `"tray"`)
	check(http.MethodPut, `"invalid"`, c.origin, cipher.challenge, http.StatusBadRequest, "")
	c.closeBehavior = &closeBehaviorStore{filename: filename}
	check(http.MethodGet, "", c.origin, cipher.challenge, http.StatusOK, `"tray"`)
	check(http.MethodPut, `"quit"`, c.origin, cipher.challenge, http.StatusOK, `"quit"`)
	check(http.MethodPut, `"ask"`, c.origin, cipher.challenge, http.StatusOK, `"ask"`)

	main := c.pageOptions(false, false, func() bool { return true })
	tool := c.pageOptions(true, false, func() bool { return true })
	if main.ResourceFilter(target) != (runtime.GOOS == "windows") || tool.ResourceFilter(target) ||
		strings.Contains(tool.PreloadScript, "__campCloseBehavior") {
		t.Fatal("close-behavior bridge must be scoped to the supported main window")
	}
}
