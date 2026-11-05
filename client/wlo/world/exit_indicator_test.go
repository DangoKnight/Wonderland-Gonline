package world

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExitIndicatorContinuousAnimation(t *testing.T) {
	p := newExitIndicator()
	p.render(0)
	first := bytes.Clone(p.pixels.Pix)
	previous := first
	for i := 1; i <= 20; i++ {
		p.render(time.Duration(i) * time.Second / 60)
		if bytes.Equal(previous, p.pixels.Pix) {
			t.Fatalf("animation paused at frame %d", i)
		}
		previous = bytes.Clone(p.pixels.Pix)
	}
	p.render(exitIndicatorPeriod)
	if !bytes.Equal(first, p.pixels.Pix) {
		t.Fatal("animation does not loop continuously")
	}
	for y := 0; y < exitIndicatorHeight; y++ {
		for x := 0; x < exitIndicatorWidth; x++ {
			if (x == 0 || y == 0 || x == exitIndicatorWidth-1 || y == exitIndicatorHeight-1) && p.pixels.NRGBAAt(x, y).A != 0 {
				t.Fatal("glow clipped at canvas edge")
			}
		}
	}
	if dir := os.Getenv("SNAPSHOT_DIR"); dir != "" {
		preview := image.NewNRGBA(image.Rect(0, 0, exitIndicatorWidth*4, exitIndicatorHeight))
		draw.Draw(preview, preview.Bounds(), image.NewUniform(color.NRGBA{R: 115, G: 77, B: 38, A: 255}), image.Point{}, draw.Src)
		for i := 0; i < 4; i++ {
			p.render(time.Duration(i) * exitIndicatorPeriod / 4)
			draw.Draw(preview, image.Rect(i*exitIndicatorWidth, 0, (i+1)*exitIndicatorWidth, exitIndicatorHeight), p.pixels, image.Point{}, draw.Over)
		}
		f, err := os.Create(filepath.Join(dir, "exit-indicator.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err = png.Encode(f, preview); err != nil {
			t.Fatal(err)
		}
	}
}
