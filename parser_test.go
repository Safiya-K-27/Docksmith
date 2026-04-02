package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDocksmithfileRejectsUnknown(t *testing.T) {
	d := t.TempDir()
	content := "FROM mini:latest\nUNKNOWN x\n"
	if err := os.WriteFile(filepath.Join(d, "Docksmithfile"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := parseDocksmithfile(d)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseDocksmithfileRequiresCmdArray(t *testing.T) {
	d := t.TempDir()
	content := "FROM mini:latest\nCMD hello\n"
	if err := os.WriteFile(filepath.Join(d, "Docksmithfile"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := parseDocksmithfile(d)
	if err == nil {
		t.Fatal("expected CMD parse error")
	}
}
