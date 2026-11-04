package assets

import "testing"

func TestGroundItemSignedDrawDepth(t *testing.T) {
	for _, c := range []struct {
		raw  [2]uint16
		want int32
	}{
		{[2]uint16{160, 0}, 160}, {[2]uint16{170, 0}, 170}, {[2]uint16{65512, 65535}, -24}, {[2]uint16{}, 0},
	} {
		if got := (GroundItem{Unknown: c.raw}).DrawDepth(); got != c.want {
			t.Fatalf("%v: got %d want %d", c.raw, got, c.want)
		}
	}
}
