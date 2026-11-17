// SPDX-FileCopyrightText: stellasora-cafe contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/egoist/mygo"
)

const credentialPath = "/__camp-desktop/credentials"
const credentialPrefix = "camp-sealed-v1:"

type credentialCipher struct {
	aead              cipher.AEAD
	origin, challenge string
}

func webviewCredentialName(origin, profile string) string {
	identity := sha256.Sum256([]byte(origin + "\x00" + profile))
	return "webview-" + hex.EncodeToString(identity[:])
}

var webviewCredentials = mygo.Secrets.Service("cafe.stellasora.camp.credentials")

func openCredentialCipher(origin, profile string) (*credentialCipher, error) {
	name := webviewCredentialName(origin, profile)
	store := webviewCredentials
	key, err := store.Get(name)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		if err = store.Set(name, key); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return newCredentialCipher(origin, key)
}

func newCredentialCipher(origin string, key []byte) (*credentialCipher, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		return nil, err
	}
	return &credentialCipher{aead: aead, origin: origin, challenge: hex.EncodeToString(challenge)}, nil
}

func (v *credentialCipher) seal(value string) (string, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	data := v.aead.Seal(nonce, nonce, []byte(value), []byte(v.origin))
	return credentialPrefix + base64.StdEncoding.EncodeToString(data), nil
}

func (v *credentialCipher) open(value string) (string, error) {
	if !strings.HasPrefix(value, credentialPrefix) {
		return "", errors.New("invalid encrypted value")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, credentialPrefix))
	if err != nil || len(data) < v.aead.NonceSize() {
		return "", errors.New("invalid encrypted value")
	}
	plain, err := v.aead.Open(nil, data[:v.aead.NonceSize()], data[v.aead.NonceSize():], []byte(v.origin))
	return string(plain), err
}

func (v *credentialCipher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost || r.Header.Get("X-Camp-Credential-Bridge") != v.challenge || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != v.origin) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 4<<20+1))
	if err != nil || len(data) > 4<<20 {
		http.Error(w, "invalid input", 400)
		return
	}
	var request struct {
		Action string `json:"action"`
		Value  string `json:"value"`
	}
	if json.Unmarshal(data, &request) != nil {
		http.Error(w, "invalid input", 400)
		return
	}
	var value string
	switch request.Action {
	case "seal":
		value, err = v.seal(request.Value)
	case "open":
		value, err = v.open(request.Value)
	default:
		err = errors.New("invalid action")
	}
	if err != nil {
		http.Error(w, "credential protection failed", 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func (v *credentialCipher) preload() string {
	if v == nil {
		return ""
	}
	origin, _ := json.Marshal(v.origin)
	challenge, _ := json.Marshal(v.challenge)
	return `;(()=>{
if(location.origin!==` + string(origin) + `)return;
const XHR=window.XMLHttpRequest;
const request=(action,value)=>{
 const xhr=new XHR();xhr.open('POST','` + credentialPath + `',false);
 xhr.setRequestHeader('Content-Type','application/json');
 xhr.setRequestHeader('X-Camp-Credential-Bridge',` + string(challenge) + `);
 xhr.send(JSON.stringify({action,value}));
 if(xhr.status!==200)throw new Error('无法保护本地凭证，请重新打开客户端。');
 return JSON.parse(xhr.responseText);
};
const prefix='` + credentialPrefix + `';
const codec=Object.freeze({seal:value=>request('seal',value),open:value=>request('open',value),isSealed:value=>typeof value==='string'&&value.startsWith(prefix)});
Object.defineProperty(window,'__campCredentialStorage',{value:codec,configurable:false,writable:false});
const get=Storage.prototype.getItem,set=Storage.prototype.setItem;
const sensitive=key=>key==='account'||key.startsWith('pizzax::')||key.startsWith('idbfallback::pizzax::');
Storage.prototype.getItem=function(key){
 key=String(key);const value=get.call(this,key);
 if(this!==localStorage||!sensitive(key)||value===null)return value;
 if(codec.isSealed(value))return codec.open(value);
 set.call(this,key,codec.seal(value));return value;
};
Storage.prototype.setItem=function(key,value){
 key=String(key);value=String(value);
 return set.call(this,key,this===localStorage&&sensitive(key)?codec.seal(value):value);
};
// Migrate the current login before any page script observes it.
if(get.call(localStorage,'account')!==null)localStorage.getItem('account');
})();`
}
