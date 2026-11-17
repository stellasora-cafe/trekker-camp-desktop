// SPDX-FileCopyrightText: stellasora-cafe contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webfrontend"
)

const startupSessionPath = "/__camp-desktop/startup-session"

// Read the actual, encrypted WebView account at its own origin. This document
// is served locally and has no network dependencies or duplicated login state.
const startupSessionScript = `(()=>{
let authenticated=false;
try {
 const account=JSON.parse(localStorage.getItem('account'));
 authenticated=typeof account?.id==='string'&&account.id.length>0&&typeof account?.token==='string'&&account.token.length>0;
} catch {}
location.href='camp-desktop://startup-session?authenticated='+(authenticated?'1':'0');
})();`

func (c *client) openBootstrap() {
	mygo.RunOnMain(func() {
		if c.ctx.Err() != nil {
			return
		}
		target := c.origin + startupSessionPath
		window := mygo.NewWindow(mygo.WindowOptions{
			Title: "旅人营地", Width: 420, Height: 460, Hidden: true,
			Page: mygo.PageOptions{
				Untrusted: true, Session: c.session, PreloadScript: c.credentials.preload(),
				ResourceFilter: func(raw string) bool { return raw == target || raw == c.origin+credentialPath },
				ResourceHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.String() == c.origin+credentialPath {
						c.credentials.ServeHTTP(w, r)
						return
					}
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					w.Header().Set("Cache-Control", "no-store")
					w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; connect-src 'self'")
					_, _ = w.Write([]byte("<!doctype html><meta charset=utf-8><script>" + startupSessionScript + "</script>"))
				}),
			},
		})
		previous := c.setMainWindow(window, false, nil)
		var resolved atomic.Bool
		finish := func(authenticated bool) {
			mygo.RunOnMain(func() {
				if c.ctx.Err() != nil || window.IsDestroyed() || !resolved.CompareAndSwap(false, true) {
					return
				}
				if authenticated {
					go c.openMain(false)
				} else {
					c.openGuest()
				}
				go c.checkMaintenance()
			})
		}
		timer := time.AfterFunc(8*time.Second, func() { finish(false) })
		window.OnClosed(func() { timer.Stop() })
		window.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
			if e.URL == target {
				return
			}
			e.PreventDefault()
			if window.Page().URL() != target {
				return
			}
			if e.URL == "camp-desktop://startup-session?authenticated=1" {
				finish(true)
			}
			if e.URL == "camp-desktop://startup-session?authenticated=0" {
				finish(false)
			}
		})
		go func() {
			if window.Page().LoadURL(target) != nil {
				finish(false)
			}
		}()
		if previous != nil {
			previous.Destroy()
		}
	})
}

// The requested tool can open as soon as its compatibility check completes.
func (c *client) loadGuestTool(parent context.Context, path string) error {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	if err := c.updates.CheckCompatibility(ctx); err != nil {
		var required *webfrontend.UpdateRequiredError
		if errors.As(err, &required) {
			c.requirePackageUpdate(required.MinimumVersion)
			return errors.New("请先完成客户端更新，再重试。")
		}
		if errors.Is(err, webfrontend.ErrMaintenance) {
			c.showMaintenance(c.origin+"/__camp-maintenance/v1.html#/", c.origin+webfrontend.UpdatePath)
			return nil
		}
		return errors.New("加载失败，请检查网络后重试。")
	}
	window, ready := c.openToolPage(c.origin + path)
	if window == nil {
		return errors.New("页面未能打开，请重试。")
	}
	select {
	case err := <-ready:
		if errors.Is(err, webfrontend.ErrMaintenance) {
			return nil
		}
		if err != nil {
			window.Destroy()
			return errors.New("加载失败，请检查网络后重试。")
		}
		return nil
	case <-ctx.Done():
		window.Destroy()
		return errors.New("加载超时，请重试。")
	}
}

func (c *client) startConnected() {
	c.frontendLoops.Do(func() {
		go c.refreshAssets()
		go c.frontendUpdateLoop()
		go c.packageUpdateLoop()
	})
}
