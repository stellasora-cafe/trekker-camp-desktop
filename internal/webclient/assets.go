// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webassets"
)

func (c *client) assetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bundle := c.assets.Load()
		if bundle == nil {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/desktop.css" {
			bundle.Handler().ServeHTTP(w, r)
			return
		}
		w.Header().Del("Access-Control-Allow-Origin")
		w.Header().Del("Access-Control-Allow-Credentials")
		w.Header().Set("Vary", "Origin")
		refresh := r.URL.Query().Get("v")
		_, refreshErr := strconv.ParseUint(refresh, 10, 64)
		if (r.URL.Host != "localhost" && r.URL.Host != "camp-assets.localhost") || (r.URL.RawQuery != "" && (len(r.URL.Query()) != 1 || refreshErr != nil)) || r.URL.RawPath != "" {
			http.NotFound(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if origin != c.origin {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", c.origin)
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		style := bundle.Style()
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Length", strconv.Itoa(len(style)))
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(style))
		}
	})
}

func hostPreload(origin string, safeMode *bool) string {
	config, _ := json.Marshal(struct {
		Origin   string `json:"origin"`
		Style    string `json:"style"`
		SafeMode *bool  `json:"safeMode"`
	}{origin, webassets.AssetBaseURL + "desktop.css", safeMode})
	return fmt.Sprintf(`;(()=>{const config=%s;
if(window!==top||location.origin!==config.origin)return;
if(config.safeMode!==null)localStorage.setItem('isSafeMode',String(config.safeMode));
const install=()=>{if(document.getElementById('camp-local-assets'))return;
const link=document.createElement('link');link.id='camp-local-assets';link.rel='stylesheet';link.href=config.style;link.crossOrigin='anonymous';document.head.append(link);};
if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',install,{once:true});else install();})();`, config)
}
