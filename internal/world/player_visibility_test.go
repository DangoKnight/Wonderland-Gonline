package world

import (
	"testing"
	"wonderland-go/internal/assets"
)

func TestStarterScenePlayerVisibility(t *testing.T) {
	maps := map[uint16]assets.Map{
		10001: {ID: 10001, Scene: 10001}, 10002: {ID: 10002, Scene: 10002}, 10003: {ID: 10003, Scene: 10003},
	}
	// Independent native content references: scene variants share the same image.
	for id := uint16(10011); id <= 10040; id++ {
		scene := uint16(10001)
		if id >= 10031 {
			scene = 10003
		} else if id >= 10021 {
			scene = 10002
		}
		maps[id] = assets.Map{ID: id, Scene: scene}
	}
	for _, id := range []uint16{11016, 11050, 11113, 11115, 60000} {
		maps[id] = assets.Map{ID: id, Scene: id}
	}
	w := New(&assets.Catalog{Maps: maps})
	for _, id := range []uint16{10001, 10002, 10003} {
		if !w.HideOtherPlayers(id) {
			t.Fatalf("starter base map %d exposes players", id)
		}
	}
	for id := uint16(10011); id <= 10040; id++ {
		if !w.HideOtherPlayers(id) {
			t.Fatalf("starter variant %d exposes players", id)
		}
	}
	for _, id := range []uint16{11016, 11050, 11113, 11115, 60000, 65535} {
		if w.HideOtherPlayers(id) {
			t.Fatalf("public or unknown map %d hides players", id)
		}
	}
}
