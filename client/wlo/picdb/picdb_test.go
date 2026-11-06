package picdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wonderland-gonline/client/wlo/surface"
)

func TestLoadDirPrefersPNGAndRetainsDeferredFilename(t *testing.T) {
	dir := t.TempDir()
	for name, pixel := range map[string]color.NRGBA{"Button.BMP": {R: 255, A: 255}, "button.bmp.PNG": {B: 255, A: 255}} {
		var raw bytes.Buffer
		img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
		img.SetNRGBA(0, 0, pixel)
		if err := png.Encode(&raw, img); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), raw.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	db := New()
	if err := db.LoadDir(dir, false, 0); err != nil {
		t.Fatal(err)
	}
	if len(db.Entries) != 1 || db.Entries[0].Image != nil {
		t.Fatal("deferred registration changed")
	}
	if got := db.image(db.Find("BUTTON")); got == nil || got.Pix[0] != surface.RGB565(0, 0, 255) {
		t.Fatal("PNG precedence or case handling changed", got)
	}
}

func TestRejectOversizedBitmapBeforeDecode(t *testing.T) {
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	data := raw.Bytes()
	// Valid IHDR checksum with an oversized canvas and tiny pixel payload.
	// A dimension rejection must precede pixel decompression/allocation.
	binary.BigEndian.PutUint32(data[16:20], 16384)
	binary.BigEndian.PutUint32(data[20:24], 16384)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "huge.bmp.png"), data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, now := range []bool{false, true} {
		if err := New().LoadDir(dir, now, 0); err == nil || !strings.Contains(err.Error(), "dimensions exceed") {
			t.Fatal("oversized bitmap reached decoder", now, err)
		}
	}
}

func TestDeferredDecodeFailureIsNotRetriedEachFrame(t *testing.T) {
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.bmp.png")
	if err := os.WriteFile(path, raw.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	db := New()
	if err := db.LoadDir(dir, false, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if db.image(0) != nil {
		t.Fatal("corrupt image loaded")
	}
	if err := os.WriteFile(path, raw.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if db.image(0) != nil {
		t.Fatal("failed asset was retried without resource reload")
	}
}

func BenchmarkLoadDir(b *testing.B) {
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		b.Fatal(err)
	}
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			dir := b.TempDir()
			for i := range count {
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("skin_%04d.bmp.png", i)), raw.Bytes(), 0600); err != nil {
					b.Fatal(err)
				}
			}
			b.ResetTimer()
			for range b.N {
				db := New()
				if err := db.LoadDir(dir, false, 0); err != nil || len(db.Entries) != count {
					b.Fatal(err, len(db.Entries))
				}
			}
		})
	}
}

// FUN_0047b274's pixel rules.
func TestConvertRules(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 6, 1))
	for x, c := range []color.RGBA{
		{0, 255, 0, 255},   // key
		{0, 0, 0, 255},     // near black -> blue 8
		{7, 7, 7, 255},     // near black -> blue 8
		{0, 0, 8, 255},     // kept
		{250, 100, 0, 255}, // tinted
		{8, 0, 0, 255},     // not near black: red 8
	} {
		img.SetRGBA(x, 0, c)
	}
	db := New()
	s := db.convert(img, 0x00102030) // blue +0x30, green +0x20, red +0x10
	want := []uint16{
		0,
		surface.RGB565(0, 0, 8),
		surface.RGB565(7+0x10, 7+0x20, 8+0x30), // still tinted: only exact (0,0,8) is kept
		surface.RGB565(0, 0, 8),
		surface.RGB565(255, 100+0x20, 0x30),
		surface.RGB565(8+0x10, 0x20, 0x30),
	}
	for i, w := range want {
		if s.Pix[i] != w {
			t.Errorf("pixel %d = %#04x, want %#04x", i, s.Pix[i], w)
		}
	}
	db.RGB555 = true
	if v := db.convert(img, 0).Pix[4]; v != uint16(250>>3)<<11|uint16(100>>3)<<5 {
		t.Errorf("555 pixel = %#04x", v)
	}
}

func TestOverrideAndCase(t *testing.T) {
	dir := t.TempDir()
	write := func(sub, name string, c color.RGBA) {
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		img.SetRGBA(0, 0, c)
		os.MkdirAll(filepath.Join(dir, sub), 0o755)
		f, _ := os.Create(filepath.Join(dir, sub, name+".bmp"))
		defer f.Close()
		// The decoder accepts any registered format; PNG bytes keep the
		// fixture small while exercising the same path.
		png.Encode(f, img)
	}
	write("white", "Btn", color.RGBA{255, 0, 0, 255})
	write("default", "btn", color.RGBA{0, 0, 255, 255})
	db := New()
	db.LoadDir(filepath.Join(dir, "white"), true, 0)
	db.LoadDir(filepath.Join(dir, "default"), true, 0)
	i := db.Find("BTN")
	if i < 0 || len(db.Entries) != 1 || db.image(i).Pix[0] != surface.RGB565(255, 0, 0) {
		t.Fatal("first registration must win", i, len(db.Entries))
	}
	if db.Find("missing") != -1 {
		t.Fatal("missing name found")
	}
}

// The converted logo matches the captured original pixel for pixel. The
// capture only confirms the decompiled rules; it does not define them.
func TestLogoMatchesCapture(t *testing.T) {
	assets := filepath.Join("..", "..", "..", "data", "media", "menu", "Skins", "default")
	db := New()
	if err := db.Load(assets, "Icon_LoginLogo_1", true, 0); err != nil || db.Find("Icon_LoginLogo_1") < 0 {
		t.Skip("client assets not installed")
	}
	f, err := os.Open(filepath.Join("..", "..", "reference", "screenshots", "Login", "login.png"))
	if err != nil {
		t.Skip("no capture")
	}
	defer f.Close()
	ref, _ := png.Decode(f)
	dst := surface.New(800, 600)
	db.Draw(dst, db.Find("Icon_LoginLogo_1"), 617, 0, true)
	got := dst.RGBA()
	w, h := db.Size(db.Find("Icon_LoginLogo_1"))
	for y := 0; y < h; y++ {
		for x := 617; x < 617+w; x++ {
			r, g, b, _ := ref.At(x, y).RGBA()
			c := got.RGBAAt(x, y)
			if c.R != uint8(r>>8) || c.G != uint8(g>>8) || c.B != uint8(b>>8) {
				t.Fatalf("(%d,%d) = %v, capture %d,%d,%d", x, y, c, r>>8, g>>8, b>>8)
			}
		}
	}
}

func TestEditablePNGTransparency(t *testing.T) {
	db := New()
	m := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	m.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 0})
	m.SetNRGBA(1, 0, color.NRGBA{R: 255, A: 255})
	db.Add("edited", m)
	if db.Opaque(db.Find("edited"), 0, 0) {
		t.Fatal("transparent edited PNG became opaque")
	}
	if !db.Opaque(db.Find("edited"), 1, 0) {
		t.Fatal("opaque edited PNG disappeared")
	}
}
