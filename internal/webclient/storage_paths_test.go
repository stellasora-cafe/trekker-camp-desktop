// SPDX-FileCopyrightText: stellasora-cafe contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateApplicationCachePreservesProfileAndInstalledFiles(t *testing.T) {
	profile, root := t.TempDir(), t.TempDir()
	for name, data := range map[string]string{"resources/index.json": "old", "frontend/test.zip": "frontend", "credentials": "private"} {
		p := filepath.Join(profile, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dest := siteResourceDirectory(root, "https://camp.test")
	if err := os.MkdirAll(dest, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "index.json"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := migrateApplicationCache(profile, root, "https://camp.test"); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{filepath.Join(dest, "index.json"): "new", filepath.Join(profile, "credentials"): "private", filepath.Join(profile, "frontend", "test.zip"): "frontend"} {
		data, err := os.ReadFile(name)
		if err != nil || string(data) != want {
			t.Fatalf("%s: %q %v", name, data, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "credentials")); !os.IsNotExist(err) {
		t.Fatal("credentials moved into installation")
	}
	if dest == siteResourceDirectory(root, "https://other.test") {
		t.Fatal("origins share assets")
	}
}
