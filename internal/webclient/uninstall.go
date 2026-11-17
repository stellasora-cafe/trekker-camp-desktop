// SPDX-FileCopyrightText: stellasora-cafe contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// ClearInstalledUserData removes the default Windows profile and WebView2 cache.
// The installer invokes it after the user opts in and the app has closed.
func ClearInstalledUserData(origin string) error {
	if runtime.GOOS != "windows" {
		return errors.ErrUnsupported
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(config) || !filepath.IsAbs(cache) {
		return errors.New("invalid Windows user-data paths")
	}
	profile := filepath.Join(config, "旅人营地")
	cache = filepath.Join(cache, "旅人营地")
	// WebView2 can take a moment to release its files after the host exits.
	for attempt := 0; attempt < 6; attempt++ {
		err = errors.Join(os.RemoveAll(profile), os.RemoveAll(cache))
		if err == nil {
			return webviewCredentials.Delete(webviewCredentialName(origin, profile))
		}
		if attempt < 5 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return err
}
