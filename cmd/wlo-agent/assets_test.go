package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAssetPackNames(t *testing.T) {
	for _, test := range []struct {
		name, manifest string
		fail           bool
	}{
		{"standalone", "", false},
		{"manifest", `{"version":1,"packs":["world.zip"]}`, false},
		{"traversal", `{"version":1,"packs":["../private.zip"]}`, true},
		{"duplicate", `{"version":1,"packs":["world.zip","world.zip"]}`, true},
		{"core duplicate", `{"version":1,"packs":["client-assets.zip"]}`, true},
		{"missing", `{"version":1,"packs":["missing.zip"]}`, true},
		{"version", `{"version":2,"packs":[]}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			f, err := os.Create(filepath.Join(dir, coreAssetPack))
			if err != nil {
				t.Fatal(err)
			}
			z := zip.NewWriter(f)
			if test.manifest != "" {
				w, err := z.Create("_packs.json")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := w.Write([]byte(test.manifest)); err != nil {
					t.Fatal(err)
				}
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			f.Close()
			for _, name := range []string{"world.zip", "old-world.zip", "private.txt"} {
				if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			names, err := assetPackNames(dir)
			if test.fail {
				if err == nil {
					t.Fatal("accepted invalid manifest")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := []string{coreAssetPack}
			if test.manifest != "" {
				want = append(want, "world.zip")
			}
			if !reflect.DeepEqual(names, want) {
				t.Fatalf("got %v want %v", names, want)
			}
		})
	}
}
