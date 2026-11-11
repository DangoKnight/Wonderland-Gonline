package protocol

import (
	"bytes"
	"reflect"
	"testing"
)

func TestAFKStateGoldenAndBounds(t *testing.T) {
	want := []byte{253, 2, 1, 0, 2, 0, 3, 0, 1, 4, 0, 0, 0, 4, 0, 0, 0, 0, 1, 1, 65, 5, 0, 0, 0, 6, 0, 0, 0, 7, 0, 0, 0, 8, 0, 0, 0, 9, 0, 0, 0, 10, 0, 0, 0, 0, 0, 0, 0, 0}
	s := AFKState{Map: 1, X: 2, Y: 3, Units: []AFKUnit{{Owner: 4, ID: 4, Name: "A", Level: 1, HP: 5, MaxHP: 6, SP: 7, MaxSP: 8, EXP: 9, EXPNext: 10}}}
	p, err := EncodeAFKState(s)
	if err != nil || !bytes.Equal(p, want) {
		t.Fatalf("golden mismatch: %v %v", p, err)
	}
	got, err := DecodeAFKState(want)
	if err != nil || !reflect.DeepEqual(got, s) {
		t.Fatalf("decode: %#v %v", got, err)
	}
	for i := range want {
		if _, err := DecodeAFKState(want[:i]); err == nil {
			t.Fatalf("accepted truncation %d", i)
		}
	}
	for _, p := range [][]byte{append(append([]byte{}, want...), 0), {253, 2, 0, 0, 0, 0, 0, 0, 21}} {
		if _, err := DecodeAFKState(p); err == nil {
			t.Fatal("accepted invalid snapshot")
		}
	}
	s.Units = make([]AFKUnit, AFKMaxUnits+1)
	if _, err := EncodeAFKState(s); err == nil {
		t.Fatal("accepted unbounded units")
	}
}

func TestAFKOnlyDropsExplicitNonessentialTraffic(t *testing.T) {
	// Independent native bytes: self 1, party 2, unrelated player 3.
	for _, p := range [][]byte{{4, 3, 0, 0, 0}, {7, 3, 0, 0, 0}, {6, 1, 3, 0, 0, 0, 0, 1, 0, 2, 0}, {32, 2, 3, 0, 0, 0, 1}, {5, 0, 3, 0, 0, 0}, {2, 1, 3, 0, 0, 0, 65}, {2, 5, 3, 0, 0, 0, 65}} {
		if !AFKNonessential(p, 1, []uint32{2}) {
			t.Fatal("did not suppress", p)
		}
	}
	for _, p := range [][]byte{{4, 1, 0, 0, 0}, {4, 2, 0, 0, 0}, {7, 2, 0, 0, 0}, {6, 2, 0}, {2, 0, 65}, {2, 4, 65}, {2, 16, 65}, {8, 3, 2, 0, 0, 0}, {20, 1}, {22, 11}, {23, 2}, {50, 1}, {51, 1}, {52, 1}, {53, 1}, {254, 1}, {253, 2}, {255, 1}, {}} {
		if AFKNonessential(p, 1, []uint32{2}) {
			t.Fatal("dropped vital/unknown packet", p)
		}
	}
}
