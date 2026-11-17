// SPDX-FileCopyrightText: stellasora-cafe contributors
// SPDX-License-Identifier: Apache-2.0

package webassets

import (
	"os"
	"path/filepath"
)

// Installed fonts are served in place and never overwritten by network caching.
func (b *Bundle) findInstalledFonts(roots []string) {
	b.installed = make(map[string]string)
	for _, face := range faces {
		name := "MiSans-" + face.name + ".woff2"
		for _, root := range roots {
			if root == "" {
				continue
			}
			file := filepath.Join(root, "fonts", "misans-v1", name)
			info, err := os.Stat(file)
			if err != nil || !info.Mode().IsRegular() || info.Size() <= 4 || info.Size() > 12<<20 {
				continue
			}
			handle, err := os.Open(file)
			if err != nil {
				continue
			}
			var magic [4]byte
			n, readErr := handle.Read(magic[:])
			handle.Close()
			if readErr != nil || n != 4 || string(magic[:]) != "wOF2" {
				continue
			}
			b.installed["private/fonts/"+name] = file
			break
		}
	}
}

func (b *Bundle) installedFont(spec resourceSpec) (cachedResource, bool) {
	file := b.installed[spec.path]
	if file == "" {
		return cachedResource{}, false
	}
	info, err := os.Stat(file)
	if err != nil {
		return cachedResource{}, false
	}
	return cachedResource{Remote: spec.remote, Bytes: info.Size(), MIME: spec.mime}, true
}
