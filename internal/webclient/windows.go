// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webfrontend"
)

func (c *client) pageOptions(tool, offline bool, useServerFrontend func() bool) mygo.PageOptions {
	var pinnedMu sync.Mutex
	var pinned *webfrontend.Bundle
	if !useServerFrontend() {
		pinned = c.frontend
		if c.updates != nil {
			if current, _ := c.updates.Current(); current != nil {
				pinned = current
			}
		}
	}
	frontendForPath := func(path string) *webfrontend.Bundle {
		if useServerFrontend() {
			return nil
		}
		pinnedMu.Lock()
		defer pinnedMu.Unlock()
		if webfrontend.IsDocumentPath(path) && c.updates != nil {
			pinned, _ = c.updates.Current()
		}
		return pinned
	}
	settings := c.closeBehaviorEnabled(tool)
	trayNotices := !tool && runtime.GOOS == "windows"
	preload := maintenancePreload(c.origin+"/__camp-maintenance/v1.html", runtime.GOOS, false) + c.credentials.preload() + c.preload + hostPreload(c.origin, c.safeMode) + windowChromePreload(c.origin, runtime.GOOS) + windowPreload(c.origin, tool, offline) + frontendUpdatePreload(c.origin)
	if settings {
		preload += c.closeBehaviorPreload()
	}
	return mygo.PageOptions{
		Untrusted: true, Session: c.session,
		PersistentStorageOrigin: c.origin,
		PreloadScript:           preload,
		ResourceFilter: func(raw string) bool {
			if !sameOrigin(c.origin, raw) {
				return false
			}
			u, err := url.Parse(raw)
			if err != nil {
				return false
			}
			if trayNotices && u.Path == trayNoticePath {
				return true
			}
			b := frontendForPath(u.Path)
			return (settings && u.Path == closeBehaviorPath) || (c.credentials != nil && u.Path == credentialPath) || (b != nil && (b.Contains(u.Path) || webfrontend.IsDocumentPath(u.Path))) || c.cachedResource(raw)
		},
		ResourceHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if trayNotices && r.URL.Path == trayNoticePath {
				c.serveTrayNotice(w, r)
				return
			}
			if settings && r.URL.Path == closeBehaviorPath {
				c.serveCloseBehavior(w, r)
				return
			}
			c.serveFrontendResource(w, r, frontendForPath(r.URL.Path))
		}),
	}
}

func (c *client) localResource(raw string) bool {
	if !sameOrigin(c.origin, raw) {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if c.frontend != nil && (c.frontend.Contains(u.Path) || webfrontend.IsDocumentPath(u.Path)) {
		return true
	}
	return c.cachedResource(raw)
}

func (c *client) cachedResource(raw string) bool {
	if c.media != nil && c.media.Matches(raw) {
		return true
	}
	assets := c.assets.Load()
	return assets != nil && assets.CachedURL(raw) != ""
}

func (c *client) serveLocalResource(w http.ResponseWriter, r *http.Request) {
	c.serveFrontendResource(w, r, c.frontend)
}

func (c *client) serveFrontendResource(w http.ResponseWriter, r *http.Request, frontend *webfrontend.Bundle) {
	if !sameOrigin(c.origin, r.URL.String()) {
		http.NotFound(w, r)
		return
	}
	if c.credentials != nil && r.URL.Path == credentialPath {
		c.credentials.ServeHTTP(w, r)
		return
	}
	if assets := c.assets.Load(); assets != nil {
		if cached := assets.CachedURL(r.URL.String()); cached != "" {
			local := r.Clone(r.Context())
			local.URL, _ = url.Parse(cached)
			assets.Handler().ServeHTTP(w, local)
			return
		}
	}
	if c.media != nil && c.media.Matches(r.URL.String()) {
		c.media.ServeHTTP(w, r)
		return
	}
	if frontend == nil {
		http.NotFound(w, r)
		return
	}
	frontend.Handler().ServeHTTP(w, r)
}

// The hidden cached page restores the actual WebView account before selecting
// the native welcome window or the full desktop interface.
func (c *client) openMain(afterLogin bool) {
	if c.ctx.Err() != nil || c.packageRequired.Load() {
		return
	}
	frontend := &frontendWindow{}
	if c.updates != nil {
		current, _ := c.updates.Current()
		frontend.online.Store(afterLogin || current == nil)
		if frontend.online.Load() {
			ctx, cancel := context.WithTimeout(c.ctx, 2*time.Second)
			frontend.onlineRevision, _ = c.updates.AvailableRevision(ctx)
			cancel()
		}
	}
	if c.ctx.Err() != nil || c.packageRequired.Load() {
		return
	}
	options := mygo.WindowOptions{
		Title: "旅人营地", Width: 1280, Height: 800, UseContentSize: true,
		MinWidth: 480, MinHeight: 480, BackgroundColor: "#edf2f8", StateKey: "main", Hidden: true,
		Page: c.pageOptions(false, false, frontend.online.Load),
	}
	applyWindowChrome(&options, runtime.GOOS)
	window := mygo.NewWindow(options)
	previous := c.setMainWindow(window, false, frontend)
	c.attachTrayClose(window)
	c.configurePage(window, false)
	if runtime.GOOS == "windows" {
		_ = window.Page().LoadURL(c.origin + "/")
		if c.updates != nil && !frontend.online.Load() {
			_, revision := c.updates.Current()
			c.watchFrontendBoot(window, c.origin+"/", revision)
		}
	} else {
		c.connect()
		window.Show()
	}
	if previous != nil {
		previous.Destroy()
	}
}

func (c *client) openGuest() {
	c.showGuestError("")
}

func (c *client) showGuestError(message string) {
	if c.ctx.Err() != nil || c.packageRequired.Load() {
		return
	}
	panel := newGuestPanel(c.ctx, c.origin, c.directory, guestCallbacks{
		OpenTool:     func(path string) { go c.openTool(c.origin + path) },
		LoadTool:     c.loadGuestTool,
		CheckUpdates: c.checkAllUpdates,
		AboutCamp:    c.openAboutCamp,
	})
	panel.loadError = message
	c.showGuestPanel(panel)
}

func (c *client) showGuestPanel(panel *guestPanel) {
	window := mygo.NewWindow(mygo.WindowOptions{
		Title: "旅人营地", Width: 420, Height: 460,
		UseContentSize: true, Frameless: true, DisableResize: true, DisableMaximize: true,
		BackgroundColor: "#ffffff", Content: ui.View(panel.View), Hidden: true,
	})
	panel.resourceDir = c.resourceDir
	panel.Attach(window)
	previous := c.setMainWindow(window, true, nil)
	window.OnClosed(panel.Close)
	c.showReadyWindow(window)
	if previous != nil {
		previous.Destroy()
	}
}

func (c *client) setMainWindow(window *mygo.Window, guest bool, frontend *frontendWindow) *mygo.Window {
	c.windowMu.Lock()
	previous := c.window
	c.window, c.guest, c.changingWindow = window, guest, false
	c.mainFrontend = frontend
	c.windowMu.Unlock()
	window.OnClosed(func() {
		c.windowMu.Lock()
		current := c.window == window
		tools := make([]*mygo.Window, 0, len(c.tools))
		for tool := range c.tools {
			tools = append(tools, tool)
		}
		c.windowMu.Unlock()
		if !current {
			return
		}
		c.loading.close()
		c.cancel()
		c.closePermissionPrompts()
		mygo.RunOnMain(func() {
			if c.maintenanceWindow != nil {
				c.maintenanceWindow.Destroy()
			}
			if c.aboutWindow != nil {
				c.aboutWindow.Destroy()
			}
		})
		for _, tool := range tools {
			tool.Destroy()
		}
	})
	return previous
}

func (c *client) openTool(raw string) { c.openToolPage(raw) }

func isAuthTool(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Path == "/desktop-auth"
}

func (c *client) openToolPage(raw string) (*mygo.Window, <-chan error) {
	if c.ctx.Err() != nil || c.packageRequired.Load() || !sameOrigin(c.origin, raw) {
		return nil, nil
	}
	auth := isAuthTool(raw)
	options := mygo.WindowOptions{
		Title: "旅人营地", Width: 960, Height: 760, MinWidth: 420, MinHeight: 400,
		// Authentication takes its document and modules from the same server
		// revision, even after the main window has cached a frontend bundle.
		UseContentSize: true, BackgroundColor: "#edf2f8", Page: c.pageOptions(true, false, func() bool { return auth }),
	}
	applyWindowChrome(&options, runtime.GOOS)
	options.Hidden = runtime.GOOS == "windows"
	if auth {
		u, _ := url.Parse(raw)
		options.Width, options.Height, options.MinWidth, options.MinHeight = 440, 480, 400, 420
		options.Title = "登录 · 旅人营地"
		if u.Query().Get("mode") == "signup" {
			options.Width, options.Height, options.Title = 480, 660, "注册 · 旅人营地"
		}
	}
	window := mygo.NewWindow(options)
	c.windowMu.Lock()
	if c.ctx.Err() != nil {
		c.windowMu.Unlock()
		window.Destroy()
		return nil, nil
	}
	c.tools[window] = struct{}{}
	ready := make(chan error, 1)
	c.toolReady[window] = ready
	c.windowMu.Unlock()
	c.configurePage(window, true)
	window.Page().OnDidFailLoad(func(err *mygo.LoadError) {
		if err.URL != raw {
			return
		}
		select {
		case ready <- err:
		default:
		}
	})
	window.OnClosed(func() {
		c.windowMu.Lock()
		delete(c.tools, window)
		delete(c.toolReady, window)
		c.windowMu.Unlock()
		select {
		case ready <- errors.New("page closed"):
		default:
		}
	})
	c.navigationTargets.Store(window, raw)
	if err := window.Page().LoadURL(raw); err != nil {
		select {
		case ready <- err:
		default:
		}
	}
	if runtime.GOOS != "windows" {
		window.Show()
	}
	return window, ready
}

func (c *client) handleWindowCommand(window *mygo.Window, raw string) bool {
	if !strings.HasPrefix(raw, "camp-desktop://window-state?") || !sameOrigin(c.origin, window.Page().URL()) {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return true
	}
	value := u.Query().Get("guest")
	network := u.Query().Get("offline")
	if (value != "0" && value != "1") || (network != "0" && network != "1") {
		return true
	}
	guest := value == "1"
	offline := network == "1"
	go func() {
		c.windowMu.Lock()
		main := c.window == window
		if main && c.mainFrontend != nil {
			c.mainFrontend.ready.Store(true)
		}
		_, tool := c.tools[window]
		ready := c.toolReady[window]
		toolLogin := tool && c.guest && !guest && !offline
		change := !c.changingWindow && ((main && guest && !offline && !c.maintenanceState.Load()) || toolLogin)
		if change {
			c.changingWindow = true
		}
		c.windowMu.Unlock()
		if main && c.updates != nil {
			go c.sendMaintenance(window)
			_ = c.updates.Confirm(u.Query().Get("frontend"))
			if !offline {
				go c.refreshFrontend()
			}
		}
		if window.IsDestroyed() || c.ctx.Err() != nil {
			return
		}
		if tool && ready != nil {
			var loadErr error
			if offline {
				loadErr = errors.New("page offline")
			}
			select {
			case ready <- loadErr:
			default:
			}
		}
		if tool && !offline {
			c.showReadyWindow(window)
		}
		if !offline && (tool || (main && !guest)) {
			c.startConnected()
		}
		if change {
			if main && guest && !offline {
				c.openGuest()
			} else {
				c.openMain(true)
				if u, err := url.Parse(window.Page().URL()); !offline && err == nil && u.Path == "/desktop-auth" {
					window.Destroy()
				}
			}
		} else if main && (offline || !guest) {
			c.showReadyWindow(window)
		}
	}()
	return true
}

func windowPreload(origin string, tool, offline bool) string {
	config, _ := json.Marshal(struct {
		Origin  string `json:"origin"`
		Tool    bool   `json:"tool"`
		Offline bool   `json:"offline"`
	}{origin, tool, offline})
	return `;(()=>{const config=` + string(config) + `;
if(window!==top||location.origin!==config.origin)return;
const isGuest=()=>{try{const a=JSON.parse(localStorage.getItem('account'));return !(a?.id&&a?.token)}catch{return true}};
const guest=isGuest();
const mark=()=>{if(!document.documentElement)return false;
document.documentElement.dataset.campDesktopWindow=config.tool?'tool':'main';
if(config.offline)document.documentElement.dataset.campConnection='offline';
if(guest&&!config.tool)document.documentElement.dataset.campDesktopGuest='true';return true;};
if(!mark()){const o=new MutationObserver(()=>{if(mark())o.disconnect()});o.observe(document,{childList:true})}
const install=()=>{
const ready=()=>document.getElementById('misskey_app')?.childElementCount>0;
let last='';
const notify=()=>{const state=document.documentElement.dataset.campConnection;
if(!ready()||(state!=='online'&&state!=='offline'))return;
const next='camp-desktop://window-state?guest='+(isGuest()?'1':'0')+'&offline='+(state==='offline'?'1':'0')+(document.documentElement.dataset.campFrontend?'&frontend='+document.documentElement.dataset.campFrontend:'');
if(next===last)return;last=next;location.href=next;};
window.addEventListener('camp-connection-change',notify);
window.addEventListener('storage',e=>{if(e.key==='account')notify()});
if(ready())notify();
else {const observer=new MutationObserver(()=>{if(ready()){observer.disconnect();requestAnimationFrame(notify)}});
observer.observe(document.body,{childList:true,subtree:true});}};
if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',install,{once:true});else install();})();`
}
