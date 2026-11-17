// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestNativeContextMenuDispatchOnlyForSameOriginMainDocument(t *testing.T) {
	const origin = "https://camp.example"
	for _, test := range []struct{ page, frame string }{
		{"https://other.example", ""},
		{"http://camp.example", ""},
		{"about:blank", ""},
		{origin, origin + "/embed"},
		{origin, "https://other.example/frame"},
	} {
		if script := nativeContextMenuScript(origin, test.page, test.frame); script != "" {
			t.Errorf("intercepted context menu outside the main document: %s (%s)", test.page, test.frame)
		}
	}
	script := nativeContextMenuScript(origin, origin+"/about", "")
	if script == "" {
		t.Fatal("same-origin main document must use the Camp context menu")
	}
	cmd := exec.Command("node", "-e", `
const assert=require('node:assert/strict'),vm=require('node:vm');
const script=require('node:fs').readFileSync(0,'utf8');
for(const [origin,frame,expected] of [['https://camp.example',false,1],['https://other.example',false,0],['https://camp.example',true,0]]){
const events=[];
const page=vm.createContext({location:{origin},Event:class{constructor(type){this.type=type}},dispatchEvent:e=>events.push(e.type)});
page.window=page;page.top=frame?{}:page;
vm.runInContext(script,page);
assert.equal(events.length,expected);
if(expected)assert.equal(events[0],'camp-native-context-menu');}
`)
	cmd.Stdin = strings.NewReader(script)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native context menu dispatch: %v\n%s", err, output)
	}
}

func TestContextPasteRequiresSameOriginClickAndKnownAction(t *testing.T) {
	const origin = "https://camp.example"
	const requestID = "d881c6c6-83ad-490d-a8dd-0c50a168c2bb"
	const target = "camp-desktop://context-action?action=paste&requestId=" + requestID
	for _, userInitiated := range []bool{true, false} {
		id, allowed, handled := contextPasteRequest(origin, origin+"/desktop-auth", target, userInitiated)
		if !handled || id != requestID || allowed != userInitiated {
			t.Fatalf("paste gesture policy: id=%q allowed=%t handled=%t", id, allowed, handled)
		}
	}
	for _, test := range []struct{ page, target string }{
		{"https://other.example", target},
		{"http://camp.example", target},
		{"https://camp.example:444", target},
		{"about:blank", target},
		{origin, "camp-desktop://context-action?action=read-files&requestId=123"},
		{origin, "camp-desktop://context-action?action=paste&requestId=123&action=paste"},
		{origin, "camp-desktop://context-action?action=paste&requestId=123&requestId=456"},
		{origin, "camp-desktop://context-action?action=paste&requestId=123&text=untrusted"},
		{origin, "camp-desktop://context-action?action=paste&requestId="},
		{origin, "camp-desktop://context-action?action=paste&requestId=" + strings.Repeat("a", 81)},
		{origin, "camp-desktop://context-action?action=paste&requestId=%3Cscript%3E"},
		{origin, "camp-desktop://context-action?action=paste&requestId=%zz"},
		{origin, "camp-desktop://context-action/path?action=paste&requestId=123"},
		{origin, "camp-desktop://user@context-action?action=paste&requestId=123"},
		{origin, target + "#extra"},
	} {
		id, allowed, _ := contextPasteRequest(origin, test.page, test.target, true)
		if id != "" || allowed {
			t.Errorf("unsafe clipboard request admitted: %s from %s", test.target, test.page)
		}
	}
	for _, target := range []string{origin + "/", "camp-desktop://window-control?action=close", "camp-desktop://retry"} {
		if _, _, handled := contextPasteRequest(origin, origin, target, true); handled {
			t.Errorf("intercepted unrelated navigation: %s", target)
		}
	}
}

func TestContextPasteResultRemainsTextInOriginatingPage(t *testing.T) {
	const clipboardText = "引号\" ' ` ${alert(1)} </script>\n下一行\u2028结束"
	scripts, err := json.Marshal(map[string]string{
		"success": contextPasteResultScript("https://camp.example", "paste-123", clipboardText, false),
		"failure": contextPasteResultScript("https://camp.example", "paste-456", clipboardText, true),
		"text":    clipboardText,
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "-e", `
const assert=require('node:assert/strict'),vm=require('node:vm');
const scripts=JSON.parse(require('node:fs').readFileSync(0,'utf8'));
function page(origin='https://camp.example',frame=false){
const events=[];
const p=vm.createContext({location:{origin},CustomEvent:class{constructor(type,options){this.type=type;this.detail=options.detail}},dispatchEvent:e=>events.push(e)});
p.window=p;p.top=frame?{}:p;
return {events,run:k=>vm.runInContext(scripts[k],p)};}
const fixture=page();fixture.run('success');fixture.run('failure');
assert.equal(fixture.events[0].type,'camp-context-result');
assert.equal(fixture.events[0].detail.requestId,'paste-123');
assert.equal(fixture.events[0].detail.text,scripts.text);
assert.equal(fixture.events[0].detail.error,undefined);
assert.equal(fixture.events[1].detail.requestId,'paste-456');
assert.equal(fixture.events[1].detail.error,true);
assert.equal(fixture.events[1].detail.text,'');
for(const blocked of [page('https://another.example'),page('https://camp.example',true)]){
blocked.run('success');assert.equal(blocked.events.length,0);}
`)
	cmd.Stdin = strings.NewReader(string(scripts))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clipboard response scripts: %v\n%s", err, output)
	}
}
