package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWritePrivateFileReplacesBroadPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coverage.json")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateFile(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions=%#o, want 0600", info.Mode().Perm())
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "new" {
		t.Fatalf("content=%q err=%v", content, err)
	}
}
