package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStagePackageAndVerify(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "installation")
	dest := filepath.Join(root, "assets")
	packs := filepath.Join(root, "upload")
	manifestPath := filepath.Join(root, "manifest.json")
	if err := os.MkdirAll(filepath.Join(source, "pic"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "pic", "map.JMG"), []byte("native archive"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "aLogin.exe"), []byte("exclude"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := run(source, dest, manifestPath, packs, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "aLogin.exe")); !os.IsNotExist(err) {
		t.Fatal("executable copied")
	}
	z, err := zip.OpenReader(filepath.Join(packs, "pic.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if len(z.File) != 2 || z.File[0].Name != "manifest.json" || z.File[1].Name != "pic/map.JMG" {
		t.Fatal(z.File)
	}
	data, err := os.ReadFile(filepath.Join(root, "asset-packages.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Packages []packageEntry `json:"packages"`
	}
	if err = json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	n, sum, err := digest(filepath.Join(packs, "pic.zip"))
	if err != nil || len(catalog.Packages) != 1 || catalog.Packages[0].File != "pic.zip" || catalog.Packages[0].Size != n || catalog.Packages[0].SHA256 != sum {
		t.Fatal(catalog, err)
	}
	if err = run("", dest, manifestPath, "", true); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(root, "installed")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	// Git can normalize tracked JSON line endings on a Windows checkout.
	if err = os.WriteFile(manifestPath, bytes.ReplaceAll(manifestData, []byte("\n"), []byte("\r\n")), 0644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // Fresh installation and an idempotent rerun.
		if err = unpackAssets(packs, installed, manifestPath, filepath.Join(root, "asset-packages.json")); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(installed, "pic", "map.JMG"), []byte("damaged"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = unpackAssets(packs, installed, manifestPath, filepath.Join(root, "asset-packages.json")); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(packs, "pic.zip"), []byte("bad download"), 0644); err != nil {
		t.Fatal(err)
	}
	untouched := filepath.Join(root, "untouched")
	if err = unpackAssets(packs, untouched, manifestPath, filepath.Join(root, "asset-packages.json")); err == nil {
		t.Fatal("bad package accepted")
	}
	if _, err = os.Stat(untouched); !os.IsNotExist(err) {
		t.Fatal("destination changed before package validation")
	}
	if err = os.WriteFile(filepath.Join(dest, "pic", "map.JMG"), []byte("broken archive"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = run("", dest, manifestPath, "", true); err == nil {
		t.Fatal("corruption accepted")
	}
}

func TestRejectUnsafeResourcePaths(t *testing.T) {
	for _, name := range []string{"../outside", "/absolute", "C:/file", "pic/../file", "pic\\file", "pic//file", ""} {
		if _, err := safePath(t.TempDir(), name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
}
