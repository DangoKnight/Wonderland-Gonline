package protocol

import (
	"bytes"
	"reflect"
	"testing"
)

func TestAFKDeltaGoldenAtomicAndBounds(t *testing.T) {
	base := AFKState{Map: 1, Units: []AFKUnit{{Owner: 4, ID: 4, Name: "A", Level: 1, HP: 5, MaxHP: 10, SP: 6, MaxSP: 11}}}
	next := base
	next.Units = append([]AFKUnit(nil), base.Units...)
	next.Units[0].HP = 3
	want := []byte{253, 8, 0, 1, 4, 0, 0, 0, 4, 0, 0, 0, 0, 4, 0, 3, 0, 0, 0}
	p, err := EncodeAFKDelta(base, next)
	if err != nil || !bytes.Equal(p, want) {
		t.Fatal(p, err)
	}
	got, err := ApplyAFKDelta(base, want)
	if err != nil || !reflect.DeepEqual(got, next) || base.Units[0].HP != 5 {
		t.Fatal(got, base, err)
	}
	for i := range want {
		if _, err := ApplyAFKDelta(base, want[:i]); err == nil {
			t.Fatal("truncation", i)
		}
	}
	for _, p := range [][]byte{append(append([]byte{}, want...), 0), {253, 8, 2, 0}, {253, 8, 0, 41}, {253, 8, 0, 1, 4, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0}} {
		if _, err := ApplyAFKDelta(base, p); err == nil {
			t.Fatal("malformed accepted", p)
		}
	}
	if _, err := ApplyAFKDelta(AFKState{}, want); err == nil {
		t.Fatal("partial update without base")
	}
	raw, _ := EncodeAFKState(base)
	full, _ := EncodeAFKState(next)
	if !bytes.Equal(CompactAFKUpdate(raw, full), want) {
		t.Fatal("delta not chosen")
	}
	if !bytes.Equal(CompactAFKUpdate(nil, full), full) {
		t.Fatal("initial snapshot compressed")
	}
}

func TestAFKDeltaRosterAndMetadata(t *testing.T) {
	before := AFKState{Map: 1, Units: []AFKUnit{{Owner: 1, ID: 1, Name: "old"}, {Owner: 1, ID: 2, Kind: 1}}}
	after := AFKState{Map: 2, X: 3, Y: 4, Units: []AFKUnit{{Owner: 1, ID: 1, Name: "renamed", Level: 2, HP: 3, MaxHP: 4, SP: 5, MaxSP: 6, EXP: 7, EXPNext: 8, Amity: 9}, {Owner: 2, ID: 2, Name: "new", Kind: 1}}}
	p, err := EncodeAFKDelta(before, after)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ApplyAFKDelta(before, p)
	if err != nil || !reflect.DeepEqual(got, after) {
		t.Fatal(got, err)
	}
	// Leader/roster reordering requires a full snapshot; preserve authored order.
	reordered := before
	reordered.Units = []AFKUnit{before.Units[1], before.Units[0]}
	old, _ := EncodeAFKState(before)
	full, _ := EncodeAFKState(reordered)
	if !bytes.Equal(CompactAFKUpdate(old, full), full) {
		t.Fatal("roster ordering lost")
	}
	// Duplicate changes cannot rewrite or delete a unit twice.
	dup := append(append([]byte{}, p...), p[10:]...)
	dup[9] *= 2
	if _, err := ApplyAFKDelta(before, dup); err == nil {
		t.Fatal("duplicate changes accepted")
	}
}
