package role

import "testing"

// TestColorsSet checks FUN_00444e28's digit layout for the two values the
// selection screen applies (groups 1 and 5).
func TestColorsSet(t *testing.T) {
	c := NeutralColors()
	c.Set(123456789, allParts, 1)
	c.Set(987654321, allParts, 5)
	want := map[int]byte{0: 1, 1: 2, 2: 3, 3: 4, 4: 5, 5: 6, 6: 7, 7: 8, 8: 9,
		9: 9, 10: 8, 11: 7, 0x27: 6, 0x28: 5, 0x29: 4, 0x2a: 3, 0x2b: 2, 0x2c: 1}
	for i, d := range c {
		w, ok := want[i]
		if !ok {
			w = neutralDigit
		}
		if d != w {
			t.Fatalf("digit %d = %d, want %d", i, d, w)
		}
	}
	// A single-group update leaves the rest alone.
	c = NeutralColors()
	c.Set(111222333, 2, 2)
	if c[9] != neutralDigit || c[0xc] != 2 || c[0xf] != 3 {
		t.Fatalf("group 2 only: %v", c)
	}
}

// TestColorsPalette checks FUN_0030105c's shift and clamp on one range and
// the uncoloured sprite IDs.
func TestColorsPalette(t *testing.T) {
	var src [256][3]uint8
	src[0x10] = [3]uint8{100, 0, 255}
	src[0x05] = [3]uint8{100, 0, 255}
	c := NeutralColors()
	c[0], c[1], c[2] = 8, 0, 4 // +100 red, -100 green, blue unchanged
	p := c.palette(1000, &src)
	if p[0x10] != [3]uint8{200, channelMin, channelMax} {
		t.Fatalf("shifted %v", p[0x10])
	}
	if p[0x05] != src[0x05] {
		t.Fatal("entries below 0x10 are not shifted")
	}
	if p := c.palette(uncoloredSprite2, &src); p[0x10] != [3]uint8{100, channelMin, channelMax} {
		t.Fatalf("uncoloured sprite %v", p[0x10])
	}
	if colorRanges[12].lo != 0xd0 || colorRanges[12].digit != 0x27 || colorRanges[11].digit != 33 {
		t.Fatal("range table")
	}
}
