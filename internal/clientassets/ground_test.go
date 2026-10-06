package clientassets

import (
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/protocol"
)

func TestGroundPrefixBoundsAndOrder(t *testing.T) {
	b := protocol.Builder{}.U32(40).U32(60).U8(1).U16(10000).U16(7).U16(8).U16(2).U16(3).Bytes([]byte{1, 2, 3, 4, 5, 6})
	g, err := DecodeGroundPrefix(append(b, 99, 100))
	if err != nil || g.BytesRead != len(b) || g.Layers[0] != (GroundLayer{10000, 7, 8}) {
		t.Fatal(g, err)
	}
	if v, ok := g.Cell(1, 0); !ok || v != 4 {
		t.Fatal("grid order", v, ok)
	}
	if _, ok := g.Cell(-1, 0); ok {
		t.Fatal("negative cell")
	}
	b[len(b)-1] = 77
	if v, _ := g.Cell(1, 2); v != 6 {
		t.Fatal("decoder aliases input")
	}
	for i := 0; i < len(b); i++ {
		if _, err := DecodeGroundPrefix(b[:i]); err == nil {
			t.Fatal("accepted truncated prefix", i)
		}
	}
	bad := protocol.Builder{}.U32(40).U32(60).U8(0).U16(65535).U16(65535)
	if _, err := DecodeGroundPrefix(bad); err == nil {
		t.Fatal("unbounded grid")
	}
}

func TestNativeGroundFirstRecord(t *testing.T) {
	dir := os.Getenv("WONDERLAND_TEST_DATA")
	if dir == "" {
		t.Skip("set WONDERLAND_TEST_DATA")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join("../..", dir)
	}
	b, err := os.ReadFile(filepath.Join(dir, "Ground.MMG"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := DecodeGroundPrefix(b)
	if err != nil {
		t.Fatal(err)
	}
	if g.Width != 1472 || g.Height != 960 || g.GridWidth != 74 || g.GridHeight != 49 || len(g.Layers) != 1 || g.Layers[0].Resource != 10000 || g.BytesRead != 3645 {
		t.Fatal(g.Width, g.Height, g.GridWidth, g.GridHeight, g.Layers, g.BytesRead)
	}
	// No map ID is inferred from this anonymous first record.
}
