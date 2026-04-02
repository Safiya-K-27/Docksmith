package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeterministicTarForSameInput(t *testing.T) {
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "a", "f.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := map[string]snapEntry{}
	after, err := snapshotTree(d)
	if err != nil {
		t.Fatal(err)
	}
	changed := changedPaths(before, after)
	t1, err := buildDeltaTar(d, changed)
	if err != nil {
		t.Fatal(err)
	}
	t2, err := buildDeltaTar(d, changed)
	if err != nil {
		t.Fatal(err)
	}
	if sha256Bytes(t1) != sha256Bytes(t2) {
		t.Fatal("tar bytes should be deterministic")
	}
}
