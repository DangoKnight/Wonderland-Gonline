package battle

import "testing"

// Copied from the 2026-11-08 Legacy-server captures, independent of server builders.
func TestCapturedSpectatorAndJoiningFormations(t *testing.T) {
	spectator := []byte{11, 250, 80, 44, 4, 2, 199, 117, 70, 0, 0, 0, 0, 0, 0, 0, 255, 255, 194, 1, 0, 0, 231, 0, 194, 1, 0, 0, 231, 0, 14, 3, 0, 0, 0, 0, 1, 2, 118, 117, 70, 0, 0, 0, 0, 0, 0, 0, 4, 2, 196, 1, 0, 0, 233, 0, 195, 1, 0, 0, 233, 0, 15, 3, 0, 0, 0, 0, 1, 4, 188, 97, 0, 0, 0, 0, 118, 117, 70, 0, 3, 2, 177, 2, 0, 0, 65, 1, 177, 2, 0, 0, 65, 1, 15, 3, 0, 0, 0, 0, 100, 7, 149, 66, 0, 0, 0, 0, 0, 0, 0, 0, 2, 2, 88, 0, 0, 0, 101, 0, 88, 0, 0, 0, 101, 0, 12, 4, 0, 0, 0, 0}
	joining := []byte{11, 250, 90, 44, 5, 2, 199, 117, 70, 0, 0, 0, 0, 0, 0, 0, 4, 3, 194, 1, 0, 0, 231, 0, 194, 1, 0, 0, 231, 0, 14, 3, 0, 0, 0, 0, 1, 2, 118, 117, 70, 0, 0, 0, 0, 0, 0, 0, 4, 2, 196, 1, 0, 0, 233, 0, 196, 1, 0, 0, 233, 0, 15, 3, 0, 0, 0, 0, 1, 4, 188, 97, 0, 0, 0, 0, 118, 117, 70, 0, 3, 2, 177, 2, 0, 0, 65, 1, 176, 2, 0, 0, 65, 1, 15, 3, 0, 0, 0, 0, 100, 7, 149, 66, 0, 0, 0, 0, 0, 0, 0, 0, 2, 2, 88, 0, 0, 0, 101, 0, 88, 0, 0, 0, 101, 0, 12, 4, 0, 0, 0, 0}
	pet := []byte{11, 5, 5, 4, 76, 55, 0, 0, 3, 0, 199, 117, 70, 0, 3, 3, 229, 0, 0, 0, 153, 0, 229, 0, 0, 0, 153, 0, 14, 1, 0, 0, 0, 0}
	s := &State{Self: 4617671}
	if h, v := s.Apply(spectator); !h || !v {
		t.Fatal("spectator formation rejected")
	}
	if !s.Spectating || len(s.Fighters) != 4 || !s.At(Cell{255, 255}).Removed {
		t.Fatal("invalid spectator roster", s)
	}
	s.Apply([]byte{52, 1})
	for i := range s.Fighters {
		if s.Owned(&s.Fighters[i]) || s.CanChoose(s.Fighters[i].Cell) {
			t.Fatal("spectator can act")
		}
	}
	if s.At(Cell{2, 2}).Side != Defender {
		t.Fatal("native mode 100 did not infer enemy side")
	}
	s.Apply([]byte{11, 0, 199, 117, 70, 0, 0, 0})
	if s.Active || s.Spectating {
		t.Fatal("spectator close retained scene")
	}
	if h, v := s.Apply(joining); !h || !v {
		t.Fatal("joining formation rejected")
	}
	if h, v := s.Apply(pet); !h || !v {
		t.Fatal("joining pet rejected")
	}
	s.Apply([]byte{50, 6, 4, 3, 0})
	s.Apply([]byte{52, 1})
	if s.Spectating || !s.CanChoose(Cell{4, 3}) || !s.CanChoose(Cell{3, 3}) || s.CanChoose(Cell{4, 2}) {
		t.Fatal("joining ownership incorrect", s)
	}
}

func TestSpectatorDoesNotConsumeGridCapacity(t *testing.T) {
	s := &State{Self: 17}
	observer := append([]byte(nil), playerWire...)
	observer[0], observer[12], observer[13] = 4, 255, 255
	p := append([]byte{11, 250, 80, 44}, observer...)
	for x := byte(1); x <= 4; x++ {
		for y := byte(1); y <= 4; y++ {
			r := append([]byte(nil), monsterWire...)
			r[2], r[12], r[13] = 100+x*4+y, x, y
			p = append(p, r...)
		}
	}
	if h, v := s.Apply(p); !h || !v || len(s.Fighters) != 17 || !s.Spectating {
		t.Fatal("full formation rejected spectator")
	}
}
