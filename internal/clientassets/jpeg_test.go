package clientassets

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The fixtures' .ppm files are libjpeg's output (djpeg -dct int), which the
// original client's IJG decoder matches.
func TestDecodeJPEGMatchesLibjpeg(t *testing.T) {
	for _, name := range []string{"ycc444", "ycc420", "ycc422", "restart"} {
		raw, err := os.ReadFile(filepath.Join("testdata", name+".jpg"))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("testdata", name+".ppm"))
		if err != nil {
			t.Fatal(err)
		}
		var w, h int
		if _, err := fmt.Sscanf(string(want), "P6\n%d %d\n255\n", &w, &h); err != nil {
			t.Fatal(name, err)
		}
		pix := want[len(want)-w*h*3:]
		got, err := DecodeJPEG(raw)
		if err != nil {
			t.Fatal(name, err)
		}
		if got.Rect.Dx() != w || got.Rect.Dy() != h {
			t.Fatal(name, got.Rect)
		}
		for i := range w * h {
			if !bytes.Equal(got.Pix[4*i:4*i+3], pix[3*i:3*i+3]) {
				t.Fatalf("%s: pixel (%d,%d) = %v, want %v", name, i%w, i/w, got.Pix[4*i:4*i+3], pix[3*i:3*i+3])
			}
		}
	}
}

func TestDecodeJPEGRejectsTruncated(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "ycc420.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{0, 2, 20, 200, len(raw) - 2} {
		if _, err := DecodeJPEG(raw[:n]); err == nil {
			t.Errorf("%d bytes accepted", n)
		}
	}
}
