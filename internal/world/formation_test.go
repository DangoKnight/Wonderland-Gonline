package world

import (
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"
)

func TestNativeFormationPreservesSlotByte(t *testing.T) {
	// Independent raw Team1 records from 11090:3; ID UInt16, slot UInt8.
	raw := []byte{16, 67, 5, 16, 67, 5, 17, 67, 2, 17, 67, 2, 18, 67, 7, 18, 67, 7, 19, 67, 8}
	w := New(&assets.Catalog{Maps: map[uint16]assets.Map{11090: {ID: 11090, Battles: []assets.BattleEntry{{ClickID: 3, Team1: raw}}}}})
	want := []FormationMember{{17168, 5}, {17168, 5}, {17169, 2}, {17169, 2}, {17170, 7}, {17170, 7}, {17171, 8}}
	if got := w.FormationMembers(11090, 3); !reflect.DeepEqual(got, want) {
		t.Fatal("native formation lost queue slots", got)
	}
	if got := w.Formation(11090, 3); !reflect.DeepEqual(got, []uint32{17168, 17168, 17169, 17169, 17170, 17170, 17171}) {
		t.Fatal("legacy template API changed", got)
	}
}
