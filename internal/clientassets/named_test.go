package clientassets

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNamedArchiveGround reads the installed Ground.MMG: 1,147 records,
// and 10017.map is the 2432×1792 Ship Deck with one background layer.
func TestNamedArchiveGround(t *testing.T) {
	path := filepath.Join(os.Getenv("WONDERLAND_CLIENT_DATA"), "data", "Ground.MMG")
	f, err := os.Open(path)
	if err != nil {
		t.Skip("set WONDERLAND_CLIENT_DATA to the original client directory")
	}
	defer f.Close()
	st, _ := f.Stat()
	a, err := ReadNamedArchive(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Entries) != 1147 {
		t.Fatalf("%d entries", len(a.Entries))
	}
	i, ok := a.Find("10017.MAP")
	if !ok {
		t.Fatal("10017.map missing")
	}
	b, err := a.Read(i)
	if err != nil {
		t.Fatal(err)
	}
	g, err := DecodeGroundPrefix(b)
	if err != nil {
		t.Fatal(err)
	}
	if g.Width != 2432 || g.Height != 1792 || len(g.Layers) != 1 || g.Layers[0].Resource != 10001 {
		t.Fatalf("%+v", g.Layers)
	}
}
