// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) != 3 {
		panic("usage: packfrontend source-directory output.zip")
	}
	if err := pack(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func pack(source, destination string) error {
	f, err := os.Create(destination + ".tmp")
	if err != nil {
		return err
	}
	defer f.Close()
	z := zip.NewWriter(f)
	count := 0
	err = filepath.WalkDir(source, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("refusing linked asset %s", filename)
		}
		relative, err := filepath.Rel(source, filename)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if !fs.ValidPath(name) || strings.HasPrefix(name, "client-assets/camp/") {
			return fmt.Errorf("forbidden asset %s", name)
		}
		file, err := os.Open(filename)
		if err != nil {
			return err
		}
		defer file.Close()
		header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)}
		header.SetMode(0644)
		out, err := z.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, file); err != nil {
			return err
		}
		count++
		return nil
	})
	if err != nil {
		_ = z.Close()
		return err
	}
	if err = z.Close(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(destination+".tmp", destination); err != nil {
		return err
	}
	info, err := os.Stat(destination)
	if err != nil {
		return err
	}
	fmt.Printf("Desktop frontend: %d files, %.2f MiB packed\n", count, float64(info.Size())/(1<<20))
	return nil
}
