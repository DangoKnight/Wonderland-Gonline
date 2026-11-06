package world

import (
	"image"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/surface"
)

func TestSceneryShadowRemovesCheckerboardPhasePulse(t *testing.T) {
	src := surface.New(8, 8)
	ink := surface.RGB565(32, 32, 48)
	for y := 0; y < src.H; y++ {
		for x := 0; x < src.W; x++ {
			if (x+y)%2 == 0 {
				src.Pix[y*src.W+x] = ink
			}
		}
	}
	before := append([]uint16(nil), src.Pix...)
	frames := smoothSceneryShadows(src, 1)
	if len(frames) != 1 {
		t.Fatal("shadow not reconstructed")
	}
	canvas := surface.New(8, 8)
	canvas.Fill(image.Rect(0, 0, 8, 8), 0xffff)
	o := Object{Image: src, Frames: 1, shadowFrames: frames}
	o.Draw(canvas, 0, 0, time.Time{})
	dst := surface.New(1, 1)
	var previous uint16
	for step := 0; step <= 8; step++ {
		dst.DrawSubpixel(image.Rect(0, 0, 1, 1), canvas, 2+float64(step)/8, 2)
		if step > 0 && dst.Pix[0] != previous {
			t.Fatal("shadow brightness changes with camera phase", step, previous, dst.Pix[0])
		}
		previous = dst.Pix[0]
	}
	if previous == 0xffff || previous == ink {
		t.Fatal("shadow lost its half-opacity coverage")
	}
	for i, pixel := range src.Pix {
		if pixel != before[i] {
			t.Fatal("authored image mutated")
		}
	}
}

func TestSceneryShadowPreservesArtworkAndFrameBoundaries(t *testing.T) {
	ink := surface.RGB565(32, 32, 48)
	for _, pixels := range [][]uint16{
		{ink, ink, ink, ink},   // Solid dark artwork.
		{0xffff, 0, 0, 0xffff}, // Bright stipple is not a shadow.
		{ink, 0, 0, 0},         // Isolated dark detail.
	} {
		src := surface.New(2, 2)
		copy(src.Pix, pixels)
		if smoothSceneryShadows(src, 1) != nil {
			t.Fatal("non-shadow artwork was filtered", pixels)
		}
	}
	src := surface.New(2, 2)
	src.Pix = []uint16{ink, 0, 0, ink}
	if smoothSceneryShadows(src, 2) != nil {
		t.Fatal("checker inferred across unrelated animation frames")
	}
	src = surface.New(4, 2)
	src.Pix = []uint16{ink, 0, 0xffff, 0, 0, ink, 0xffff, 0}
	filtered := smoothSceneryShadows(src, 1)[0]
	if filtered.NRGBAAt(2, 0).A != 255 || filtered.NRGBAAt(2, 0).R != 255 || filtered.NRGBAAt(3, 0).A != 0 {
		t.Fatal("shadow filtering altered adjacent opaque/transparent artwork")
	}
}
