package picdb

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientbundle"
	"wonderland-gonline/internal/clientfs"
)

func TestCompiledUISkinMatchesPNGForKeysAlphaTintAndRGB555(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "data")
	dir := filepath.Join(source, "media", "menu", "Skins", "white")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	m := image.NewNRGBA(image.Rect(0, 0, 40, 30))
	colors := []color.NRGBA{{}, {A: 255}, {R: 7, G: 7, B: 7, A: 255}, {G: 255, A: 255}, {R: 48, G: 125, B: 227, A: 128}, {R: 159, G: 221, B: 127, A: 255}, {R: 1, G: 254, B: 1, A: 255}}
	for y := 7; y < 25; y++ {
		for x := 4; x < 30; x++ {
			m.SetNRGBA(x, y, colors[(x+y)%len(colors)])
		}
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "button.bmp.png"), raw.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(root, "assets.zip")
	if _, err := clientbundle.BuildRuntime(clientbundle.Options{Source: source, Output: bundle, Contract: filepath.Join(root, "contract.json")}); err != nil {
		t.Fatal(err)
	}
	packed, closePacks, err := clientfs.Mount(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer closePacks()
	for _, now := range []bool{false, true} {
		for _, rgb555 := range []bool{false, true} {
			for _, tint := range []uint32{0, 0x112233} {
				want, got := New(), New()
				want.RGB555, got.RGB555 = rgb555, rgb555
				if err := want.LoadDir(dir, now, tint); err != nil {
					t.Fatal(err)
				}
				if err := got.LoadDir(filepath.Join(packed, "media", "menu", "Skins", "white"), now, tint); err != nil {
					t.Fatal(err)
				}
				if len(got.Entries) != 1 || len(got.Missing) != 0 {
					t.Fatal("compiled skin enumeration failed")
				}
				if !reflect.DeepEqual(got.image(0), want.image(0)) {
					t.Fatalf("UI conversion differs: immediate=%v RGB555=%v tint=%x", now, rgb555, tint)
				}
				a, b := surface.New(80, 60), surface.New(80, 60)
				a.Fill(image.Rect(0, 0, a.W, a.H), 9)
				b.Fill(image.Rect(0, 0, b.W, b.H), 9)
				want.Draw(a, 0, 10, 10, true)
				got.Draw(b, 0, 10, 10, true)
				if !reflect.DeepEqual(a.Pix, b.Pix) {
					t.Fatal("UI blit differs")
				}
			}
		}
	}
}
