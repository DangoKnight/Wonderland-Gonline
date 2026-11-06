package surface

import (
	"image"
	"testing"
)

func TestSubpixelInterpolatesDecodedColors(t *testing.T) {
	src := New(2, 2)
	src.Pix = []uint16{0xf800, 0x001f, 0, 0xffff}
	dst := New(1, 1)
	for _, tc := range []struct {
		x, y float64
		want uint16
	}{
		{0, 0, 0xf800}, {.5, 0, 0x780f}, {.5, .5, 0x79ef},
	} {
		dst.DrawSubpixel(image.Rect(0, 0, 1, 1), src, tc.x, tc.y)
		if dst.Pix[0] != tc.want {
			t.Fatalf("offset %.2f,%.2f: %04x want %04x", tc.x, tc.y, dst.Pix[0], tc.want)
		}
	}
}

func TestSubpixelReconstructionRetainsFineDetail(t *testing.T) {
	src := New(4, 4)
	for y := 0; y < 4; y++ {
		src.Pix[y*4+1] = 0xffff
	}
	dst := New(1, 1)
	dst.DrawSubpixel(image.Rect(0, 0, 1, 1), src, 1.25, 1)
	// An isolated bright stroke retains 56/64 coverage at this phase;
	// bilinear sampling retained only 3/4 and visibly softened sprite detail.
	if dst.Pix[0] != 0xdefb {
		t.Fatalf("fine stroke reconstructed as %04x", dst.Pix[0])
	}
}
