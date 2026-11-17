package mygo

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Camp's installer-owned resources are replaced, while its frontend/resource
// caches are independent top-level entries and survive package upgrades.
func TestCampPackageUpdatePreservesLocalStores(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Windows/Linux directory installation")
	}
	srv, key, _, _ := updateServer(t, "1.2.0", "new executable", "")
	up, err := checkUpdate(context.Background(), feedSource(srv.URL+"/update.json"), key, "1.1.0", false)
	if err != nil {
		t.Fatal(err)
	}
	target, main := installedApp(t)
	for _, directory := range []string{"frontend-cache", "resource-cache", "user-data"} {
		if err := os.MkdirAll(filepath.Join(target, directory), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, directory, "keep"), []byte("preserved"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(target, "resource"), []byte("old resource"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := installUpdate(context.Background(), httpGet, up.manifest, key, up.current, target, nil); err != nil {
		t.Fatal(err)
	}
	for path, expected := range map[string]string{
		main:                              "new executable",
		filepath.Join(target, "resource"): "unchanged",
		filepath.Join(target, "frontend-cache/keep"): "preserved",
		filepath.Join(target, "resource-cache/keep"): "preserved",
		filepath.Join(target, "user-data/keep"):      "preserved",
	} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != expected {
			t.Errorf("%s = %q, %v", path, data, err)
		}
	}
}
