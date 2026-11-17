// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const permissionFontAsset = "fonts/misans-v1/MiSans-Regular.ttf"

var permissionFontRegistration struct {
	sync.Mutex
	loaded bool
}

// Register the packaged TrueType face before the first native window is drawn.
// Source-only builds retain the cached/downloaded fallback below.
func registerInstalledFont(directory string) {
	permissionFontRegistration.Lock()
	defer permissionFontRegistration.Unlock()
	if permissionFontRegistration.loaded {
		return
	}
	data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(permissionFontAsset)))
	if err == nil && len(data) <= 32<<20 {
		permissionFontRegistration.loaded = ui.RegisterFont(data, "MiSans") == nil
	}
}

func (c *client) loadPermissionFont() {
	if c.permissionFontPending {
		return
	}
	c.permissionFontPending = true
	go func() {
		permissionFontRegistration.Lock()
		if !permissionFontRegistration.loaded {
			cache := filepath.Join(c.resourceDir, filepath.FromSlash(permissionFontAsset))
			permissionFontRegistration.loaded = preparePermissionFont(c.ctx, c.origin, cache, permissionFontCandidates(cache), func(data []byte) error {
				return ui.RegisterFont(data, "MiSans")
			}) == nil
		}
		permissionFontRegistration.Unlock()
		mygo.RunOnMain(func() {
			c.permissionFontPending = false
			if c.permissionWindow != nil && c.ctx.Err() == nil {
				c.permissionWindow.Invalidate()
			}
		})
	}()
}

func permissionFontCandidates(cache string) []string {
	paths := []string{cache}
	if root := os.Getenv("CAMP_ASSETS_DIR"); root != "" {
		paths = append(paths, filepath.Join(root, filepath.FromSlash(permissionFontAsset)))
	}
	// Developer checkouts already share the website's licensed runtime assets.
	if dir, err := os.Getwd(); err == nil {
		for {
			paths = append(paths,
				filepath.Join(dir, "private-assets", "public", filepath.FromSlash(permissionFontAsset)),
				filepath.Join(dir, "apps", "misskey", "packages", "frontend", "assets", "camp", filepath.FromSlash(permissionFontAsset)))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return paths
}

func preparePermissionFont(ctx context.Context, origin, cache string, candidates []string, register func([]byte) error) error {
	const limit = 32 << 20
	for _, path := range candidates {
		info, err := os.Stat(path)
		if err != nil || info.Size() > limit {
			continue
		}
		data, err := os.ReadFile(path)
		if err == nil && register(data) == nil {
			return nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/client-assets/camp/"+permissionFontAsset, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("MiSans: HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return err
	}
	if len(data) > limit {
		return fmt.Errorf("MiSans: 字体超过 32 MiB")
	}
	if err := register(data); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cache), 0700); err != nil {
		return nil // The registered font remains usable without a disk cache.
	}
	file, err := os.CreateTemp(filepath.Dir(cache), ".misans-*.ttf")
	if err != nil {
		return nil
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr == nil && closeErr == nil {
		_ = os.Rename(file.Name(), cache)
	}
	return nil
}
