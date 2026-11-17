// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webfrontend"
)

func (c *client) maintenanceReturnURL(window *mygo.Window) string {
	if target, ok := c.navigationTargets.Load(window); ok {
		return target.(string)
	}
	if target := window.Page().URL(); sameOrigin(c.origin, target) && !webfrontend.IsMaintenanceURL(c.origin, target) {
		return target
	}
	return c.origin + "/desktop-auth?mode=signin"
}

func (c *client) showMaintenance(raw string, targets ...string) {
	if !webfrontend.IsMaintenanceURL(c.origin, raw) {
		return
	}
	for _, target := range targets {
		if sameOrigin(c.origin, target) {
			c.maintenanceProbeTargets.Store(target, struct{}{})
		}
	}
	c.maintenanceState.Store(true)
	c.openMaintenanceWindow(raw)
}

func (c *client) closeMaintenanceWindow() {
	mygo.RunOnMain(func() {
		if c.maintenanceState.Load() {
			return
		}
		if c.maintenanceWindow != nil && !c.maintenanceWindow.IsDestroyed() {
			c.maintenanceWindow.Destroy()
		}
		c.maintenanceWindow = nil
		c.maintenanceProbeTargets.Range(func(key, value any) bool { c.maintenanceProbeTargets.Delete(key); return true })
		c.maintenanceRetries.Range(func(key, value any) bool {
			window := key.(*mygo.Window)
			c.maintenanceRetries.Delete(key)
			if !window.IsDestroyed() {
				go window.Page().LoadURL(value.(string))
			}
			return true
		})
	})
}

func (c *client) openMaintenanceWindow(raw string) {
	mygo.RunOnMain(func() {
		if c.inTray.Load() || c.ctx.Err() != nil || !c.maintenanceState.Load() {
			return
		}
		guest, windows := c.maintenanceTargets()
		if !guest {
			if c.maintenanceWindow != nil && !c.maintenanceWindow.IsDestroyed() {
				c.maintenanceWindow.Destroy()
			}
			c.maintenanceWindow = nil
			for _, window := range windows {
				go c.sendMaintenance(window)
			}
			return
		}
		if c.maintenanceWindow != nil && !c.maintenanceWindow.IsDestroyed() {
			return
		}
		options := mygo.WindowOptions{Title: "营地维护中", Width: 760, Height: 820, MinWidth: 440, MinHeight: 500, UseContentSize: true, BackgroundColor: "#eef2f8", Page: mygo.PageOptions{Untrusted: true, PreloadScript: maintenancePreload(raw, runtime.GOOS, true)}}
		applyWindowChrome(&options, runtime.GOOS)
		window := mygo.NewWindow(options)
		c.maintenanceWindow = window
		// Use the actual redirect origin (maintenance may live at the production edge).
		origin := strings.Split(raw, "/__camp-maintenance/")[0]
		chrome := &client{origin: origin, ctx: c.ctx, window: window}
		chrome.configureWindowChrome(window)
		page := window.Page()
		page.OnContextMenu(func(e *mygo.ContextMenuEvent) { e.PreventDefault() })
		page.OnPageTitleUpdated(func(e *mygo.TitleEvent) { e.PreventDefault() })
		page.OnWillNavigate(func(e *mygo.NavigateEvent) {
			if chrome.handleWindowControl(window, e.URL) {
				e.PreventDefault()
				return
			}
			if !webfrontend.IsMaintenanceURL(c.origin, e.URL) {
				e.PreventDefault()
			}
		})
		page.SetWindowOpenHandler(func(mygo.WindowOpenRequest) *mygo.WindowOptions { return nil })
		pollContext, stopPolling := context.WithCancel(c.ctx)
		go c.pollMaintenanceWindow(pollContext)
		window.OnClosed(func() {
			stopPolling()
			if c.maintenanceWindow == window {
				c.maintenanceWindow = nil
			}
		})
		go page.LoadURL(raw)
	})
}

func maintenancePreload(raw, platform string, hideActions bool) string {
	origin := strings.Split(raw, "/__camp-maintenance/")[0]
	encodedOrigin, _ := json.Marshal(origin)
	chrome := ""
	if platform == "windows" {
		chrome = `<div id="camp-maintenance-caption"><div data-drag></div><button data-action="minimize" aria-label="最小化">` + string(captionAsset("minimize")) + `</button><button data-action="maximize" aria-label="最大化"><span class="maximize">` + string(captionAsset("maximize")) + `</span><span class="restore">` + string(captionAsset("restore")) + `</span></button><button data-action="close" aria-label="关闭">` + string(captionAsset("close")) + `</button></div>`
	}
	encodedChrome, _ := json.Marshal(chrome)
	actionsCSS, actionsScript := "", ""
	if hideActions {
		actionsCSS = ".actions,[data-camp-offline-toolbox]{display:none!important}"
		actionsScript = "document.querySelectorAll('.actions,[data-camp-offline-toolbox]').forEach(node=>node.remove());"
	}
	return windowChromePreload(origin, platform) + `;(()=>{if(window!==top||location.origin!==` + string(encodedOrigin) + `||location.pathname!=="/__camp-maintenance/v1.html")return;
const css='` + actionsCSS + `#camp-maintenance-caption{position:fixed;inset:0 0 auto;height:50px;display:flex;z-index:2147483646;background:#eef2f8;color:#4b5082;user-select:none}#camp-maintenance-caption [data-drag]{flex:1}#camp-maintenance-caption button{width:42px;height:50px;border:0;padding:0;border-radius:0;display:grid;place-items:center;color:inherit;background:transparent;transition:background-color 100ms,color 100ms}#camp-maintenance-caption button:hover{background:#dce5ee}#camp-maintenance-caption button[data-action=close]:hover{background:#e81123;color:white}#camp-maintenance-caption svg{width:10px;height:10px;pointer-events:none}#camp-maintenance-caption span{display:flex}#camp-maintenance-caption .restore{display:none}[data-camp-window-maximized=true] #camp-maintenance-caption .maximize{display:none}[data-camp-window-maximized=true] #camp-maintenance-caption .restore{display:flex}[data-camp-window-active=false] #camp-maintenance-caption button:not(:hover){color:#9297af}[data-camp-window-chrome=true] body{padding-top:70px!important}@media(prefers-reduced-motion:reduce){#camp-maintenance-caption button{transition:none}}';
const install=()=>{if(!document.documentElement)return;
if(!document.getElementById('camp-maintenance-style')){const style=document.createElement('style');style.id='camp-maintenance-style';style.textContent=css;document.documentElement.append(style)}
if(!document.body)return;
` + actionsScript + `
const html=` + string(encodedChrome) + `;if(html&&!document.getElementById('camp-maintenance-caption')){const holder=document.createElement('div');holder.innerHTML=html;const caption=holder.firstElementChild;document.body.prepend(caption);const command=action=>location.href='camp-desktop://window-control?action='+action;
caption.querySelectorAll('button').forEach(button=>button.addEventListener('click',()=>command(button.dataset.action)));const drag=caption.querySelector('[data-drag]');drag.addEventListener('mousedown',event=>{if(event.button===0&&event.detail===1)command('drag')});drag.addEventListener('dblclick',()=>command('maximize'));}
};new MutationObserver(install).observe(document,{childList:true,subtree:true});install();})();`
}

// Guest windows do not start the main frontend loops. Keep recovery independent.
func (c *client) pollMaintenanceWindow(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			probe, cancel := context.WithTimeout(ctx, 4*time.Second)
			active, err := c.probeMaintenance(probe)
			cancel()
			if err == nil && !active {
				c.setMaintenance(false)
				return
			}
		}
	}
}
