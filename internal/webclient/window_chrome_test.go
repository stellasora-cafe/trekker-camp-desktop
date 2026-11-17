// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

func TestWindowChromeKeepsNativeResizingAndUntrustedPage(t *testing.T) {
	for _, platform := range []string{"windows", "darwin", "linux"} {
		t.Run(platform, func(t *testing.T) {
			options := mygo.WindowOptions{Width: 440, Height: 480, MinWidth: 400, MinHeight: 420, Page: mygo.PageOptions{Untrusted: true}}
			applyWindowChrome(&options, platform)
			if !options.Page.Untrusted || options.DisableResize || options.Frameless || options.DisableShadow || options.MinWidth != 400 {
				t.Fatal("custom chrome changed trust, resizing or shadow policy")
			}
			if platform == "windows" {
				if options.TitleBarStyle != mygo.TitleBarHidden || !options.HideWindowButtons || options.TitleBarHeight != 50 {
					t.Fatal("Windows must retain its hidden-caption frame with custom controls")
				}
			} else if options.HideWindowButtons || options.TitleBarStyle != "" || windowChromePreload("https://camp.example", platform) != "" {
				t.Fatal("non-Windows caption changed")
			}
		})
	}
}

func TestWindowControlOnlyAcceptsKnownSameOriginActions(t *testing.T) {
	const origin = "https://camp.example"
	for _, action := range []string{"minimize", "maximize", "close", "drag"} {
		got, handled := windowControlAction(origin, origin+"/desktop-auth?mode=signin", "camp-desktop://window-control?action="+action)
		if !handled || got != action {
			t.Fatalf("valid control %q rejected", action)
		}
	}
	for _, test := range []struct{ page, target string }{
		{"https://other.example", "camp-desktop://window-control?action=close"},
		{"http://camp.example", "camp-desktop://window-control?action=close"},
		{"https://camp.example:444", "camp-desktop://window-control?action=close"},
		{"about:blank", "camp-desktop://window-control?action=close"},
		{origin, "camp-desktop://window-control?action=eval"},
		{origin, "camp-desktop://window-control?action=close&action=drag"},
		{origin, "camp-desktop://window-control?action=close&target=another-window"},
		{origin, "camp-desktop://window-control?%zz=close"},
		{origin, "camp-desktop://window-control/path?action=close"},
		{origin, "camp-desktop://user@window-control?action=close"},
		{origin, "camp-desktop://window-control?action=close#extra"},
	} {
		if action, _ := windowControlAction(origin, test.page, test.target); action != "" {
			t.Errorf("unsafe window action admitted: %s from %s", test.target, test.page)
		}
	}
	for _, target := range []string{origin + "/", "camp-desktop://window-state?guest=0&offline=0", "camp-desktop://retry"} {
		if _, handled := windowControlAction(origin, origin, target); handled {
			t.Errorf("intercepted an unrelated navigation: %s", target)
		}
	}
}

func TestWindowChromeStatePreloadAndEvents(t *testing.T) {
	scripts, err := json.Marshal(map[string]string{
		"preload": windowChromePreload("https://camp.example", "windows"),
		"normal":  windowChromeStateScript("https://camp.example", windowChromeState{Active: true}),
		"max":     windowChromeStateScript("https://camp.example", windowChromeState{Maximized: true, Active: true}),
		"full":    windowChromeStateScript("https://camp.example", windowChromeState{Fullscreen: true}),
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "-e", `
const assert=require('node:assert/strict'),vm=require('node:vm');
const scripts=JSON.parse(require('node:fs').readFileSync(0,'utf8'));
function page(origin='https://camp.example',frame=false){
const events=[];let observed;
const p=vm.createContext({location:{origin},document:{documentElement:{dataset:{}}},Event:class{constructor(type){this.type=type}},
dispatchEvent:e=>events.push(e.type),MutationObserver:class{constructor(fn){observed=fn}observe(){}disconnect(){}}});
p.window=p;p.top=frame?{}:p;
return{p,events,run:k=>vm.runInContext(scripts[k],p),attach:()=>{p.document.documentElement={dataset:{}};observed()}};}
let s=page();s.run('preload');assert.equal(s.p.document.documentElement.dataset.campWindowChrome,'true');
s.run('normal');assert.equal(s.p.document.documentElement.dataset.campWindowMaximized,'false');assert.equal(s.p.document.documentElement.dataset.campWindowActive,'true');
s.run('max');assert.equal(s.p.document.documentElement.dataset.campWindowMaximized,'true');
s.run('full');assert.equal(s.p.document.documentElement.dataset.campWindowMaximized,'false');assert.equal(s.p.document.documentElement.dataset.campWindowFullscreen,'true');assert.equal(s.p.document.documentElement.dataset.campWindowActive,'false');
assert.deepEqual(s.events,Array(3).fill('camp-window-state-change'));
s=page();s.p.document.documentElement=null;s.run('preload');s.attach();assert.equal(s.p.document.documentElement.dataset.campWindowChrome,'true');
for(const fixture of [page('https://another.example'),page('https://camp.example',true)]){
fixture.run('preload');fixture.run('max');assert.equal(Object.keys(fixture.p.document.documentElement.dataset).length,0);assert.equal(fixture.events.length,0);}
`)
	cmd.Stdin = strings.NewReader(string(scripts))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("window chrome scripts: %v\n%s", err, output)
	}
}
