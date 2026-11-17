// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webfrontend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const HostAPI = 2

const UpdatePath = "/_desktop/frontend/manifest.json"
const maxArchiveSize = 128 << 20

// UpdateRequiredError is returned before downloading an incompatible frontend.
type UpdateRequiredError struct {
	MinimumVersion string `json:"minimumVersion"`
	HostAPI        int    `json:"hostApi"`
	ServerVersion  string `json:"serverVersion"`
}

func (e *UpdateRequiredError) Error() string {
	return "服务器要求更新客户端：" + e.MinimumVersion
}

type Release struct {
	Revision string `json:"revision"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	HostAPI  int    `json:"hostApi"`
	Archive  string `json:"archive"`
}

type updateState struct {
	Staged   string `json:"staged"`
	Current  string `json:"current"`
	Previous string `json:"previous"`
	Pending  bool   `json:"pending"`
	Rejected string `json:"rejected"`
}

// Updates retains immutable archives so an interrupted download cannot replace
// the last working frontend. A version is committed only after the UI boots.
type Updates struct {
	mu                         sync.Mutex
	checkMu                    sync.Mutex
	origin, directory          string
	version                    string
	state                      updateState
	current, previous, bundled *Bundle
}

func OpenUpdates(origin, directory string, version ...string) (*Updates, error) {
	u, err := newUpdates(origin, directory, nil)
	if err == nil && len(version) > 0 && version[0] != "" {
		u.version = version[0]
	}
	return u, err
}

func newUpdates(origin, directory string, bundled *Bundle) (*Updates, error) {
	key := sha256.Sum256([]byte(origin))
	u := &Updates{origin: origin, directory: filepath.Join(directory, hex.EncodeToString(key[:8])), version: "0.1.0", bundled: bundled, current: bundled}
	if err := os.MkdirAll(u.directory, 0700); err != nil {
		return nil, err
	}
	if data, err := os.ReadFile(filepath.Join(u.directory, "state.json")); err == nil {
		_ = json.Unmarshal(data, &u.state)
	}
	if u.state.Pending {
		u.state.Rejected = u.state.Current
		u.state.Current = u.state.Previous
		u.state.Previous = ""
		u.state.Pending = false
	}
	if b := u.read(u.state.Current); b != nil {
		u.current = b
	} else {
		u.state.Current = ""
	}
	u.previous = u.read(u.state.Previous)
	if err := u.save(); err != nil {
		return nil, err
	}
	if u.state.Staged != "" && u.state.Staged != u.state.Rejected {
		_ = u.Activate(u.state.Staged)
	}
	return u, nil
}

func (u *Updates) Stage(revision string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !validRevision(revision) || revision == u.state.Rejected {
		return errors.New("invalid staged release")
	}
	u.state.Staged = revision
	return u.save()
}

func validRevision(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == hex.EncodeToString(decoded)
}

func (u *Updates) read(revision string) *Bundle {
	if !validRevision(revision) {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(u.directory, revision+".zip"))
	if err != nil {
		return nil
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != revision {
		return nil
	}
	b, err := FromArchive(data, u.origin)
	if err != nil {
		return nil
	}
	b.document = strings.Replace(b.document, "<html ", "<html data-camp-frontend=\""+revision+"\" ", 1)
	return b
}

func (u *Updates) save() error {
	data, err := json.Marshal(u.state)
	if err != nil {
		return err
	}
	temporary := filepath.Join(u.directory, "state.json.tmp")
	if err = os.WriteFile(temporary, data, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(u.directory, "state.json"))
}

func (u *Updates) Current() (*Bundle, string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.current, u.state.Current
}

func (u *Updates) Pending(revision string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state.Pending && u.state.Current == revision
}

func (u *Updates) Confirm(revision string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if revision != u.state.Current || !u.state.Pending {
		return nil
	}
	u.state.Pending = false
	return u.save()
}

func (u *Updates) Rollback(revision string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if revision != u.state.Current || !u.state.Pending {
		return nil
	}
	u.state.Rejected = revision
	u.state.Current, u.state.Previous, u.state.Pending = u.state.Previous, "", false
	u.current = u.previous
	if u.current == nil {
		u.current = u.bundled
	}
	u.previous = nil
	return u.save()
}

func (u *Updates) get(ctx context.Context, path string, limit int64) ([]byte, error) {
	origin, err := url.Parse(u.origin)
	if err != nil || (origin.Scheme != "https" && origin.Hostname() != "localhost" && origin.Hostname() != "127.0.0.1") {
		return nil, errors.New("desktop updates require HTTPS")
	}
	client := &http.Client{CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		if IsMaintenanceURL(u.origin, req.URL.String()) {
			return ErrMaintenance
		}
		if req.URL.Scheme != origin.Scheme || req.URL.Host != origin.Host {
			return errors.New("cross-origin desktop update")
		}
		return nil
	}}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.origin+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("X-Camp-Desktop-Version", u.version)
	req.Header.Set("X-Camp-Host-Api", strconv.Itoa(HostAPI))
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.Header.Get("X-Camp-Page") == "maintenance" {
		return nil, ErrMaintenance
	}
	if response.StatusCode == http.StatusUpgradeRequired {
		var required UpdateRequiredError
		if err := json.NewDecoder(io.LimitReader(response.Body, 16384)).Decode(&required); err != nil {
			return nil, fmt.Errorf("invalid compatibility response: %w", err)
		}
		return nil, &required
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("desktop update HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if int64(len(data)) > limit {
		return nil, errors.New("desktop update too large")
	}
	return data, err
}

// CheckCompatibility validates the server handshake without downloading or changing
// the installed frontend. Authentication windows can open as soon as it succeeds.
func (u *Updates) CheckCompatibility(ctx context.Context) error {
	_, err := u.AvailableRevision(ctx)
	return err
}

// AvailableRevision reads the compatible release without downloading its archive.
func (u *Updates) AvailableRevision(ctx context.Context) (string, error) {
	release, err := u.release(ctx)
	if err != nil {
		return "", err
	}
	return release.Revision, nil
}

func (u *Updates) release(ctx context.Context) (Release, error) {
	var release Release
	data, err := u.get(ctx, UpdatePath, 16384)
	if err != nil {
		return release, err
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return release, err
	}
	if release.HostAPI != HostAPI {
		return release, &UpdateRequiredError{HostAPI: release.HostAPI}
	}
	if !validRevision(release.Revision) || release.SHA256 != release.Revision || release.Size <= 0 || release.Size > maxArchiveSize || release.Archive != "/_desktop/frontend/"+release.Revision+".zip" {
		return release, errors.New("incompatible desktop release")
	}
	return release, nil
}

// Check downloads a validated archive, leaving current windows on their pinned
// version. Activate is called once the main window can safely reload.
func (u *Updates) Check(ctx context.Context) (string, error) {
	u.checkMu.Lock()
	defer u.checkMu.Unlock()
	release, err := u.release(ctx)
	if err != nil {
		return "", err
	}
	u.mu.Lock()
	unchanged := (u.current != nil && release.Revision == u.current.revision) || release.Revision == u.state.Rejected
	u.mu.Unlock()
	if unchanged {
		return "", nil
	}
	if u.read(release.Revision) != nil {
		return release.Revision, nil
	}
	data, err := u.get(ctx, release.Archive, release.Size)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != release.Size || hex.EncodeToString(sum[:]) != release.Revision {
		return "", errors.New("desktop archive integrity check failed")
	}
	if _, err := FromArchive(data, u.origin); err != nil {
		return "", err
	}
	filename := filepath.Join(u.directory, release.Revision+".zip")
	if err := os.WriteFile(filename+".tmp", data, 0600); err != nil {
		return "", err
	}
	if err := os.Rename(filename+".tmp", filename); err != nil {
		return "", err
	}
	return release.Revision, nil
}

func (u *Updates) Activate(revision string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if revision == u.state.Current {
		return nil
	}
	b := u.read(revision)
	if b == nil || revision == u.state.Rejected {
		return errors.New("desktop release unavailable")
	}
	old := u.state
	u.state.Previous, u.state.Current, u.state.Pending = u.state.Current, revision, true
	u.state.Staged = ""
	if err := u.save(); err != nil {
		u.state = old
		return err
	}
	u.previous, u.current = u.current, b
	return nil
}
