// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

// Package fsutil holds small filesystem helpers shared across packages.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome replaces a leading "~/" with the user's home directory. The
// path is returned unchanged if it has no "~/" prefix or the home directory
// cannot be determined.
func ExpandHome(p string) string {
	if !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[2:])
}

// AtomicWrite writes content to path by writing a temp file in the same
// directory and renaming it into place, so a crash never leaves a truncated
// file. If path already exists its permission bits are kept (a user's
// chmod 600 survives); otherwise perm is used. Symlinks are followed so the
// link target is replaced, not the link.
func AtomicWrite(path string, content []byte, perm os.FileMode) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}
	tmpPath = ""
	return nil
}
