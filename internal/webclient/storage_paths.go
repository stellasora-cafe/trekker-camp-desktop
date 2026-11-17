// SPDX-FileCopyrightText: stellasora-cafe contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// Application resources are writable in the per-user installation. Browser
// profiles, private media and credentials retain their independent user-data path.
func applicationDirectory(profile string) (string, error) {
	if mygo.IsDev() || mygo.App.Version() == "" {
		return filepath.Join(profile, "application"), nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(executable), nil
}

func hostCacheVersion() string {
	version := mygo.App.Version()
	if version == "" {
		return "development"
	}
	return version
}

func siteResourceDirectory(root, origin string) string {
	key := sha256.Sum256([]byte(origin))
	return filepath.Join(root, "resource-cache", hex.EncodeToString(key[:8]))
}

// Copy once, preserving the old profile and any already-installed cache. The
// marker is written last so an interrupted copy can safely resume next launch.
func migrateApplicationCache(profile, root, origin string) error {
	resources := siteResourceDirectory(root, origin)
	if err := os.MkdirAll(resources, 0700); err != nil {
		return err
	}
	marker := filepath.Join(resources, ".profile-cache-imported")
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	for _, pair := range [][2]string{
		{filepath.Join(profile, "resources"), resources},
		{filepath.Join(profile, "fonts"), filepath.Join(resources, "fonts")},
	} {
		if err := copyMissingCache(pair[0], pair[1]); err != nil {
			return err
		}
	}
	return os.WriteFile(marker, []byte("1\n"), 0600)
}

func copyMissingCache(source, destination string) error {
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("cache contains a symbolic link: %s", relative)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		if _, err := os.Stat(target); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		temporary, err := os.CreateTemp(filepath.Dir(target), ".import-*")
		if err != nil {
			return err
		}
		defer os.Remove(temporary.Name())
		if _, err := temporary.Write(data); err != nil {
			temporary.Close()
			return err
		}
		if err := temporary.Close(); err != nil {
			return err
		}
		return os.Rename(temporary.Name(), target)
	})
}
