// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"encoding/json"
	"errors"
	"net/url"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

func nativeContextMenuScript(origin, pageURL, frameURL string) string {
	if frameURL != "" || !sameOrigin(origin, pageURL) {
		return ""
	}
	encodedOrigin, _ := json.Marshal(origin)
	return `(()=>{if(window!==top||location.origin!==` + string(encodedOrigin) + `)return;
window.dispatchEvent(new Event('camp-native-context-menu'))})()`
}

func (c *client) configureContextMenu(window *mygo.Window) {
	if runtime.GOOS != "windows" {
		return
	}
	page := window.Page()
	page.OnContextMenu(func(event *mygo.ContextMenuEvent) {
		script := nativeContextMenuScript(c.origin, page.URL(), event.FrameURL)
		if script == "" || c.ctx.Err() != nil {
			return
		}
		// Existing page menus call preventDefault and never reach this handler.
		// The frontend retained the DOM target in its capture-phase listener.
		event.PreventDefault()
		go func() {
			if window.IsDestroyed() || c.ctx.Err() != nil {
				return
			}
			_, _ = page.EvalContext(c.ctx, script)
		}()
	})
}

// Editing and menu presentation stay in the page. Only a direct click on its
// paste action may request clipboard text from the desktop host.
func contextPasteRequest(origin, pageURL, raw string, userInitiated bool) (requestID string, allowed, handled bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "camp-desktop" || u.Host != "context-action" {
		return "", false, false
	}
	if !sameOrigin(origin, pageURL) || u.User != nil || u.Path != "" || u.Fragment != "" {
		return "", false, true
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(query) != 2 || len(query["action"]) != 1 || query.Get("action") != "paste" || len(query["requestId"]) != 1 {
		return "", false, true
	}
	id := query.Get("requestId")
	if len(id) == 0 || len(id) > 80 {
		return "", false, true
	}
	for _, char := range id {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_') {
			return "", false, true
		}
	}
	return id, userInitiated, true
}

func contextPasteResultScript(origin, requestID, text string, failed bool) string {
	encodedOrigin, _ := json.Marshal(origin)
	result := struct {
		RequestID string `json:"requestId"`
		Text      string `json:"text"`
		Error     bool   `json:"error,omitempty"`
	}{RequestID: requestID, Text: text, Error: failed}
	if failed {
		result.Text = ""
	}
	encodedResult, _ := json.Marshal(result)
	return `(()=>{if(window!==top||location.origin!==` + string(encodedOrigin) + `)return;
window.dispatchEvent(new CustomEvent('camp-context-result',{detail:` + string(encodedResult) + `}))})()`
}

// OnWillNavigate only receives top-level navigations. The app keeps MyGo's
// general Go bridge disabled for all hosted pages.
func (c *client) handleContextAction(window *mygo.Window, raw string, userInitiated bool) bool {
	requestID, allowed, handled := contextPasteRequest(c.origin, window.Page().URL(), raw, userInitiated)
	if !handled {
		return false
	}
	if runtime.GOOS != "windows" || requestID == "" {
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
		if window.IsDestroyed() || c.ctx.Err() != nil || !sameOrigin(c.origin, window.Page().URL()) {
			return
		}
		var text string
		failed := !allowed
		if allowed {
			data, err := mygo.Clipboard.ReadFormat(transfer.Text)
			failed = err != nil && !errors.Is(err, transfer.ErrFormat)
			if !failed {
				text = string(data)
			}
		}
		_, _ = window.Page().EvalContext(c.ctx, contextPasteResultScript(c.origin, requestID, text, failed))
	}()
	return true
}
