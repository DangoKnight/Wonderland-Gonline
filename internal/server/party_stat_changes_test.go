package server

import (
	"bytes"
	"testing"
)

func TestPartyStatChangesKeepNativeAbsoluteValues(t *testing.T) {
	old := [8]int64{7, 10, 4, 50, 30, 2, 3, 0}
	next := old
	next[3] = 42
	packets := teammateStatChanges(1, next, &old)
	// Native AC8:3: player 1, current HP 25, positive absolute 42, target 0.
	want := []byte{8, 3, 1, 0, 0, 0, 25, 1, 42, 0, 0, 0, 0, 0, 0, 0}
	if len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Fatal(packets)
	}
	if len(teammateStatChanges(1, next, &next)) != 0 {
		t.Fatal("unchanged stats resent")
	}
	if len(teammateStatChanges(1, next, nil)) != 8 {
		t.Fatal("initial snapshot incomplete")
	}
	next[5] = -5
	packets = teammateStatChanges(1, next, &old)
	if len(packets) != 2 || packets[1][7] != 2 {
		t.Fatal("negative stat sign lost", packets)
	}
}

func TestPartySnapshotDoesNotRebroadcastToOtherEntrants(t *testing.T) {
	s, cs, ws := worldFixture(t)
	p := &party{members: cs}
	for _, c := range cs {
		c.party = p
	}
	s.partySnapshot(cs[0])
	if len(ws[0].packets(t)) != 17 || ws[1].Len() != 0 || ws[2].Len() != 0 {
		t.Fatal("recipient refresh rebroadcast")
	}
	if cs[0].partyVitals != [8]int64{} {
		t.Fatal("recipient-only refresh marked unsent broadcasts current")
	}
	for _, w := range ws {
		w.Reset()
	}
	s.partyUpdate(p)
	for i, w := range ws {
		if len(w.packets(t)) != 17 {
			t.Fatal("full party snapshot missing fields", i)
		}
		if cs[i].partyVitals != s.vitals(cs[i]) {
			t.Fatal("broadcast cache not advanced")
		}
	}
}
