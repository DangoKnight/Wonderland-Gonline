package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDistributionIncludesEditableSprites(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "distribution")
	os.MkdirAll(filepath.Join(src, "sprites", "001"), 0755)
	os.WriteFile(filepath.Join(src, "sprites", "001", "editable.json"), []byte("editable metadata"), 0644)
	if _, _, _, err := syncData(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "sprites", "001", "editable.json")); err != nil {
		t.Fatal(err)
	}
}
