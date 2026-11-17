// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

// Package campweb adapts the server's web frontend to desktop device sessions.
package campweb

import (
	_ "embed"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webfrontend"
)

//go:embed preload.js
var transportScript string

// Preload adapts transport at the camp origin using the WebView's own account.
func Preload(origin, platform string, versions ...string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return "", errors.New("营地地址必须是完整的网站源地址")
	}
	local := strings.EqualFold(u.Hostname(), "localhost")
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		local = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", errors.New("营地地址必须使用 HTTPS；本机开发可使用 HTTP")
	}
	u.Path = ""
	version := "0.1.0"
	if len(versions) > 0 && versions[0] != "" {
		version = versions[0]
	}
	seed, err := json.Marshal(struct {
		Origin   string `json:"origin"`
		Platform string `json:"platform"`
		Version  string `json:"version"`
		HostAPI  int    `json:"hostApi"`
	}{u.String(), platform, version, webfrontend.HostAPI})
	if err != nil {
		return "", errors.New("客户端账号初始化失败")
	}
	return "(()=>{const seed=" + string(seed) + ";\n" + transportScript + "\n})();", nil
}
