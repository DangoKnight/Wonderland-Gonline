package clientassets

import "testing"

func TestGroundBackdropsRawBytes(t *testing.T) {
	// Independently authored wire bytes: 20x20, one grid cell, one sound
	// zone and one object, default 80, region 91 at (10,20)-(300,400).
	raw := []byte{
		20, 0, 0, 0, 20, 0, 0, 0, 0, 1, 0, 1, 0, 0,
		1, 0, 7, 0, 8, 0, 9, 0,
		1, 0, 1, 0, 0, 0, 2, 0, 3, 0,
		100, 80, 1, 91, 10, 0, 20, 0, 44, 1, 144, 1,
	}
	b, err := DecodeGroundBackdrops(raw)
	if err != nil || b.Default != 80 || len(b.Regions) != 1 || b.Regions[0] != (GroundBackdropRegion{91, 10, 20, 300, 400}) {
		t.Fatal(b, err)
	}
	for i := range len(raw) {
		if _, err := DecodeGroundBackdrops(raw[:i]); err == nil {
			t.Fatalf("accepted truncated byte %d", i)
		}
	}
	bad := append([]byte(nil), raw...)
	bad[len(bad)-8], bad[len(bad)-7] = 255, 255
	if _, err := DecodeGroundBackdrops(bad); err == nil {
		t.Fatal("accepted inverted rectangle")
	}
}
