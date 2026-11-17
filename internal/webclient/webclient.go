// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

// Package webclient hosts the camp website in an isolated, persistent WebView.
package webclient

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/stellasora-cafe/trekker-camp-desktop/internal/campweb"
	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webassets"
	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webfrontend"
	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webmedia"
)

type client struct {
	origin                  string
	directory               string
	applicationDir          string
	resourceDir             string
	packageChecking         atomic.Bool
	packageRequired         atomic.Bool
	packagePrompt           *packageUpdatePrompt
	aboutWindow             *mygo.Window
	maintenanceWindow       *mygo.Window
	navigationTargets       sync.Map
	maintenanceRetries      sync.Map
	maintenanceProbeTargets sync.Map
	frontendLoops           sync.Once
	credentials             *credentialCipher
	window                  *mygo.Window
	ctx                     context.Context
	cancel                  context.CancelFunc
	loading                 loadGate
	assets                  atomic.Pointer[webassets.Bundle]
	frontend                *webfrontend.Bundle
	updates                 *webfrontend.Updates
	updateChecking          atomic.Bool
	updateMu                sync.Mutex
	mainFrontend            *frontendWindow
	maintenanceState        atomic.Bool
	media                   *webmedia.Cache
	session                 *mygo.Session
	preload                 string
	safeMode                *bool
	guest                   bool
	changingWindow          bool
	windowMu                sync.Mutex
	tools                   map[*mygo.Window]struct{}
	toolReady               map[*mygo.Window]chan error
	permissionWindow        *mygo.Window
	permissionQueue         []*permissionDecision
	permissionFontPending   bool
	inTray                  atomic.Bool
	tray                    *mygo.Tray
	trayWindows             map[*mygo.Window]bool
	trayClosing             bool
	closeBehavior           *closeBehaviorStore
	quitting                bool
}

// Open creates the main window immediately. Call it from App.WhenReady after
// setting the application's user-data directory and language.
func Open(origin, directory string, safeMode *bool) error {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	version := mygo.App.Version()
	if version == "" {
		version = "0.1.0"
	}
	preload, err := campweb.Preload(origin, runtime.GOOS, version)
	if err != nil {
		return err
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		return err
	}
	applicationDir, err := applicationDirectory(directory)
	if err != nil {
		return err
	}
	if err := migrateApplicationCache(directory, applicationDir, origin); err != nil {
		return err
	}
	resourceDir := siteResourceDirectory(applicationDir, origin)
	registerInstalledFont(filepath.Join(applicationDir, "resources", "camp"))
	ctx, cancel := context.WithCancel(context.Background())
	updates, err := webfrontend.OpenUpdates(origin, filepath.Join(applicationDir, "frontend-cache", hostCacheVersion()), version)
	if err != nil {
		cancel()
		return err
	}
	credentials, err := openCredentialCipher(origin, directory)
	if err != nil {
		cancel()
		return err
	}
	frontend, _ := updates.Current()
	c := &client{origin: origin, directory: directory, applicationDir: applicationDir, resourceDir: resourceDir, credentials: credentials, ctx: ctx, cancel: cancel, frontend: frontend, updates: updates,
		preload: preload, safeMode: safeMode, session: mygo.NewSession(origin + directory), tools: make(map[*mygo.Window]struct{}), toolReady: make(map[*mygo.Window]chan error)}
	c.closeBehavior = &closeBehaviorStore{filename: filepath.Join(directory, "close-behavior")}
	c.media = webmedia.New(origin, filepath.Join(directory, "media"))
	if cached, err := webassets.LoadCached(origin, resourceDir, filepath.Join(applicationDir, "resources", "camp")); err == nil {
		c.assets.Store(cached)
	}
	if err := mygo.Protocol.Handle(webassets.Scheme, c.assetHandler()); err != nil {
		cancel()
		return err
	}
	c.configureTrayLifecycle()
	c.openBootstrap()
	go c.maintenanceLoop()
	return nil
}

func (c *client) configurePage(window *mygo.Window, tool bool) {
	window.OnClosed(func() { c.navigationTargets.Delete(window); c.maintenanceRetries.Delete(window) })
	page := window.Page()
	c.configureWindowChrome(window)
	c.configureContextMenu(window)
	page.OnPageTitleUpdated(func(e *mygo.TitleEvent) { e.PreventDefault() })
	page.OnWillNavigate(func(e *mygo.NavigateEvent) {
		if c.ctx.Err() != nil {
			e.PreventDefault()
			return
		}
		if webfrontend.IsMaintenanceURL(c.origin, e.URL) {
			c.maintenanceRetries.Store(window, c.maintenanceReturnURL(window))
			if c.mainNeedsMaintenanceDocument(window) {
				c.maintenanceState.Store(true)
				c.showReadyWindow(window)
				return
			}
			e.PreventDefault()
			c.windowMu.Lock()
			ready := c.toolReady[window]
			c.windowMu.Unlock()
			if ready != nil {
				select {
				case ready <- webfrontend.ErrMaintenance:
				default:
				}
			}
			go c.showMaintenance(e.URL)
			return
		}
		if c.handleFrontendCommand(window, e.URL) || c.handleWindowControl(window, e.URL) || c.handleContextAction(window, e.URL, e.UserInitiated) || c.handleWindowCommand(window, e.URL) {
			e.PreventDefault()
			return
		}
		action := navigation(c.origin, e.URL, e.UserInitiated)
		if action == navigateHere {
			c.navigationTargets.Store(window, e.URL)
			return
		}
		e.PreventDefault()
		switch action {
		case retryConnection:
			go page.LoadURL(c.origin + "/")
		case openExternal:
			go mygo.Shell.OpenExternal(e.URL)
		}
	})
	page.SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
		if c.ctx.Err() != nil {
			return nil
		}
		browserAuth := browserAuthorizationTarget(c.origin, page.URL(), req.URL)
		if req.FrameName == "camp-desktop-browser-auth" || browserAuth {
			if browserAuth {
				go mygo.Shell.OpenExternal(req.URL)
			}
			return nil
		}
		switch navigation(c.origin, req.URL, req.UserInitiated) {
		case navigateHere:
			if req.FrameName == "camp-desktop-tool" {
				go c.openTool(req.URL)
			} else {
				go page.LoadURL(req.URL)
			}
		case openExternal:
			go mygo.Shell.OpenExternal(req.URL)
		}
		return nil
	})
	page.OnWillDownload(func(e *mygo.DownloadEvent) {
		if c.ctx.Err() != nil || !sameOrigin(c.origin, page.URL()) {
			e.PreventDefault()
			return
		}
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: window, Title: "保存文件", DefaultPath: e.Path, ButtonLabel: "保存",
		})
		if err != nil || path == "" || c.ctx.Err() != nil {
			e.PreventDefault()
			return
		}
		e.Path = path
	})
	page.SetPermissionRequestHandler(c.requestPermission)
}

func (c *client) refreshAssets() {
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	bundle, err := webassets.Prepare(ctx, c.origin, c.resourceDir, filepath.Join(c.applicationDir, "resources", "camp"))
	if err != nil || c.ctx.Err() != nil {
		return
	}
	c.assets.Store(bundle)
	c.windowMu.Lock()
	window := c.window
	c.windowMu.Unlock()
	if window == nil || window.IsDestroyed() {
		return
	}
	if icon, err := os.ReadFile(filepath.Join(c.resourceDir, "public", "branding", "app-icon-192-v1.png")); err == nil {
		_ = window.SetIcon(icon)
	}
	if page := window.Page(); page != nil {
		_, _ = page.Eval(`(()=>{const link=document.getElementById('camp-local-assets');if(link?.tagName==='LINK')link.href=link.href.split('?')[0]+'?v='+Date.now()})()`)
	}
}

func (c *client) connect() {
	if !c.loading.begin() {
		return
	}
	c.window.Page().LoadHTML(connectionDocument(nil), "about:blank")
	go func() {
		defer c.loading.finish()
		ctx, cancel := context.WithTimeout(c.ctx, 20*time.Second)
		defer cancel()
		type htmlResult struct {
			document string
			err      error
		}
		htmlDone := make(chan htmlResult, 1)
		go func() {
			document, err := fetchHTML(ctx, c.origin)
			htmlDone <- htmlResult{document, err}
		}()
		bundle, assetErr := webassets.Prepare(ctx, c.origin, c.resourceDir)
		result := <-htmlDone
		if c.ctx.Err() != nil {
			return
		}
		if result.err == nil {
			result.err = assetErr
		}
		if result.err == nil {
			result.document, result.err = bundle.HTML(result.document)
		}
		if result.err != nil {
			c.window.Page().LoadHTML(connectionDocument(result.err), "about:blank")
			return
		}
		c.assets.Store(bundle)
		if icon, err := os.ReadFile(filepath.Join(c.resourceDir, "public", "branding", "app-icon-192-v1.png")); err == nil {
			_ = c.window.SetIcon(icon)
		}
		c.window.Page().LoadHTML(result.document, c.origin+"/")
	}()
}

func permissionLabel(origin string, req mygo.PermissionRequest) (string, bool) {
	if !sameOrigin(origin, req.Origin) || len(req.Permissions) == 0 {
		return "", false
	}
	labels := make([]string, 0, len(req.Permissions))
	for _, permission := range req.Permissions {
		switch permission {
		case mygo.PermissionCamera:
			labels = append(labels, "使用摄像头")
		case mygo.PermissionMicrophone:
			labels = append(labels, "使用麦克风")
		case mygo.PermissionGeolocation:
			labels = append(labels, "获取你的位置")
		case mygo.PermissionNotifications:
			labels = append(labels, "发送通知")
		default:
			return "", false
		}
	}
	return strings.Join(labels, "、"), true
}
