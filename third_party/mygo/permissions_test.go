package mygo

import (
	"testing"
	"time"
)

func TestPermissionRequestHandler(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{})
	var respond func(bool)
	w.Page().SetPermissionRequestHandler(func(req PermissionRequest, answer func(bool)) {
		if !isMainThread() || req.Origin != "https://example.com" || len(req.Permissions) != 1 || req.Permissions[0] != PermissionNotifications {
			t.Errorf("request on main=%v: %+v", isMainThread(), req)
		}
		respond = answer
	})
	type result struct{ allowed, main bool }
	results := make(chan result, 2)
	onMain(func() {
		fw.H.PermissionRequested([]string{"notifications"}, "https://example.com", func(allowed bool) {
			results <- result{allowed, isMainThread()}
		})
	})
	if respond == nil || len(results) != 0 {
		t.Fatal("the request did not wait for its response")
	}
	done := make(chan struct{})
	go func() {
		respond(true)
		respond(false)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("response did not return from the main thread")
	}
	if len(results) != 1 {
		t.Fatalf("got %d responses, want exactly one", len(results))
	}
	if got := <-results; !got.allowed || !got.main {
		t.Fatalf("response = %+v, want allow on main thread", got)
	}

	// Either setter replaces the other, and nil restores trusted-origin defaults.
	w.Page().SetPermissionHandler(func(PermissionRequest) bool { return false })
	ask := func() (allowed bool) {
		onMain(func() {
			fw.H.PermissionRequested([]string{"camera"}, "mygo://localhost", func(value bool) { allowed = value })
		})
		return
	}
	if ask() {
		t.Fatal("synchronous handler did not replace the asynchronous handler")
	}
	w.Page().SetPermissionRequestHandler(nil)
	if !ask() {
		t.Fatal("nil asynchronous handler did not restore the default")
	}
}

func TestPermissionRequestResponseAfterClose(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{})
	var respond func(bool)
	w.Page().SetPermissionRequestHandler(func(_ PermissionRequest, answer func(bool)) { respond = answer })
	called, allowed := false, true
	onMain(func() {
		fw.H.PermissionRequested([]string{"camera"}, "https://example.com", func(value bool) { called, allowed = true, value })
	})
	w.Destroy()
	respond(true)
	if !called || allowed {
		t.Fatal("a response after closing the page granted permission")
	}
}
