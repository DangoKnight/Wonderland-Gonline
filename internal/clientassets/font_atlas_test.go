package clientassets

import (
	"os"
	"path/filepath"
	"testing"
)

// This compares every glyph against the independently decoded original file.
func TestSourceFontAtlas(t *testing.T) {
	media := os.Getenv("WONDERLAND_TEST_CLIENT_MEDIA")
	original := os.Getenv("WONDERLAND_TEST_CLIENT_FONT")
	if media == "" || original == "" {
		t.Skip("set original font and exported media paths")
	}
	atlas, err := LoadFontAtlas(filepath.Join(media, "font", "TATPC1_TWN"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	native, err := DecodeFont(raw)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < fontAtlasASCIICharacters; i++ {
		if atlas.ASCII(byte(i)) != native.ASCII(byte(i)) {
			t.Fatalf("ASCII glyph %d differs", i)
		}
	}
	for i := 0; i < fontBig5Count; i++ {
		if atlas.Wide(i) != native.Wide(i) {
			t.Fatalf("wide glyph %d differs", i)
		}
	}
}
