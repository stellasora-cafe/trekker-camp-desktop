// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const maxHTMLBytes = 4 << 20

func fetchHTML(ctx context.Context, origin string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/html")
	client := &http.Client{
		Timeout:       20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("连接营地失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("营地暂时无法连接（HTTP %d）", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxHTMLBytes+1))
	if err != nil {
		return "", fmt.Errorf("读取营地页面失败：%w", err)
	}
	if len(body) > maxHTMLBytes {
		return "", fmt.Errorf("营地页面超过 4 MiB")
	}
	return string(body), nil
}

type navigationAction uint8

const (
	blockNavigation navigationAction = iota
	navigateHere
	openExternal
	retryConnection
)

func navigation(origin, raw string, userInitiated bool) navigationAction {
	if raw == "camp-desktop://retry" {
		return retryConnection
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return blockNavigation
	}
	if sameOrigin(origin, raw) {
		return navigateHere
	}
	if userInitiated && ((u.Scheme == "http" || u.Scheme == "https") && u.Host != "" || u.Scheme == "mailto" && u.Opaque != "") {
		return openExternal
	}
	return blockNavigation
}

func sameOrigin(origin, raw string) bool {
	a, err := url.Parse(origin)
	if err != nil {
		return false
	}
	b, err := url.Parse(raw)
	if err != nil || b.User != nil || b.Host == "" || (b.Scheme != "https" && b.Scheme != "http") {
		return false
	}
	port := func(u *url.URL) string {
		if p := u.Port(); p != "" {
			return p
		}
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	return a.Scheme == b.Scheme && strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

// A failed attempt can be retried; a running or closed window cannot start
// another attempt. The single atomic state also closes a racing retry.
type loadGate struct{ state atomic.Uint32 }

func (g *loadGate) begin() bool { return g.state.CompareAndSwap(0, 1) }
func (g *loadGate) finish()     { g.state.CompareAndSwap(1, 0) }
func (g *loadGate) close()      { g.state.Store(2) }

func connectionDocument(err error) string {
	message := "正在连接旅人营地…"
	detail := "很快就好。"
	button := ""
	if err != nil {
		message = "暂时无法连接营地"
		detail = html.EscapeString(err.Error())
		button = `<a href="camp-desktop://retry">重新连接</a>`
	}
	return `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>旅人营地</title><style>
:root{color-scheme:light dark;font-family:MiSans,"Microsoft YaHei",sans-serif;background:#edf2f8;color:#4a547b}body{margin:0;min-height:100vh;display:grid;place-items:center}main{text-align:center;padding:32px;max-width:560px}h1{font-size:24px;font-weight:600}p{font-size:14px;line-height:1.8;overflow-wrap:anywhere;opacity:.8}a{display:inline-block;margin-top:18px;padding:12px 32px;border-radius:24px;color:white;background:#00a9b3;text-decoration:none}a:focus-visible{outline:3px solid #5364a8;outline-offset:4px}@media(prefers-color-scheme:dark){:root{background:#20212b;color:#dedfeb}}</style><main><h1>` + message + `</h1><p>` + detail + `</p>` + button + `</main></html>`
}
