package main

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUnpackRejectsUnlistedZIPEntry(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.json")
	m := manifest{Version: 1, Files: []entry{{Path: "pic/image.bmp", Size: 1, SHA256: "unused"}}}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(root, "pic.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	w, err := z.Create("../outside")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	n, sum, err := digest(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	catalog := struct {
		Version  int            `json:"version"`
		Packages []packageEntry `json:"packages"`
	}{1, []packageEntry{{"pic.zip", n, sum}}}
	data, _ = json.Marshal(catalog)
	catalogPath := filepath.Join(root, "packages.json")
	if err = os.WriteFile(catalogPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err = unpackAssets(root, filepath.Join(root, "assets"), manifestPath, catalogPath); err == nil {
		t.Fatal("path traversal entry accepted")
	}
	if _, err = os.Stat(filepath.Join(root, "outside")); !os.IsNotExist(err) {
		t.Fatal("wrote outside asset directory")
	}
}
