// SPDX-FileCopyrightText: stellasora-cafe contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os/exec"
	"testing"
)

func TestCredentialCipherAuthenticationAndOriginIsolation(t *testing.T) {
	v, err := newCredentialCipher("https://camp.test", bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	value := `{"token":"test-account-secret"}`
	sealed, err := v.seal(value)
	if err != nil || bytes.Contains([]byte(sealed), []byte("test-account-secret")) {
		t.Fatal("plaintext credential persisted")
	}
	if got, err := v.open(sealed); err != nil || got != value {
		t.Fatalf("roundtrip %v", err)
	}
	other, _ := newCredentialCipher("https://other.test", bytes.Repeat([]byte{1}, 32))
	if _, err := other.open(sealed); err == nil {
		t.Fatal("credential accepted by another origin")
	}
	for _, challenge := range []string{"", v.challenge} {
		body, _ := json.Marshal(map[string]string{"action": "seal", "value": value})
		r := httptest.NewRequest("POST", v.origin+credentialPath, bytes.NewReader(body))
		r.Header.Set("X-Camp-Credential-Bridge", challenge)
		w := httptest.NewRecorder()
		v.ServeHTTP(w, r)
		want := 200
		if challenge == "" {
			want = 403
		}
		if w.Code != want {
			t.Fatalf("status %d want %d", w.Code, want)
		}
	}
}

func TestCredentialPreloadProtectsStorageAndMigratesLegacy(t *testing.T) {
	v, _ := newCredentialCipher("https://camp.test", bytes.Repeat([]byte{2}, 32))
	script := `const vm=require('node:vm'),assert=require('node:assert/strict');
class Storage { constructor(){this.values=new Map()}getItem(k){return this.values.get(k)??null}setItem(k,v){this.values.set(k,String(v))}removeItem(k){this.values.delete(k)}clear(){this.values.clear()} }
const storage=new Storage();storage.setItem('account','{"token":"old-token"}');
class XHR {open(method,url,async){assert.equal(async,false)}setRequestHeader(){}send(body){const r=JSON.parse(body);this.status=200;this.responseText=JSON.stringify(r.action==='seal'?'camp-sealed-v1:'+Buffer.from(r.value).toString('base64'):Buffer.from(r.value.slice(15),'base64').toString())}}
const context=vm.createContext({Storage,localStorage:storage,location:{origin:'https://camp.test'},XMLHttpRequest:XHR});context.window=context;
vm.runInContext(`
	encoded, _ := json.Marshal(v.preload())
	script += string(encoded) + `,context);
assert.ok(storage.values.get('account').startsWith('camp-sealed-v1:'));
assert.equal(storage.getItem('account'),'{"token":"old-token"}');
storage.setItem('account','{"token":"new-token"}');assert.equal(storage.getItem('account'),'{"token":"new-token"}');
assert.ok(!storage.values.get('account').includes('new-token'));
storage.setItem('theme','light');assert.equal(storage.values.get('theme'),'light');
storage.removeItem('account');assert.equal(storage.getItem('account'),null);
`
	if out, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
