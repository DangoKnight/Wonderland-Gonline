package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"wonderland-gonline/internal/clientassets"
)

func TestPictureAtlasesPreservePixelsAndNamespaces(t *testing.T) {
	root := t.TempDir()
	names := []string{"item/a.png", "item/b.png", "item/c.png", "item_d01/a.png", "map/large.png"}
	doc := manifest{SchemaVersion: 1, Files: []source{{}}}
	originals := map[string]*image.NRGBA{}
	for i, name := range names {
		width := 3
		if i == 4 {
			width = smallPictureMaxSide + 1
		}
		m := image.NewNRGBA(image.Rect(0, 0, width, 2))
		c := color.NRGBA{R: uint8(30 + i), G: 90, B: 120, A: 180}
		if i == 2 {
			c.R = 30
		}
		for y := 0; y < 2; y++ {
			for x := 0; x < width; x++ {
				m.SetNRGBA(x, y, c)
			}
		}
		originals[name] = m
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(f, m); err != nil {
			t.Fatal(err)
		}
		f.Close()
		doc.Files[0].Pictures = append(doc.Files[0].Pictures, picture{Resource: name, PNG: name, Width: width, Height: 2})
	}
	if err := atlasPictures(root, &doc); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "manifest.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	ps := doc.Files[0].Pictures
	if ps[0].PNG != ps[1].PNG || *ps[0].Rect == *ps[1].Rect {
		t.Fatal("distinct images must occupy separate rectangles on one page")
	}
	if *ps[0].Rect != *ps[2].Rect {
		t.Fatal("identical images were not deduplicated")
	}
	if ps[0].PNG == ps[3].PNG {
		t.Fatal("patch namespace merged with base")
	}
	if ps[4].Rect != nil || ps[4].PNG != names[4] {
		t.Fatal("large image must remain standalone")
	}
	for _, name := range names {
		got, err := clientassets.LoadPicture(root, name)
		if err != nil {
			t.Fatal(err)
		}
		if got.Bounds().Min != (image.Point{}) || !samePicturePixels(got, originals[name]) {
			t.Fatalf("pixels or origin changed: %s", name)
		}
	}
	if _, err = clientassets.ReadPicturePNG(filepath.Join(root, ps[0].PNG), &clientassets.SpriteRect{X: -1, Width: 3, Height: 2}); err == nil {
		t.Fatal("out-of-bounds rectangle accepted")
	}
}

func TestFailedPictureOptimizationPreservesOriginal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pictures")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"schema_version":1,"files":[{"pictures":[{"png":"missing.png","width":3,"height":2}]}]}`)
	path := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err := optimizePictures(root); err == nil {
		t.Fatal("missing image accepted")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatal("original manifest changed after failure")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatal("failed conversion left new files in original")
	}
}
