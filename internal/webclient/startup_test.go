// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
)

func TestFetchHTMLAnonymousAndBounded(t *testing.T) {
	var redirectHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectHits.Add(1)
	}))
	defer target.Close()
	for _, kind := range []string{"document", "redirect", "failure", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("startup request must be an anonymous GET of the original root")
				}
				switch kind {
				case "redirect":
					http.Redirect(w, r, target.URL, http.StatusFound)
				case "failure":
					w.WriteHeader(http.StatusServiceUnavailable)
				case "oversize":
					_, _ = w.Write([]byte(strings.Repeat("x", maxHTMLBytes+1)))
				default:
					_, _ = fmt.Fprint(w, "<!doctype html><html><head></head><body>营地</body></html>")
				}
			}))
			defer server.Close()
			document, err := fetchHTML(context.Background(), server.URL)
			if kind == "document" {
				if err != nil || !strings.Contains(document, "营地") {
					t.Fatalf("document not loaded: %v", err)
				}
			} else if err == nil || document != "" {
				t.Fatalf("invalid startup response accepted: %s", kind)
			}
		})
	}
	if redirectHits.Load() != 0 {
		t.Fatal("startup followed a redirect")
	}
}

func TestFetchHTMLCancellation(t *testing.T) {
	requested := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requested)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := fetchHTML(ctx, server.URL); done <- err }()
	<-requested
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("closing must cancel the pending request: %v", err)
	}
}

func TestNavigationAndPermissions(t *testing.T) {
	const origin = "https://camp.example"
	for _, tc := range []struct {
		url    string
		user   bool
		action navigationAction
	}{
		{origin + "/settings/profile", false, navigateHere},
		{"https://CAMP.example:443/notes/one", true, navigateHere},
		{"camp-desktop://retry", true, retryConnection},
		{"camp-desktop://retry?token=secret", true, blockNavigation},
		{"https://elsewhere.example/path", true, openExternal},
		{"https://elsewhere.example/path", false, blockNavigation},
		{"https://camp.example.evil.test/", false, blockNavigation},
		{"http://camp.example/", false, blockNavigation},
		{"https://user:password@camp.example/", true, blockNavigation},
		{"mailto:camp@example.com", true, openExternal},
		{"mailto:camp@example.com", false, blockNavigation},
		{"file:///C:/private", true, blockNavigation},
		{"javascript:alert(1)", true, blockNavigation},
		{"data:text/html,hello", true, blockNavigation},
		{"about:blank", true, blockNavigation},
	} {
		if got := navigation(origin, tc.url, tc.user); got != tc.action {
			t.Errorf("navigation(%q, %v) = %v, want %v", tc.url, tc.user, got, tc.action)
		}
	}
	known := mygo.PermissionRequest{Origin: origin, Permissions: []mygo.Permission{mygo.PermissionCamera, mygo.PermissionMicrophone, mygo.PermissionGeolocation, mygo.PermissionNotifications}}
	if label, ok := permissionLabel(origin, known); !ok || !strings.Contains(label, "发送通知") {
		t.Fatal("supported same-origin permissions must be presented together")
	}
	for _, request := range []mygo.PermissionRequest{
		{Origin: "https://other.example", Permissions: []mygo.Permission{mygo.PermissionCamera}},
		{Origin: origin, Permissions: []mygo.Permission{"clipboard-read"}},
		{Origin: origin},
	} {
		if _, ok := permissionLabel(origin, request); ok {
			t.Fatal("external or unknown permission accepted")
		}
	}
}

func TestStartupRetryAndClose(t *testing.T) {
	var gate loadGate
	var admitted atomic.Int32
	var work sync.WaitGroup
	for range 20 {
		work.Go(func() {
			if gate.begin() {
				admitted.Add(1)
			}
		})
	}
	work.Wait()
	if admitted.Load() != 1 {
		t.Fatal("parallel reconnects must share one attempt")
	}
	gate.finish()
	if !gate.begin() {
		t.Fatal("failed attempt must allow retry")
	}
	gate.close()
	gate.finish()
	if gate.begin() {
		t.Fatal("closed window must stay closed after a racing attempt finishes")
	}
	failed := connectionDocument(errors.New("<script>unsafe</script>"))
	if !strings.Contains(failed, `href="camp-desktop://retry"`) || strings.Contains(failed, "<script>") {
		t.Fatal("error document must offer reconnect and escape server text")
	}
	if strings.Contains(connectionDocument(nil), "camp-desktop://retry") {
		t.Fatal("loading must not offer concurrent reconnect")
	}
}
