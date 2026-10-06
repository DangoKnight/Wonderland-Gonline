package clientbundle

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/clientimage"
)

func TestCompiledImagesAtlasTrimDedupAndEditableParity(t *testing.T) {
	o, source := fixture(t)
	dir := filepath.Join(source, "media", "menu")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.bmp.png", "two.bmp.png", "empty.bmp.png"} {
		m := image.NewNRGBA(image.Rect(0, 0, 40, 30))
		if name != "empty.bmp.png" {
			for y := 10; y < 20; y++ {
				for x := 5; x < 15; x++ {
					m.SetNRGBA(x, y, color.NRGBA{R: 255, A: 128})
				}
			}
		}
		var b bytes.Buffer
		if err := png.Encode(&b, m); err != nil {
			t.Fatal(err)
		}
		put(t, filepath.Join(dir, name), b.Bytes())
	}
	if _, err := BuildRuntime(o); err != nil {
		t.Fatal(err)
	}
	root, closePacks, err := clientfs.Mount(o.Output)
	if err != nil {
		t.Fatal(err)
	}
	var firstFile string
	for _, name := range []string{"one.bmp.png", "two.bmp.png", "empty.bmp.png"} {
		relative := filepath.Join("media", "menu", name)
		if _, err := clientfs.Stat(filepath.Join(root, relative)); !os.IsNotExist(err) {
			t.Fatal("source PNG shipped instead of compiled record")
		}
		m, err := clientimage.Open(filepath.Join(root, relative))
		if err != nil {
			t.Fatal(err)
		}
		got, err := m.NRGBA(m.Bounds())
		if err != nil {
			t.Fatal(err)
		}
		want, err := clientassets.ReadPicturePNG(filepath.Join(source, relative), nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("compiled image differs from PNG")
		}
		stats := clientimage.Stats()
		if stats.Bytes > clientimage.CacheBytes {
			t.Fatal("image cache exceeded byte budget")
		}
	}
	// Small images use a shared trimmed page. Their independent original canvases
	// remain 40x30, and duplicate artwork points to exactly the same tile.
	for _, name := range []string{"one.bmp.png", "two.bmp.png"} {
		var data []byte
		data, err = clientfs.ReadFile(filepath.Join(root, "media", "menu", name+clientimage.DescriptorSuffix))
		if err != nil {
			t.Fatal(err)
		}
		if len(data) == 0 {
			t.Fatal("compiled descriptor missing")
		}
	}
	// Count content-addressed pages rather than depending on an encoder's bytes.
	entries, err := clientfs.ReadDir(filepath.Join(root, "runtime", "images", "core"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("duplicate/small pictures produced %d pages", len(entries))
	}
	firstFile = entries[0].Name()
	closePacks()
	// Existing dimension contracts apply before compiling or publishing records.
	info, _ := os.Stat(o.Output)
	picture(t, filepath.Join(dir, "one.bmp.png"), 41, 30, color.NRGBA{A: 255})
	if _, err = BuildRuntime(o); err == nil {
		t.Fatal("dimension contract ignored")
	}
	after, _ := os.Stat(o.Output)
	if !os.SameFile(info, after) {
		t.Fatal("invalid PNG replaced published bundle")
	}
	picture(t, filepath.Join(dir, "one.bmp.png"), 40, 30, color.NRGBA{B: 255, A: 255})
	if _, err = BuildRuntime(o); err != nil {
		t.Fatal(err)
	}
	root, closePacks, err = clientfs.Mount(o.Output)
	if err != nil {
		t.Fatal(err)
	}
	defer closePacks()
	got, err := clientassets.ReadPicturePNG(filepath.Join(root, "media", "menu", "one.bmp.png"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.NRGBAAt(0, 0).B != 255 {
		t.Fatal("edited PNG not recompiled")
	}
	entries, err = clientfs.ReadDir(filepath.Join(root, "runtime", "images", "core"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() == firstFile {
		t.Fatal("stale image pages retained in new pack")
	}
}

func TestEditingLargeImageKeepsOtherCompiledImages(t *testing.T) {
	o, source := fixture(t)
	dir := filepath.Join(source, "pictures", "map")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	picture(t, filepath.Join(dir, "one.png"), 512, 512, color.NRGBA{R: 255, A: 255})
	picture(t, filepath.Join(dir, "two.png"), 512, 512, color.NRGBA{G: 127, A: 255})
	if _, err := BuildRuntime(o); err != nil {
		t.Fatal(err)
	}
	absolute, _ := filepath.Abs(source)
	cache := filepath.Join(filepath.Dir(o.Output), ".client-runtime-"+digest([]byte(absolute))[:12])
	unchanged := filepath.Join(cache, "pictures", "map", "two.png"+clientimage.DescriptorSuffix)
	before, err := os.Stat(unchanged)
	if err != nil {
		t.Fatal(err)
	}
	picture(t, filepath.Join(dir, "one.png"), 512, 512, color.NRGBA{B: 255, A: 255})
	if _, err := BuildRuntime(o); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(unchanged)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("editing one large image recompiled its neighbour")
	}
	// Identical artwork, even in separate files, references the same physical page.
	picture(t, filepath.Join(dir, "one.png"), 512, 512, color.NRGBA{G: 127, A: 255})
	if _, err := BuildRuntime(o); err != nil {
		t.Fatal(err)
	}
	root, closePacks, err := clientfs.Mount(o.Output)
	if err != nil {
		t.Fatal(err)
	}
	defer closePacks()
	a, err := clientimage.Open(filepath.Join(root, "pictures", "map", "one.png"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := clientimage.Open(filepath.Join(root, "pictures", "map", "two.png"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Files(), b.Files()) {
		t.Fatal("identical images not deduplicated")
	}
}
