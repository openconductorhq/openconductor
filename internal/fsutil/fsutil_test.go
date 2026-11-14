// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	tests := []struct{ in, want string }{
		{"~/foo/bar", filepath.Join(home, "foo/bar")},
		{"/abs/path", "/abs/path"},
		{"rel/path", "rel/path"},
		{"~user/x", "~user/x"},
	}
	for _, tt := range tests {
		if got := ExpandHome(tt.in); got != tt.want {
			t.Errorf("ExpandHome(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")

	// New file gets the requested perm.
	if err := AtomicWrite(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertFile(t, path, "one", 0o644)

	// Existing file keeps its mode even when a looser perm is passed.
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(path, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertFile(t, path, "two", 0o600)

	// Writing through a symlink replaces the target, keeping the link.
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(link, []byte("three"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link was replaced: %v", err)
	}
	assertFile(t, path, "three", 0o600)

	// No temp files left behind.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func assertFile(t *testing.T, path, want string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Errorf("content = %q, want %q", data, want)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != mode {
		t.Errorf("mode = %o, want %o", fi.Mode().Perm(), mode)
	}
}
