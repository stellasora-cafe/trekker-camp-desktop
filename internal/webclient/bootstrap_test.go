// SPDX-FileCopyrightText: stellasora-cafe contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"
)

func TestStartupReadsEncryptedCurrentAccountWithoutNetwork(t *testing.T) {
	v, _ := newCredentialCipher("https://camp.test", bytes.Repeat([]byte{3}, 32))
	preload, _ := json.Marshal(v.preload())
	probe, _ := json.Marshal(startupSessionScript)
	script := `const vm=require('node:vm'),assert=require('node:assert/strict');
 for(const [stored,want] of [[null,'0'],['{"id":"alice","token":"local-secret"}','1'],['{"id":"bob","token":"other-secret"}','1'],['{}','0'],['invalid','0'],['{"id":"alice"}','0']]){
 class Storage{constructor(){this.data=new Map()}getItem(k){return this.data.get(k)??null}setItem(k,v){this.data.set(k,String(v))}removeItem(k){this.data.delete(k)}}
 const storage=new Storage();
 if(stored!==null)storage.setItem('account','camp-sealed-v1:'+Buffer.from(stored).toString('base64'));
 class XHR{open(method,path,async){assert.equal(method,'POST');assert.equal(path,'/__camp-desktop/credentials');assert.equal(async,false)}setRequestHeader(){}send(body){const r=JSON.parse(body);assert.equal(r.action,'open');this.status=200;this.responseText=JSON.stringify(Buffer.from(r.value.slice(15),'base64').toString())}}
 const ctx=vm.createContext({Storage,localStorage:storage,location:{origin:'https://camp.test'},XMLHttpRequest:XHR,fetch:()=>{throw Error('startup performed a network fetch')}});ctx.window=ctx;
 vm.runInContext(` + string(preload) + `,ctx);vm.runInContext(` + string(probe) + `,ctx);
 assert.equal(ctx.location.href,'camp-desktop://startup-session?authenticated='+want);
 assert.ok(!ctx.location.href.includes('secret'));
 storage.removeItem('account');vm.runInContext(` + string(probe) + `,ctx);
 assert.equal(ctx.location.href,'camp-desktop://startup-session?authenticated=0');
 }
 `
	if out, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
