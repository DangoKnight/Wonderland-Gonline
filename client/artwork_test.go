package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wonderland-gonline/internal/clientassets"
)

func TestNativeArtworkDecode(t *testing.T) {
	dir := os.Getenv("WONDERLAND_CLIENT_DATA")
	if dir == "" {
		t.Skip("set WONDERLAND_CLIENT_DATA")
	}
	for _, name := range []string{"map.JMG", "images.BMg", "item.BMg"} {
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(filepath.Join(dir, "pic", name))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			stat, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}
			a, err := clientassets.ReadImageArchive(f, stat.Size())
			if err != nil {
				t.Fatal(err)
			}
			decoded := 0
			for i, e := range a.Entries {
				img, err := decodeImage(a, i)
				if err != nil {
					// This source declares 255 colors but contains pixel index 255.
					// Retain the malformed original rather than inventing a color.
					if e.Name == "HiTurtle_2.bmp" && strings.Contains(err.Error(), "invalid palette index") {
						t.Logf("known malformed resource: %s (%v)", e.Name, err)
						continue
					}
					t.Errorf("%s: %v", e.Name, err)
					continue
				}
				if img.Bounds().Empty() {
					t.Fatal("empty image", e.Name)
				}
				decoded++
			}
			t.Logf("decoded %d of %d images", decoded, len(a.Entries))
		})
	}
}
