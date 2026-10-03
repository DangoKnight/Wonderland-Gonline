package surface

import (
	"image"
	"testing"
)

// TestDrawLight checks ro_Clipper_LightAlpha_ColorKey_Blt: each channel
// adds the source scaled by level/32, saturating; the key is skipped.
func TestDrawLight(t *testing.T) {
	src := New(3, 1)
	src.Pix = []uint16{RGB565(0x80, 0x80, 0x80), 0, RGB565(0xff, 0xff, 0xff)}
	dst := New(3, 1)
	grey := RGB565(0x40, 0x40, 0x40)
	dst.Pix = []uint16{grey, grey, grey}
	dst.DrawLight(0, 0, image.Rect(0, 0, 3, 1), src, LightLevelFull/2)
	// 0x80 is 16/32 in five bits and 32/64 in six: half of it is 8 and 16;
	// half of white is 15 and 31.
	want := []uint16{grey + 8<<11 + 16<<5 + 8, grey, grey + 15<<11 + 31<<5 + 15}
	for i, w := range want {
		if dst.Pix[i] != w {
			t.Errorf("pixel %d = %04x, want %04x", i, dst.Pix[i], w)
		}
	}
	dst.Pix[0] = RGB565(0xf0, 0xf0, 0xf0)
	dst.DrawLight(0, 0, image.Rect(0, 0, 1, 1), src, LightLevelFull)
	if dst.Pix[0] != 0xffff {
		t.Errorf("saturated pixel = %04x", dst.Pix[0])
	}
}

// TestFillAlpha16: FillRectAlpha blends the 5/6/5-bit channels; the
// original's tooltip ($F98B3D at 200) over (0, 125, 156) gives
// (41, 134, 231) on screen.
func TestFillAlpha16(t *testing.T) {
	s := New(1, 1)
	s.Pix[0] = RGB565(0, 125, 156)
	s.FillAlpha(image.Rect(0, 0, 1, 1), 0xf98b3d, 200)
	if got := Expand(s.Pix[0]); got.R != 41 || got.G != 134 || got.B != 231 {
		t.Fatalf("blend %v", got)
	}
}
