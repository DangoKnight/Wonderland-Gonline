package protocol

import (
	"bytes"
	"testing"
)

func TestBattleBackdropWireGoldens(t *testing.T) {
	for _, sub := range []byte{244, 245, 246} {
		want := []byte{1, sub, 71, 76, 66, 68, 1, 0}
		if !bytes.Equal(BattleBackdropCapability(sub), want) || !IsBattleBackdropCapability(want, sub) {
			t.Fatal(want)
		}
		for _, bad := range [][]byte{want[:7], {1, sub, 1}, {1, sub, 71, 76, 66, 68, 2, 0}} {
			if IsBattleBackdropCapability(bad, sub) {
				t.Fatal("accepted malformed capability", bad)
			}
		}
	}
	want := []byte{1, 247, 71, 76, 66, 68, 1, 0, 97, 234, 154, 8, 115, 5}
	l := BattleBackdropLocation{60001, 2202, 1395}
	if !bytes.Equal(l.Packet(), want) {
		t.Fatal(l.Packet())
	}
	if got, ok := ParseBattleBackdropLocation(want); !ok || got != l {
		t.Fatal(got, ok)
	}
	for i := range len(want) {
		if _, ok := ParseBattleBackdropLocation(want[:i]); ok {
			t.Fatal("accepted truncated location", i)
		}
	}
}
