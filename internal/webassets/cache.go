// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webassets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (b *Bundle) fetch(ctx context.Context, client *http.Client, spec resourceSpec, previous cachedResource) (cachedResource, string, error) {
	if r, ok := b.installedFont(spec); ok {
		return r, "installed", nil
	}
	usable := false
	if previous.Bytes > 0 {
		info, err := os.Stat(b.filename(spec.path))
		usable = err == nil && info.Mode().IsRegular() && info.Size() == previous.Bytes
	}
	fallback := func(err error) (cachedResource, string, error) {
		if usable {
			return previous, "cache", err
		}
		return cachedResource{}, "unavailable", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.origin+spec.remote, nil)
	if err != nil {
		return fallback(err)
	}
	if usable && previous.Remote == spec.remote {
		if previous.ETag != "" {
			req.Header.Set("If-None-Match", previous.ETag)
		}
		if previous.LastModified != "" {
			req.Header.Set("If-Modified-Since", previous.LastModified)
		}
	}
	response, err := client.Do(req)
	if err != nil {
		return fallback(err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified && usable && previous.Remote == spec.remote {
		return previous, "validated", nil
	}
	if response.StatusCode != http.StatusOK {
		return fallback(fmt.Errorf("HTTP %d", response.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, spec.limit+1))
	if err != nil {
		return fallback(err)
	}
	if len(data) == 0 || int64(len(data)) > spec.limit {
		return fallback(fmt.Errorf("invalid resource size"))
	}
	if !validResource(spec.mime, data) {
		return fallback(fmt.Errorf("unexpected resource content"))
	}
	if err := writeAtomic(b.filename(spec.path), data); err != nil {
		return fallback(err)
	}
	return cachedResource{spec.remote, response.Header.Get("ETag"), response.Header.Get("Last-Modified"), int64(len(data)), spec.mime}, "download", nil
}

func validResource(mime string, data []byte) bool {
	switch {
	case mime == "font/woff2":
		return bytes.HasPrefix(data, []byte("wOF2"))
	case mime == "application/json":
		return json.Valid(data)
	case mime == "image/png":
		return bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n"))
	case mime == "image/webp":
		return len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP"))
	case mime == "image/svg+xml":
		return bytes.Contains(data, []byte("<svg"))
	case strings.HasPrefix(mime, "text/css"):
		return bytes.Contains(data, []byte("@font-face")) && bytes.Contains(data, []byte("tabler-icons")) && !bytes.Contains(data, []byte("<html"))
	}
	return false
}

func writeAtomic(filename string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(filename), ".resource-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, filename)
}
