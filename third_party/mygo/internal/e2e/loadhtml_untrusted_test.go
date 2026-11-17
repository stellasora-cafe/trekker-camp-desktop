//go:build !js

package e2e

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo"
)

func TestLoadHTMLUntrustedReplacement(t *testing.T) {
	for _, persistent := range []bool{false, true} {
		p := mygo.PageOptions{Untrusted: true, PreloadScript: "window.loadHTMLPreload = true;"}
		if persistent {
			p.Session = mygo.NewSession("loadhtml-untrusted")
		}
		w := newWindow(t, mygo.WindowOptions{Hidden: true, Page: p})
		w.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
			t.Error("LoadHTML reached the page navigation handler")
			e.PreventDefault()
		})
		// A connection can fail before the first document has started. Repeat
		// once the view is ready, as when the user retries an unavailable site.
		for i := range 2 {
			w.Page().LoadHTML("<!doctype html><title>连接中</title>\n<p>稍候…</p>", "about:blank")
			html := fmt.Sprintf("<!doctype html><title>连接失败</title>\n<p id=retry data-attempt='%d'>重新连接 🌍</p>", i)
			w.Page().LoadHTML(html, "about:blank")
			waitFor(t, w, fmt.Sprintf("document.getElementById('retry')?.dataset.attempt === '%d' && window.loadHTMLPreload", i))
		}
	}
}
