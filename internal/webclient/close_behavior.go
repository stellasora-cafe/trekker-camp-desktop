// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type closeBehavior string

const (
	closeAsk  closeBehavior = "ask"
	closeTray closeBehavior = "tray"
	closeQuit closeBehavior = "quit"
)

const closeBehaviorPath = "/__camp-desktop/close-behavior"

type closeBehaviorStore struct {
	mu       sync.Mutex
	filename string
}

func (s *closeBehaviorStore) get() closeBehavior {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := os.ReadFile(s.filename)
	switch value := closeBehavior(strings.TrimSpace(string(data))); value {
	case closeTray, closeQuit:
		return value
	default:
		return closeAsk
	}
}

func (s *closeBehaviorStore) set(value closeBehavior) error {
	switch value {
	case closeAsk, closeTray, closeQuit:
	default:
		return errors.New("invalid close behavior")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.CreateTemp(filepath.Dir(s.filename), ".close-behavior-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.WriteString(string(value) + "\n")
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), s.filename)
}

// Only the main WebView gets the host-local settings endpoint and its bridge.
func (c *client) closeBehaviorEnabled(tool bool) bool {
	return !tool && runtime.GOOS == "windows" && c.closeBehavior != nil && c.credentials != nil
}

func (c *client) closeBehaviorPreload() string {
	origin, _ := json.Marshal(c.origin)
	challenge, _ := json.Marshal(c.credentials.challenge)
	return `;(()=>{
if(window!==top||location.origin!==` + string(origin) + `)return;
const fetchLocal=window.fetch.bind(window);
const request=async(value)=>{
 const response=await fetchLocal('` + closeBehaviorPath + `',{
  method:value===undefined?'GET':'PUT',
  headers:{'Content-Type':'application/json','X-Camp-Close-Behavior':` + string(challenge) + `},
  body:value===undefined?undefined:JSON.stringify(value),redirect:'error',cache:'no-store',
 });
 if(!response.ok)throw new Error('Desktop close behavior request failed');
 return response.json();
};
Object.defineProperty(window,'__campCloseBehavior',{value:Object.freeze({get:()=>request(),set:value=>request(value)})});
})();`
}

func (c *client) serveCloseBehavior(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !sameOrigin(c.origin, r.URL.String()) || r.Header.Get("X-Camp-Close-Behavior") != c.credentials.challenge ||
		(r.Header.Get("Origin") != "" && r.Header.Get("Origin") != c.origin) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodPut {
		var value closeBehavior
		body, err := io.ReadAll(io.LimitReader(r.Body, 33))
		if err != nil || len(body) > 32 || json.Unmarshal(body, &value) != nil || (value != closeAsk && value != closeTray && value != closeQuit) {
			http.Error(w, "invalid close behavior", http.StatusBadRequest)
			return
		}
		if err := c.closeBehavior.set(value); err != nil {
			http.Error(w, "failed to save setting", http.StatusInternalServerError)
			return
		}
	} else if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(c.closeBehavior.get())
}
