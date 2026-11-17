//go:build !js

package mygo

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHTTPResourcesKeepOriginAndCompleteResponse(t *testing.T) {
	const target = "https://camp.example/vite/app.js?build=one"
	payload := bytes.Repeat([]byte("module\x00\xff;"), 32000)
	var handled atomic.Int32
	var owner *Window
	options := PageOptions{
		Untrusted:      true,
		ResourceFilter: func(raw string) bool { return raw == target },
		ResourceHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handled.Add(1)
			if r.URL.String() != target || r.RequestURI != "/vite/app.js?build=one" || r.Method != http.MethodGet {
				t.Errorf("original request changed: %s %s %s", r.Method, r.URL, r.RequestURI)
			}
			if CallerWindow(r.Context()) != owner || CallerPage(r.Context()) != owner.Page() {
				t.Error("HTTP handler lost caller window/page context")
			}
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload[:1024])
			w.(http.Flusher).Flush()
			_, _ = w.Write(payload[1024:])
		}),
	}
	w, fw := testWindow(t, WindowOptions{URL: "https://camp.example/", Page: options})
	owner = w
	if fw.Opts.ResourceFilter == nil || !fw.Opts.ResourceFilter(target) || fw.Opts.ResourceFilter("https://other.example/vite/app.js") {
		t.Fatal("backend did not receive the window's selective filter")
	}
	rec := request(t, fw, http.MethodGet, target, nil)
	if rec.status != http.StatusOK || rec.body.String() != string(payload) || rec.header.Get("Cache-Control") != "no-cache" || rec.header.Get("Content-Type") != "text/javascript; charset=utf-8" {
		t.Fatalf("HTTP response changed: status=%d length=%d headers=%v", rec.status, rec.body.Len(), rec.header)
	}
	if handled.Load() != 1 {
		t.Fatalf("handler ran %d times", handled.Load())
	}
	if fw.URL() != "https://camp.example/" {
		t.Fatal("resource response changed the document origin")
	}
}

func TestHTTPResourcesArePerWindowAndFilter(t *testing.T) {
	const firstURL = "https://camp.example/vite/first.js"
	const secondURL = "https://camp.example/vite/second.js"
	var firstCalls, secondCalls atomic.Int32
	_, first := testWindow(t, WindowOptions{Page: PageOptions{
		ResourceFilter: func(raw string) bool { return raw == firstURL },
		ResourceHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			firstCalls.Add(1)
			_, _ = fmt.Fprint(w, "first")
		}),
	}})
	_, second := testWindow(t, WindowOptions{Page: PageOptions{
		ResourceFilter: func(raw string) bool { return raw == secondURL },
		ResourceHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			secondCalls.Add(1)
			_, _ = fmt.Fprint(w, "second")
		}),
	}})
	if rec := request(t, first, http.MethodGet, firstURL, nil); rec.body.String() != "first" {
		t.Fatal("first window used another handler")
	}
	if rec := request(t, second, http.MethodGet, secondURL, nil); rec.body.String() != "second" {
		t.Fatal("second window used another handler")
	}
	if first.Opts.ResourceFilter(secondURL) || second.Opts.ResourceFilter(firstURL) {
		t.Fatal("window filters leaked")
	}
	// A native backend leaves these requests on the network. Even an unexpected
	// forwarded request cannot invoke another window's HTTP handler.
	if rec := request(t, first, http.MethodGet, secondURL, nil); rec.status != http.StatusNotFound {
		t.Fatal("unselected request reached a handler")
	}
	if rec := request(t, second, http.MethodGet, firstURL, nil); rec.status != http.StatusNotFound {
		t.Fatal("unselected request reached a handler")
	}
	if firstCalls.Load() != 1 || secondCalls.Load() != 1 {
		t.Fatal("HTTP handlers crossed window boundaries")
	}
}

func TestHTTPResourcesRequireBothOptions(t *testing.T) {
	var filterCalls, handlerCalls atomic.Int32
	filter := func(string) bool { filterCalls.Add(1); return true }
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handlerCalls.Add(1); _, _ = io.WriteString(w, "owned") })
	for _, options := range []PageOptions{{}, {ResourceFilter: filter}, {ResourceHandler: handler}} {
		w, fw := testWindow(t, WindowOptions{Page: options})
		if fw.Opts.ResourceFilter != nil || w.resourceFilter != nil {
			t.Fatal("incomplete options enabled request interception")
		}
		if rec := request(t, fw, http.MethodGet, "https://camp.example/vite/app.js", nil); rec.status != http.StatusNotFound {
			t.Fatal("default page intercepted HTTP")
		}
	}
	if filterCalls.Load() != 0 || handlerCalls.Load() != 0 {
		t.Fatal("default/incomplete options invoked HTTP callbacks")
	}
}

func TestHTTPResourcesDoNotReplaceCustomProtocols(t *testing.T) {
	const scheme = "httpresourcecheck"
	if err := Protocol.HandleFunc(scheme, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "protocol") }); err != nil {
		t.Fatal(err)
	}
	defer Protocol.Unhandle(scheme)
	_, fw := testWindow(t, WindowOptions{Page: PageOptions{
		ResourceFilter:  func(string) bool { return true },
		ResourceHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "http") }),
	}})
	if rec := request(t, fw, http.MethodGet, scheme+"://localhost/value", nil); strings.TrimSpace(rec.body.String()) != "protocol" {
		t.Fatal("HTTP handler replaced a custom protocol")
	}
}
