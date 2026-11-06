package game

import "testing"

func TestTentWallAndFloorClassification(t *testing.T) {
	if !IsTentWallpaper(39064, "") || !IsTentWallpaper(0, "Pink Wall") {
		t.Fatal("wallpaper not recognized by id or name")
	}
	if IsTentWallpaper(0, "Wall Lamp") {
		t.Fatal("wall lamp must stay furniture")
	}
	if !IsTentFloor(39062, "") || !IsTentFloor(0, "Wood Floor") {
		t.Fatal("floor not recognized by id or name")
	}
	if IsTentFloor(0, "Floor Gift Box") {
		t.Fatal("gift must stay furniture")
	}
}

func TestTentFootprintSwapsOnQuarterTurns(t *testing.T) {
	def := ItemDefinition{CellWidth: 3, CellHeight: 1}
	if w, h := TentFootprint(def, 0); w != 3 || h != 1 {
		t.Fatalf("rotation 0 = %dx%d", w, h)
	}
	if w, h := TentFootprint(def, 1); w != 1 || h != 3 {
		t.Fatalf("rotation 1 = %dx%d", w, h)
	}
	if w, h := TentFootprint(ItemDefinition{}, 2); w != 1 || h != 1 {
		t.Fatalf("missing size = %dx%d", w, h)
	}
}

func TestTentFloorBoundsWidenByType(t *testing.T) {
	if !TentInFloorBounds(29, 38, 1, false) || !TentInFloorBounds(44, 50, 1, false) {
		t.Fatal("base bounds rejected corners")
	}
	if TentInFloorBounds(45, 42, 1, false) || TentInFloorBounds(40, 33, 1, false) {
		t.Fatal("base bounds accepted outside cells")
	}
	if !TentInFloorBounds(40, 33, 2, false) || !TentInFloorBounds(40, 28, 3, false) || !TentInFloorBounds(40, 33, 1, true) {
		t.Fatal("enlarged or higher-type bounds not widened")
	}
}

func TestTentFreeSpotPrefersCenterThenScans(t *testing.T) {
	if x, y := TentFreeSpot(nil, 1, 1, 1, false); x != 42 || y != 42 {
		t.Fatalf("empty floor = (%d,%d)", x, y)
	}
	occupied := []TentRect{{X: 42, Y: 42, W: 1, H: 1}}
	if x, y := TentFreeSpot(occupied, 1, 1, 1, false); x == 42 && y == 42 {
		t.Fatal("occupied center reused")
	}
	other := []TentRect{{X: 42, Y: 42, W: 1, H: 1, Floor: 1}}
	if x, y := TentFreeSpot(other, 1, 1, 1, false); x != 42 || y != 42 {
		t.Fatal("other floor blocked the center")
	}
}
