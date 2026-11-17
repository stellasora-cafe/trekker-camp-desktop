// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"encoding/json"
	"net/url"
	"runtime"
	"sync"

	"github.com/egoist/mygo"
)

const windowCaptionHeight = 50

func applyWindowChrome(options *mygo.WindowOptions, platform string) {
	if platform != "windows" {
		return
	}
	options.TitleBarStyle = mygo.TitleBarHidden
	options.TitleBarHeight = windowCaptionHeight
	options.HideWindowButtons = true
}

func windowChromePreload(origin, platform string) string {
	if platform != "windows" {
		return ""
	}
	encoded, _ := json.Marshal(origin)
	return `;(()=>{if(window!==top||location.origin!==` + string(encoded) + `)return;
const mark=()=>{const root=document.documentElement;if(!root)return false;
root.dataset.campWindowChrome='true';return true;};
if(!mark()){const observer=new MutationObserver(()=>{if(mark())observer.disconnect()});observer.observe(document,{childList:true})}})();`
}

type windowChromeState struct {
	Maximized  bool `json:"maximized"`
	Fullscreen bool `json:"fullscreen"`
	Active     bool `json:"active"`
}

func windowChromeStateScript(origin string, state windowChromeState) string {
	encodedOrigin, _ := json.Marshal(origin)
	encodedState, _ := json.Marshal(state)
	return `(()=>{if(window!==top||location.origin!==` + string(encodedOrigin) + `||!document.documentElement)return;
const state=` + string(encodedState) + `,root=document.documentElement;
root.dataset.campWindowMaximized=String(state.maximized);
root.dataset.campWindowFullscreen=String(state.fullscreen);
root.dataset.campWindowActive=String(state.active);
window.dispatchEvent(new Event('camp-window-state-change'))})()`
}

func (c *client) configureWindowChrome(window *mygo.Window) {
	if runtime.GOOS != "windows" {
		return
	}
	var publishing sync.Mutex
	publish := func() {
		// Page.Eval waits for WebView2, so it must run outside window callbacks.
		go func() {
			publishing.Lock()
			defer publishing.Unlock()
			if window.IsDestroyed() || c.ctx.Err() != nil || !sameOrigin(c.origin, window.Page().URL()) {
				return
			}
			state := windowChromeState{window.IsMaximized(), window.IsFullScreen(), window.IsFocused()}
			_, _ = window.Page().EvalContext(c.ctx, windowChromeStateScript(c.origin, state))
		}()
	}
	window.Page().OnDOMReady(publish)
	window.OnMaximize(publish)
	window.OnUnmaximize(publish)
	window.OnFocus(publish)
	window.OnBlur(publish)
	window.OnShow(publish)
	window.OnRestore(publish)
	window.OnEnterFullScreen(publish)
	window.OnLeaveFullScreen(publish)
}

func windowControlAction(origin, pageURL, raw string) (action string, handled bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "camp-desktop" || u.Host != "window-control" {
		return "", false
	}
	if !sameOrigin(origin, pageURL) || u.User != nil || u.Path != "" || u.Fragment != "" {
		return "", true
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(query) != 1 || len(query["action"]) != 1 {
		return "", true
	}
	switch value := query.Get("action"); value {
	case "minimize", "maximize", "close", "drag":
		return value, true
	default:
		return "", true
	}
}

// MyGo only dispatches OnWillNavigate for top-level navigations. Keep this
// narrow native surface separate from the untrusted page's disabled Go bridge.
func (c *client) handleWindowControl(window *mygo.Window, raw string) bool {
	action, handled := windowControlAction(c.origin, window.Page().URL(), raw)
	if !handled {
		return false
	}
	if runtime.GOOS != "windows" || action == "" {
		return true
	}
	c.windowMu.Lock()
	_, tool := c.tools[window]
	owned := c.window == window || tool
	c.windowMu.Unlock()
	if !owned {
		return true
	}
	go func() {
		if window.IsDestroyed() || c.ctx.Err() != nil {
			return
		}
		switch action {
		case "minimize":
			if window.IsMinimizable() {
				window.Minimize()
			}
		case "maximize":
			if window.IsMaximizable() && !window.IsFullScreen() {
				window.ToggleMaximize()
			}
		case "close":
			window.Close()
		case "drag":
			if window.IsMovable() && !window.IsFullScreen() {
				startWindowDrag(window)
			}
		}
	}()
	return true
}
