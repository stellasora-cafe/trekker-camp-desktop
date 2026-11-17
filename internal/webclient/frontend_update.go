// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webfrontend"
)

// frontendWindow tracks the source of the running document independently
// from the downloadable cache. Its revisions are guarded by windowMu.
type frontendWindow struct {
	ready          atomic.Bool
	online         atomic.Bool
	onlineRevision string
	pending        string
}

func (f *frontendWindow) stage(revision string) bool {
	if f.online.Load() {
		if f.onlineRevision == "" {
			f.onlineRevision = revision
		}
		if revision == f.onlineRevision {
			f.pending = ""
			return false
		}
	}
	f.pending = revision
	return true
}

func frontendUpdatePreload(origin string) string {
	encoded, _ := json.Marshal(origin)
	return `;(()=>{
if(window!==top||location.origin!==` + string(encoded) + `)return;
window.__campFrontendCanReload=()=>document.getElementById('misskey_app')?.childElementCount>0;
const check=()=>{location.href='camp-desktop://frontend-check'};
window.addEventListener('online',check);
window.addEventListener('camp-stream-connected',check);
})();`
}

func (c *client) handleFrontendCommand(window *mygo.Window, raw string) bool {
	if (raw != "camp-desktop://frontend-check" && raw != "camp-desktop://frontend-apply" && raw != "camp-desktop://check-updates" && raw != "camp-desktop://about-camp") || !sameOrigin(c.origin, window.Page().URL()) {
		return false
	}
	c.windowMu.Lock()
	owned := c.window == window
	_, tool := c.tools[window]
	c.windowMu.Unlock()
	if owned || tool {
		if raw == "camp-desktop://about-camp" {
			c.openAboutCamp()
		} else if raw == "camp-desktop://check-updates" {
			c.checkAllUpdates()
		} else if raw == "camp-desktop://frontend-apply" {
			if tool {
				go window.Page().LoadURL(window.Page().URL())
			} else {
				go c.applyFrontendUpdate()
			}
		} else {
			go c.refreshFrontend()
			go c.checkMaintenance()
			go c.checkPackageUpdate()
		}
	}
	return true
}

func (c *client) refreshFrontend() {
	if c.inTray.Load() || c.updates == nil || !c.updateChecking.CompareAndSwap(false, true) {
		return
	}
	defer c.updateChecking.Store(false)
	ctx, cancel := context.WithTimeout(c.ctx, 90*time.Second)
	defer cancel()
	revision, err := c.updates.Check(ctx)
	var required *webfrontend.UpdateRequiredError
	if errors.As(err, &required) {
		c.requirePackageUpdate(required.MinimumVersion)
		return
	}
	if errors.Is(err, webfrontend.ErrMaintenance) {
		c.setMaintenance(true)
		return
	}
	if err != nil || revision == "" || c.ctx.Err() != nil {
		return
	}
	_, _ = c.stageFrontend(revision)
}

func (c *client) stageFrontend(revision string) (bool, error) {
	if err := c.updates.Stage(revision); err != nil {
		return false, err
	}
	c.windowMu.Lock()
	w, frontend := c.window, c.mainFrontend
	hadUpdate := frontend != nil && frontend.pending != ""
	ready := frontend != nil && frontend.stage(revision)
	c.windowMu.Unlock()
	if (ready || hadUpdate) && w != nil && !w.IsDestroyed() {
		value := "false"
		if ready {
			value = "true"
		}
		ctx, cancel := context.WithTimeout(c.ctx, 2*time.Second)
		defer cancel()
		_, _ = w.Page().EvalContext(ctx, `document.documentElement.dataset.campFrontendReady='`+value+`';window.dispatchEvent(new Event('camp-frontend-ready'))`)
	}
	return ready, nil
}

func (c *client) frontendUpdateLoop() {
	go c.refreshFrontend()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			go c.refreshFrontend()
		}
	}
}

func (c *client) applyFrontendUpdate() {
	c.updateMu.Lock()
	defer c.updateMu.Unlock()
	if c.ctx.Err() != nil || c.packageRequired.Load() {
		return
	}
	_, current := c.updates.Current()
	if c.updates.Pending(current) {
		return
	}
	c.windowMu.Lock()
	// The native welcome screen has no page to acknowledge a successful boot.
	// Apply after opening the main WebView; login windows keep their version.
	if c.window == nil || c.mainFrontend == nil || c.changingWindow {
		c.windowMu.Unlock()
		return
	}
	window, frontend := c.window, c.mainFrontend
	c.windowMu.Unlock()
	if window.IsDestroyed() {
		return
	}
	ctx, cancel := context.WithTimeout(c.ctx, 2*time.Second)
	defer cancel()
	safe, err := window.Page().EvalContext(ctx, `window.__campFrontendCanReload?.()===true`)
	if err != nil || safe != true {
		return
	}
	c.windowMu.Lock()
	if c.window != window || c.changingWindow {
		c.windowMu.Unlock()
		return
	}
	revision := frontend.pending
	c.windowMu.Unlock()
	if revision != "" {
		if err := c.updates.Activate(revision); err != nil {
			return
		}
		frontend.online.Store(false)
		c.windowMu.Lock()
		if frontend.pending == revision {
			frontend.pending = ""
		}
		c.windowMu.Unlock()
	}
	target := window.Page().URL()
	_ = window.Page().LoadURL(target)
	if revision == "" {
		return
	}
	c.watchFrontendBoot(window, target, revision)
}

func (c *client) watchFrontendBoot(window *mygo.Window, target, revision string) {
	if revision == "" || !c.updates.Pending(revision) {
		return
	}
	go func() {
		timer := time.NewTimer(45 * time.Second)
		defer timer.Stop()
		select {
		case <-c.ctx.Done():
			return
		case <-timer.C:
		}
		if !c.updates.Pending(revision) {
			return
		}
		if c.updates.Rollback(revision) == nil && !window.IsDestroyed() {
			if current, _ := c.updates.Current(); current == nil {
				c.openBootstrap()
			} else {
				_ = window.Page().LoadURL(target)
			}
		}
	}()
}
