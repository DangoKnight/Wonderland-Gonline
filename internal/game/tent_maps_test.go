package game

import "testing"

func TestTentMapIDs(t *testing.T) {
	if !IsTentMap(TentMapID) || !IsTentMap(TentUpperMapID) {
		t.Fatal("tent maps not recognised")
	}
	for _, id := range []uint16{0, 12000, TentMapID - 1, TentUpperMapID + 1} {
		if IsTentMap(id) {
			t.Fatalf("map %d treated as a tent interior", id)
		}
	}
	if TentFloor(TentMapID) != 0 || TentFloor(TentUpperMapID) != 1 {
		t.Fatalf("unexpected levels: %d %d", TentFloor(TentMapID), TentFloor(TentUpperMapID))
	}
	if TentFloor(12000) != 0 {
		t.Fatal("world map resolved to an upper level")
	}
}
