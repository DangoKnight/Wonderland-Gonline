package surface

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wonderland-gonline/internal/clientimage"
)

func TestTiledDrawClippingMatchesNormalSurface(t *testing.T) {
	root := t.TempDir()
	source := "pictures/test.png"
	original := image.NewNRGBA(image.Rect(0, 0, 600, 500))
	for y := 0; y < 500; y++ {
		for x := 0; x < 600; x++ {
			original.SetNRGBA(x, y, color.NRGBA{R: byte(x), G: byte(y), B: byte(x * y), A: byte(x + y)})
		}
	}
	d := clientimage.Descriptor{Version: clientimage.Version, Source: source, Width: 600, Height: 500}
	for y := 0; y < 500; y += clientimage.TileSide {
		for x := 0; x < 600; x += clientimage.TileSide {
			r := image.Rect(x, y, min(x+clientimage.TileSide, 600), min(y+clientimage.TileSide, 500))
			data, err := clientimage.EncodeChunk(original.SubImage(r).(*image.NRGBA))
			if err != nil {
				t.Fatal(err)
			}
			file := clientimage.ChunkPath("world", data)
			path := filepath.Join(root, filepath.FromSlash(file))
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				t.Fatal(err)
			}
			d.Tiles = append(d.Tiles, clientimage.Tile{Rect: r, Page: image.Rect(0, 0, r.Dx(), r.Dy()), File: file})
		}
	}
	data, err := clientimage.EncodeDescriptor(d)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(source))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+clientimage.DescriptorSuffix, data, 0644); err != nil {
		t.Fatal(err)
	}
	compiled, err := clientimage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	lazy, normal := FromCompiled(compiled), FromImage(original)
	for _, r := range []image.Rectangle{image.Rect(0, 0, 600, 500), image.Rect(230, 220, 590, 450), image.Rect(-5, -10, 90, 100)} {
		for _, at := range []image.Point{{}, {X: -53, Y: -17}, {X: 100, Y: 40}, {X: 401, Y: 201}} {
			for _, transparent := range []bool{false, true} {
				a, b := New(400, 200), New(400, 200)
				a.Fill(image.Rect(0, 0, 400, 200), 15)
				b.Fill(image.Rect(0, 0, 400, 200), 15)
				a.DrawRect(at.X, at.Y, r, normal, transparent)
				b.DrawRect(at.X, at.Y, r, lazy, transparent)
				if !reflect.DeepEqual(a.Pix, b.Pix) {
					t.Fatalf("tiled clipping differs: %v at %v transparent=%v", r, at, transparent)
				}
			}
		}
	}
	if lazy.Pix != nil {
		t.Fatal("drawing materialized full map")
	}
}
