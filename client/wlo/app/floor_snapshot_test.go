package app

import "testing"

func TestFloorItemsResetAcrossEmptyMapAndReturn(t *testing.T) {
	c, _, _ := enteredClient(t)
	_ = wire(t, c)
	c.dispatch([]byte{23, 4, 3, 1, 0, 106, 160, 0, 0, 174, 5, 103, 3, 160, 0, 0, 0})
	if len(c.floorItems) != 1 {
		t.Fatal("missing initial coconut")
	}
	// No ground packets arrive on the intervening map. Previously floorMap
	// stayed on the original map and retained that visit's collectible objects.
	c.dispatch([]byte{12, 17, 39, 0, 0, 43, 39, 162, 2, 43, 4})
	if len(c.floorItems) != 0 {
		t.Fatal("items survived leaving their map")
	}
	c.dispatch([]byte{12, 17, 39, 0, 0, 33, 39, 18, 4, 51, 4})
	c.dispatch([]byte{23, 4})
	if len(c.floorItems) != 0 {
		t.Fatal("collected coconut reappeared on return")
	}
	// A subsequent one-node respawn remains an incremental update.
	c.dispatch([]byte{23, 4, 3, 1, 0, 106, 160, 0, 0, 174, 5, 103, 3, 160, 0, 0, 0})
	c.dispatch([]byte{23, 4, 3, 2, 0, 106, 160, 0, 0, 206, 1, 26, 2, 170, 0, 0, 0})
	if len(c.floorItems) != 2 {
		t.Fatal("respawn discarded another available item")
	}
	c.floorHovered, c.floorPendingPickup = 1, 1
	c.dispatch([]byte{23, 4})
	if len(c.floorItems) != 0 || c.floorHovered != 0 || c.floorPendingPickup != 0 {
		t.Fatal("empty snapshot retained item interactions")
	}
}

func TestFloorItemsResetOnSameMapReload(t *testing.T) {
	c, _, _ := enteredClient(t)
	_ = wire(t, c)
	c.dispatch([]byte{23, 3, 106, 160, 177, 5, 117, 3, 0, 0, 0, 0, 1})
	c.dispatch([]byte{12, 17, 39, 0, 0, 33, 39, 18, 4, 51, 4})
	if len(c.floorItems) != 0 {
		t.Fatal("same-map arrival retained previous snapshot")
	}
}
