// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"os/exec"
	"strings"
	"testing"
)

func TestFrontendWindowOnlyOffersNewOnlineRevision(t *testing.T) {
	for _, initial := range []string{"first", ""} {
		view := &frontendWindow{onlineRevision: initial}
		view.online.Store(true)
		if view.stage("first") || view.pending != "" {
			t.Fatal("initial offline cache download must not trigger a refresh")
		}
		if !view.stage("next") || view.pending != "next" {
			t.Fatal("a later server revision must still offer refresh")
		}
		if view.stage("first") || view.pending != "" {
			t.Fatal("a server rollback to the running revision must clear the update")
		}
	}
	cached := &frontendWindow{}
	if !cached.stage("next") || cached.pending != "next" {
		t.Fatal("a restored cached window should offer a newer release")
	}
}

func TestFrontendUpdateManualRefreshProtectsEditing(t *testing.T) {
	cmd := exec.Command("node", "-e", `
const assert=require('node:assert/strict'),vm=require('node:vm');
const script=require('node:fs').readFileSync(0,'utf8');
let time=0,editing=false,ready=true;const events=new Map();
const p=vm.createContext({Date:{now:()=>time},location:{origin:'https://camp.example'},
document:{documentElement:{dataset:{campConnection:'online'}},getElementById:()=>({childElementCount:ready?1:0}),querySelector:()=>editing?{}:null},
addEventListener:(n,fn)=>events.set(n,fn)});p.window=p;p.top=p;vm.runInContext(script,p);
assert.equal(p.__campFrontendCanReload(),true);
editing=true;assert.equal(p.__campFrontendCanReload(),true);editing=false;

p.document.documentElement.dataset.campConnection='offline';assert.equal(p.__campFrontendCanReload(),true);
p.document.documentElement.dataset.campConnection='online';ready=false;assert.equal(p.__campFrontendCanReload(),false);
ready=true;assert.equal(p.__campFrontendCanReload(),true);
events.get('camp-stream-connected')();assert.equal(p.location.href,'camp-desktop://frontend-check');
p.location.href='';events.get('online')();assert.equal(p.location.href,'camp-desktop://frontend-check');
`)
	cmd.Stdin = strings.NewReader(frontendUpdatePreload("https://camp.example"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
}
