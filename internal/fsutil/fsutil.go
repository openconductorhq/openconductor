// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

// Package fsutil holds small filesystem helpers shared across packages.
package fsutil

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
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
// chmod 600 survives); otherwise perm is used, filtered by the umask like
// os.WriteFile. Symlinks are followed so the link target is replaced, not
// the link.
func AtomicWrite(path string, content []byte, perm os.FileMode) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	existing, statErr := os.Stat(path)

	// os.CreateTemp always uses 0600; OpenFile with perm lets the umask apply.
	var tmp *os.File
	var err error
	for range 10 {
		name := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp-"+strconv.FormatUint(rand.Uint64(), 36))
		tmp, err = os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if !errors.Is(err, os.ErrExist) {
			break
		}
	}
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
	if statErr == nil {
		if err := os.Chmod(tmpPath, existing.Mode().Perm()); err != nil {
			return fmt.Errorf("setting permissions: %w", err)
		}
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}
	tmpPath = ""
	return nil
}
