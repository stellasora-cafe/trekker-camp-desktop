// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

// Package webmedia retains public images viewed through the desktop client.
package webmedia

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxImageBytes int64 = 12 << 20
const maxCacheBytes int64 = 256 << 20
const maxHeaderBytes = 4096

type Cache struct {
	origin    string
	directory string
	client    *http.Client
	budget    int64
	mu        sync.Mutex
}

type imageHeader struct {
	ContentType  string
	ETag         string
	LastModified string
	Headers      http.Header
}

var retainedHeaders = []string{
	"Content-Encoding", "Content-Disposition", "Content-Security-Policy",
	"X-Content-Type-Options", "Cross-Origin-Resource-Policy",
	"Access-Control-Allow-Origin", "Access-Control-Allow-Credentials", "Vary",
}

type imageEntry struct {
	header imageHeader
	body   []byte
}

// New isolates cache files by server origin. Invalid origins fail closed.
func New(origin, directory string) *Cache {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.ResponseHeaderTimeout = 5 * time.Second
	c := &Cache{budget: maxCacheBytes, client: &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
	u, err := url.Parse(origin)
	if err != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return c
	}
	c.origin = originOf(u)
	if c.origin != "" {
		key := sha256.Sum256([]byte(c.origin))
		c.directory = filepath.Join(directory, hex.EncodeToString(key[:]))
	}
	return c
}

func originOf(u *url.URL) string {
	if u.User != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if port == "80" && u.Scheme == "http" || port == "443" && u.Scheme == "https" {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return u.Scheme + "://" + host
}

func (c *Cache) Matches(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || c.origin == "" || originOf(u) != c.origin || u.Fragment != "" || u.RawPath != "" {
		return false
	}
	if strings.ContainsAny(u.Path, "\\\x00\r\n") || strings.TrimSuffix(u.Path, "/") != path.Clean(u.Path) {
		return false
	}
	return strings.HasPrefix(u.Path, "/files/") || strings.HasPrefix(u.Path, "/proxy/") ||
		u.Path == "/avatar" || strings.HasPrefix(u.Path, "/avatar/") ||
		u.Path == "/identicon" || strings.HasPrefix(u.Path, "/identicon/")
}

func (c *Cache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !c.Matches(r.URL.String()) {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// Native WebView headers may arrive with Chromium's lowercase spelling.
	headers := http.Header{}
	for name, values := range r.Header {
		for _, value := range values {
			headers.Add(name, value)
		}
	}
	r = r.Clone(r.Context())
	r.Header = headers
	public := r.Header.Get("Authorization") == "" && r.Header.Get("Cookie") == ""
	key := c.filename(r.URL)
	var cached *imageEntry
	if public {
		cached = c.read(key)
	}
	request := r.Clone(r.Context())
	request.RequestURI = ""
	request.Host = ""
	request.Header.Del("Connection")
	if cached != nil && request.Header.Get("If-None-Match") == "" && request.Header.Get("If-Modified-Since") == "" {
		if cached.header.ETag != "" {
			request.Header.Set("If-None-Match", cached.header.ETag)
		} else if cached.header.LastModified != "" {
			request.Header.Set("If-Modified-Since", cached.header.LastModified)
		}
	}
	response, err := c.client.Do(request)
	if err != nil {
		if cached != nil && r.Context().Err() == nil {
			serveCached(w, r, cached)
		} else {
			http.Error(w, "image unavailable", http.StatusBadGateway)
		}
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusNotFound {
		c.remove(key)
	}
	if cached != nil && (response.StatusCode == http.StatusNotModified || response.StatusCode == http.StatusBadGateway || response.StatusCode == http.StatusServiceUnavailable || response.StatusCode == http.StatusGatewayTimeout) {
		serveCached(w, r, cached)
		return
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 && response.Header.Get("Location") != "" {
		target, err := r.URL.Parse(response.Header.Get("Location"))
		if err != nil || originOf(target) == "" {
			http.Error(w, "image redirect blocked", http.StatusBadGateway)
			return
		}
		// Leave HTTP(S) redirects to the WebView, which owns normal resource
		// loading and cross-origin credential handling. The proxy never follows.
	}
	typeOf, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	cacheControl := strings.ToLower(response.Header.Get("Cache-Control"))
	cacheable := public && strings.HasPrefix(typeOf, "image/") && response.Header.Get("Set-Cookie") == "" && !strings.Contains(cacheControl, "private") && !strings.Contains(cacheControl, "no-store")
	if public && response.StatusCode == http.StatusOK && !cacheable {
		c.remove(key)
	}
	if r.Method == http.MethodGet && response.StatusCode == http.StatusOK && cacheable && response.ContentLength <= maxImageBytes {
		body, err := io.ReadAll(io.LimitReader(response.Body, maxImageBytes+1))
		if err != nil {
			if cached != nil {
				serveCached(w, r, cached)
			} else {
				http.Error(w, "image unavailable", http.StatusBadGateway)
			}
			return
		}
		if len(body) > 0 && int64(len(body)) <= maxImageBytes {
			header := imageHeader{
				ContentType: response.Header.Get("Content-Type"), ETag: response.Header.Get("ETag"),
				LastModified: response.Header.Get("Last-Modified"), Headers: http.Header{},
			}
			for _, name := range retainedHeaders {
				if values := response.Header.Values(name); len(values) != 0 {
					header.Headers[name] = values
				}
			}
			c.write(key, &imageEntry{header, body})
		}
		response.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(body), response.Body), response.Body}
	}
	for name, values := range response.Header {
		switch name {
		case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
			continue
		}
		w.Header()[name] = values
	}
	w.WriteHeader(response.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, response.Body)
	}
}

func serveCached(w http.ResponseWriter, r *http.Request, entry *imageEntry) {
	for _, name := range retainedHeaders {
		if values := entry.header.Headers.Values(name); len(values) != 0 {
			w.Header()[name] = values
		}
	}
	w.Header().Set("Content-Type", entry.header.ContentType)
	w.Header().Set("Cache-Control", "no-cache")
	if entry.header.ETag != "" {
		w.Header().Set("ETag", entry.header.ETag)
	}
	modified, _ := http.ParseTime(entry.header.LastModified)
	http.ServeContent(w, r, "", modified, bytes.NewReader(entry.body))
}

func (c *Cache) filename(u *url.URL) string {
	key := sha256.Sum256([]byte(u.RequestURI()))
	return filepath.Join(c.directory, hex.EncodeToString(key[:])+".image")
}

func (c *Cache) read(filename string) *imageEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	file, err := os.Open(filename)
	if err != nil {
		return nil
	}
	defer file.Close()
	var length uint32
	if binary.Read(file, binary.BigEndian, &length) != nil || length > maxHeaderBytes {
		return nil
	}
	header := make([]byte, length)
	var entry imageEntry
	if _, err := io.ReadFull(file, header); err != nil || json.Unmarshal(header, &entry.header) != nil || !strings.HasPrefix(strings.ToLower(entry.header.ContentType), "image/") {
		return nil
	}
	entry.body, err = io.ReadAll(io.LimitReader(file, maxImageBytes+1))
	if err != nil || len(entry.body) == 0 || int64(len(entry.body)) > maxImageBytes {
		return nil
	}
	now := time.Now()
	_ = os.Chtimes(filename, now, now)
	return &entry
}

func (c *Cache) write(filename string, entry *imageEntry) {
	header, err := json.Marshal(entry.header)
	if err != nil || len(header) > maxHeaderBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if os.MkdirAll(c.directory, 0700) != nil {
		return
	}
	file, err := os.CreateTemp(c.directory, ".image-*")
	if err != nil {
		return
	}
	defer os.Remove(file.Name())
	err = binary.Write(file, binary.BigEndian, uint32(len(header)))
	if err == nil {
		_, err = file.Write(header)
	}
	if err == nil {
		_, err = file.Write(entry.body)
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil || os.Rename(file.Name(), filename) != nil {
		return
	}
	c.evict()
}

func (c *Cache) remove(filename string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = os.Remove(filename)
}

func (c *Cache) evict() {
	files, _ := os.ReadDir(c.directory)
	var total int64
	type candidate struct {
		name string
		info os.FileInfo
	}
	var entries []candidate
	for _, file := range files {
		if len(file.Name()) != 70 || !strings.HasSuffix(file.Name(), ".image") {
			continue
		}
		if _, err := hex.DecodeString(strings.TrimSuffix(file.Name(), ".image")); err != nil {
			continue
		}
		info, err := file.Info()
		if err == nil && info.Mode().IsRegular() {
			total += info.Size()
			entries = append(entries, candidate{file.Name(), info})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].info.ModTime().Before(entries[j].info.ModTime()) })
	for _, entry := range entries {
		if total <= c.budget {
			break
		}
		if os.Remove(filepath.Join(c.directory, entry.name)) == nil {
			total -= entry.info.Size()
		}
	}
}
