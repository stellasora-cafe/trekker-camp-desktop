// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"context"
	"github.com/egoist/mygo"
	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webfrontend"
	"strconv"
	"time"
)

func (c *client) maintenanceLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		c.checkMaintenance()
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *client) checkMaintenance() {
	if c.inTray.Load() {
		return
	}
	ctx, cancel := context.WithTimeout(c.ctx, 4*time.Second)
	defer cancel()
	active, err := c.probeMaintenance(ctx)
	if err == nil {
		c.setMaintenance(active)
	}
}

func (c *client) setMaintenance(active bool) {
	c.maintenanceState.Store(active)
	_, windows := c.maintenanceTargets()
	if !active {
		c.closeMaintenanceWindow()
	}
	for _, w := range windows {
		c.sendMaintenance(w)
	}
}

// Maintenance follows the current window, independently of the frontend cache.
func (c *client) maintenanceTargets() (bool, []*mygo.Window) {
	c.windowMu.Lock()
	defer c.windowMu.Unlock()
	windows := make([]*mygo.Window, 0, len(c.tools)+1)
	if c.window != nil && !c.guest && c.mainFrontend != nil {
		windows = append(windows, c.window)
	}
	for w := range c.tools {
		windows = append(windows, w)
	}
	return c.guest, windows
}

func (c *client) sendMaintenance(w *mygo.Window) {
	if w.IsDestroyed() || w.Page() == nil {
		return
	}
	ctx, cancel := context.WithTimeout(c.ctx, 2*time.Second)
	defer cancel()
	_, _ = w.Page().EvalContext(ctx, `(()=>{const value='`+strconv.FormatBool(c.maintenanceState.Load())+`';if(document.documentElement.dataset.campMaintenance===value)return;document.documentElement.dataset.campMaintenance=value;window.dispatchEvent(new Event('camp-server-maintenance'));})()`)
}

// Recovery requires every entrypoint previously redirected into maintenance to
// recover, even when the site root remains available throughout maintenance.
func (c *client) probeMaintenance(ctx context.Context) (bool, error) {
	targets := map[string]struct{}{c.origin + "/": {}}
	c.maintenanceRetries.Range(func(_, value any) bool { targets[value.(string)] = struct{}{}; return true })
	c.maintenanceProbeTargets.Range(func(key, _ any) bool { targets[key.(string)] = struct{}{}; return true })
	var failure error
	for target := range targets {
		active, err := webfrontend.ProbeTarget(ctx, c.origin, target)
		if active {
			return true, nil
		}
		if err != nil {
			failure = err
		}
	}
	return false, failure
}

// A main window without a mounted frontend must load the server's maintenance
// document; sending a frontend event to its empty page cannot make it visible.
func (c *client) mainNeedsMaintenanceDocument(window *mygo.Window) bool {
	c.windowMu.Lock()
	defer c.windowMu.Unlock()
	return c.window == window && !c.guest && c.mainFrontend != nil && !c.mainFrontend.ready.Load()
}
