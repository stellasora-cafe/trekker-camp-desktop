// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webassets"
)

func TestAssetStylesheetOriginAndReadiness(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	c := &client{origin: server.URL}
	handler := c.assetHandler()
	request := func(method, path, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "camp-assets://localhost"+path, nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if response := request(http.MethodGet, "/desktop.css", server.URL); response.Code != http.StatusNotFound {
		t.Fatal("unprepared assets must not be served")
	}
	bundle, err := webassets.Prepare(context.Background(), server.URL, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.assets.Store(bundle)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		response := request(method, "/desktop.css", server.URL)
		if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != server.URL || response.Header().Get("Access-Control-Allow-Credentials") != "" || response.Header().Get("Content-Type") != "text/css; charset=utf-8" {
			t.Fatalf("same-origin stylesheet rejected: %d", response.Code)
		}
	}
	if response := request(http.MethodGet, "/desktop.css", "https://outside.example"); response.Code != http.StatusForbidden || response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("external document may not read the cache")
	}
	if response := request(http.MethodPost, "/desktop.css", server.URL); response.Code != http.StatusMethodNotAllowed {
		t.Fatal("cache must be read-only")
	}
	if response := request(http.MethodGet, "/desktop.css?anything", server.URL); response.Code != http.StatusNotFound {
		t.Fatal("stylesheet URL must not accept arbitrary query parameters")
	}
	if response := request(http.MethodGet, "/desktop.css?v=1794567000000", server.URL); response.Code != http.StatusOK {
		t.Fatal("refreshed runtime assets must load without reloading the account")
	}
}

func TestHostPreload(t *testing.T) {
	on, off := true, false
	data, err := json.Marshal(map[string]string{
		"default": hostPreload("https://camp.example", nil),
		"on":      hostPreload("https://camp.example", &on),
		"off":     hostPreload("https://camp.example", &off),
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "-e", `
const assert=require('node:assert/strict'),vm=require('node:vm');
const scripts=JSON.parse(require('node:fs').readFileSync(0,'utf8'));
function run(key,options={}){
const added=[],events=[],values=new Map([['isSafeMode','existing']]);
const context=vm.createContext({location:{origin:options.origin||'https://camp.example'},localStorage:{setItem:(k,v)=>values.set(k,v)},
document:{readyState:options.loading?'loading':'complete',getElementById:()=>options.existing||added.length?{}:null,createElement:()=>({}),head:{append:x=>added.push(x)},addEventListener:(name,fn)=>events.push(fn)}});
context.window=context;context.top=options.frame?{}:context;vm.runInContext(scripts[key],context);return{added,events,values};}
let page=run('default');assert.equal(page.values.get('isSafeMode'),'existing');assert.equal(page.added.length,1);assert.equal(page.added[0].id,'camp-local-assets');assert.equal(page.added[0].href,'http://camp-assets.localhost/desktop.css');
assert.equal(run('on').values.get('isSafeMode'),'true');assert.equal(run('off').values.get('isSafeMode'),'false');
assert.equal(run('default',{existing:true}).added.length,0);
page=run('on',{loading:true});assert.equal(page.values.get('isSafeMode'),'true');assert.equal(page.added.length,0);page.events[0]();assert.equal(page.added.length,1);page.events[0]();assert.equal(page.added.length,1);
for(const options of[{origin:'null'},{origin:'https://external.example'},{frame:true}]){page=run('on',options);assert.equal(page.added.length,0);assert.equal(page.values.get('isSafeMode'),'existing');}
`)
	cmd.Stdin = strings.NewReader(string(data))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("host preload failed: %v\n%s", err, output)
	}
}
