// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

const aboutOrigin = "https://about.camp-desktop.invalid"

//go:embed about/*
var aboutFiles embed.FS

func aboutHandler(resources, version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; font-src 'self'; connect-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if !sameOrigin(aboutOrigin, r.URL.String()) || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}
		var data []byte
		var err error
		mime := ""
		switch r.URL.Path {
		case "/":
			data, err = aboutFiles.ReadFile("about/index.html")
			if err == nil {
				var t *template.Template
				t, err = template.New("about").Parse(string(data))
				if err == nil {
					var b bytes.Buffer
					label := version
					if label == "" {
						label = "开发版"
					}
					err = t.Execute(&b, struct {
						Version                            string
						Minimize, Maximize, Restore, Close template.HTML
					}{label, captionAssetHTML("minimize"), captionAssetHTML("maximize"), captionAssetHTML("restore"), captionAssetHTML("close")})
					data = b.Bytes()
				}
			}
			mime = "text/html; charset=utf-8"
		case "/about.css":
			data, err = aboutFiles.ReadFile("about/about.css")
			mime = "text/css; charset=utf-8"
		case "/about.js":
			data, err = aboutFiles.ReadFile("about/about.js")
			mime = "text/javascript; charset=utf-8"
		default:
			files := map[string]string{
				"/images/camp-icon.png": "about/camp-icon.png", "/images/organization.png": "about/organization.png",
				"/fonts/regular.woff2": "camp/fonts/misans-v1/MiSans-Regular.woff2", "/fonts/medium.woff2": "camp/fonts/misans-v1/MiSans-Medium.woff2",
				"/fonts/semibold.woff2": "camp/fonts/misans-v1/MiSans-Semibold.woff2", "/fonts/bold.woff2": "camp/fonts/misans-v1/MiSans-Bold.woff2",
			}
			file, ok := files[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			data, err = os.ReadFile(filepath.Join(resources, filepath.FromSlash(file)))
			if strings.HasSuffix(file, ".png") {
				mime = "image/png"
			} else {
				mime = "font/woff2"
			}
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", mime)
		if r.Method != http.MethodHead {
			_, _ = w.Write(data)
		}
	})
}

func (c *client) openAboutCamp() {
	mygo.RunOnMain(func() {
		if c.ctx.Err() != nil {
			return
		}
		if c.aboutWindow != nil && !c.aboutWindow.IsDestroyed() {
			c.aboutWindow.Show()
			c.aboutWindow.Focus()
			return
		}
		options := mygo.WindowOptions{Title: "关于营地", Width: 680, Height: 760, MinWidth: 440, MinHeight: 440, UseContentSize: true, BackgroundColor: "#edf2f8", Page: mygo.PageOptions{
			Untrusted: true, PreloadScript: windowChromePreload(aboutOrigin, runtime.GOOS),
			ResourceFilter:  func(raw string) bool { return sameOrigin(aboutOrigin, raw) },
			ResourceHandler: aboutHandler(filepath.Join(c.applicationDir, "resources"), mygo.App.Version()),
		}}
		applyWindowChrome(&options, runtime.GOOS)
		window := mygo.NewWindow(options)
		c.aboutWindow = window
		chrome := &client{ctx: c.ctx, origin: aboutOrigin, window: window}
		chrome.configureWindowChrome(window)
		page := window.Page()
		page.OnContextMenu(func(e *mygo.ContextMenuEvent) { e.PreventDefault() })
		page.OnPageTitleUpdated(func(e *mygo.TitleEvent) { e.PreventDefault() })
		page.OnWillNavigate(func(e *mygo.NavigateEvent) {
			if chrome.handleWindowControl(window, e.URL) {
				e.PreventDefault()
				return
			}
			if e.URL == aboutOrigin+"/" {
				return
			}
			e.PreventDefault()
			if !e.UserInitiated {
				return
			}
			if e.URL == "camp-desktop://about-misskey" {
				if c.frontend == nil {
					go mygo.Shell.OpenExternal(c.origin + "/about-camp/misskey")
				} else {
					go c.openTool(c.origin + "/about-camp/misskey")
				}
				return
			}
			if e.URL == "camp-desktop://font-license" {
				go mygo.Shell.OpenExternal(filepath.Join(c.applicationDir, "resources", "licenses", "MiSans-license.pdf"))
				return
			}
			if strings.HasPrefix(e.URL, "https://") {
				go mygo.Shell.OpenExternal(e.URL)
			}
		})
		page.SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
			if req.UserInitiated && strings.HasPrefix(req.URL, "https://") {
				go mygo.Shell.OpenExternal(req.URL)
			}
			return nil
		})
		window.OnClosed(func() {
			if c.aboutWindow == window {
				c.aboutWindow = nil
			}
		})
		go page.LoadURL(aboutOrigin + "/")
	})
}
