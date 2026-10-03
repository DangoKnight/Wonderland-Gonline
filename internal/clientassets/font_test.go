package clientassets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFontIndexing(t *testing.T) {
	raw := make([]byte, fontASCIISize+fontBig5Count*fontWideSize)
	for i := range raw {
		raw[i] = fontKey
	}
	mark := func(i int) { raw[fontASCIISize+i*fontWideSize] = 0xff ^ fontKey }
	mark(0)     // A440
	mark(13501) // A3BF
	mark(5401)  // C940, after the C6A1-C8FE gap
	raw['A'*FontHeight] = 0x80 ^ fontKey
	f, err := DecodeFont(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range [][2]byte{{0xa4, 0x40}, {0xa3, 0xbf}, {0xc9, 0x40}} {
		if g, ok := f.Big5(c[0], c[1]); !ok || g.Rows[0] != 0xff00 {
			t.Fatalf("%x: %v %v", c, g.Rows[0], ok)
		}
	}
	if i := Big5Index(0xc6, 0xa1); i != 0x34be {
		t.Fatal("C6A1", i)
	}
	if i := Big5Index('A', 'B'); i <= MaxGlyph {
		t.Fatal("single byte accepted", i)
	}
	g := f.Glyphs([]byte("A\xa4\x40"))
	if len(g) != 2 || g[0].Width != 8 || !g[0].Set(0, 0) || g[1].Width != 16 {
		t.Fatal(g)
	}
	if _, err := DecodeFont(raw[1:]); err == nil {
		t.Fatal("size accepted")
	}
}

func TestPackagedFont(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(os.Getenv("WONDERLAND_CLIENT_DATA"), "font", "TATPC1.TWN"))
	if err != nil {
		t.Skip("set WONDERLAND_CLIENT_DATA to the original client directory")
	}
	f, err := DecodeFont(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Row 3 of "L" as drawn on the server list (client/reference/server_select.png).
	if r := f.ASCII('L').Rows[3]; r != 0x7800 {
		t.Fatalf("L row 3 = %#04x", r)
	}
	// 一: a horizontal stroke with a small end serif.
	if g, _ := f.Big5(0xa4, 0x40); g.Rows[7] != 0xfffe || g.Rows[6] != 0x0004 {
		t.Fatal(g.Rows)
	}
}
