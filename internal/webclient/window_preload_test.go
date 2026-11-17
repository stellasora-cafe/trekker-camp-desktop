// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestWindowSelectionAfterSessionRestore(t *testing.T) {
	data, err := json.Marshal(map[string]string{
		"main":    windowPreload("https://camp.example", false, false),
		"tool":    windowPreload("https://camp.example", true, false),
		"offline": windowPreload("https://camp.example", false, true),
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "-e", `
const assert=require('node:assert/strict'),vm=require('node:vm');
const scripts=JSON.parse(require('node:fs').readFileSync(0,'utf8'));
function page(origin='https://camp.example',frame=false){
const values=new Map(),session=new Map(),events=new Map();let observer,ready=true;
const storage=m=>({getItem:k=>m.get(k)||null,setItem:(k,v)=>m.set(k,v),clear:()=>m.clear()});
const p=vm.createContext({location:{origin},localStorage:storage(values),sessionStorage:storage(session),
document:{readyState:'loading',documentElement:{dataset:{}},body:{},addEventListener:(n,fn)=>events.set(n,fn),getElementById:()=>({childElementCount:ready?1:0})},
addEventListener:(n,fn)=>events.set(n,fn),
MutationObserver:class {constructor(fn){observer=fn}observe(){}disconnect(){}},requestAnimationFrame:fn=>fn()});
p.window=p;p.top=frame?{}:p;
return {p,values,events,run:k=>vm.runInContext(scripts[k],p),install:()=>events.get('DOMContentLoaded')(),
network:state=>{p.document.documentElement.dataset.campConnection=state;events.get('camp-connection-change')?.()},
ready:value=>{ready=value;if(value)observer?.()}};}
let s;
const login=s=>s.values.set('account',JSON.stringify({id:'test-user',token:'cs_test-only'}));
s=page();s.run('main');s.install();assert.equal(s.p.location.href,undefined,'do not open native welcome before connectivity is known');
s.network('offline');assert.equal(s.p.location.href,'camp-desktop://window-state?guest=1&offline=1');
s.network('online');assert.equal(s.p.location.href,'camp-desktop://window-state?guest=1&offline=0');
s=page();login(s);s.run('main');s.network('online');s.install();assert.equal(s.p.location.href,'camp-desktop://window-state?guest=0&offline=0');
s.network('offline');assert.equal(s.p.location.href,'camp-desktop://window-state?guest=0&offline=1');
s.values.clear();s.events.get('storage')({key:'account'});assert.equal(s.p.location.href,'camp-desktop://window-state?guest=1&offline=1','offline logout keeps maintenance');
s.network('online');assert.equal(s.p.location.href,'camp-desktop://window-state?guest=1&offline=0','reconnected guest returns to native welcome');
s=page();login(s);s.run('main');s.values.clear();s.network('online');s.install();assert.equal(s.p.location.href,'camp-desktop://window-state?guest=1&offline=0','restore can revoke a saved account');
s=page();s.run('tool');assert.equal(s.p.document.documentElement.dataset.campDesktopWindow,'tool');assert.equal(s.p.document.documentElement.dataset.campDesktopGuest,undefined);
s.install();s.network('offline');assert.equal(s.p.location.href,'camp-desktop://window-state?guest=1&offline=1');
s=page();s.ready(false);s.run('offline');s.install();assert.equal(s.p.location.href,undefined,'wait for the maintenance view before showing the window');
s.ready(true);assert.equal(s.p.location.href,'camp-desktop://window-state?guest=1&offline=1');
s=page('https://other.example');s.run('main');assert.equal(s.events.size,0,'ignore cross-origin pages');
s=page('https://camp.example',true);s.run('main');assert.equal(s.events.size,0,'ignore subframes');
`)
	cmd.Stdin = strings.NewReader(string(data))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("window/account preload: %v\n%s", err, output)
	}
}
