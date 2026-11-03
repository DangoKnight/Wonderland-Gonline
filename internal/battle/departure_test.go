package battle

import (
	"bytes"
	"testing"
)

func TestDepartureAnnouncesWinningSide(t *testing.T) {
	for _, pvp := range []bool{false, true} {
		b := &Battle{PvP: pvp}
		if got := b.Departure(true)[0]; !bytes.Equal(got, []byte{11, 12, 1}) {
			t.Fatal("attacker win", got)
		}
		if got := b.Departure(false)[0]; !bytes.Equal(got, []byte{11, 12, 2}) {
			t.Fatal("defender win", got)
		}
	}
}
