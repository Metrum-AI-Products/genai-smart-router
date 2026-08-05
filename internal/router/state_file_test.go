package router

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVersionedStateFileRecoversOnlyMissingPrimaryFromAtomicBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := writeVersionedStateFile(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := writeVersionedStateFile(path, []byte("second")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	raw, err := readVersionedStateFile(path)
	if err != nil || string(raw) != "first" {
		t.Fatalf("missing-primary recovery raw=%q err=%v", raw, err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err = readVersionedStateFile(path)
	if err != nil || string(raw) != "tampered" {
		t.Fatalf("present primary must not be masked raw=%q err=%v", raw, err)
	}
}
