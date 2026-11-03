package app

import (
	"bytes"
	"os"
	"testing"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestLegacyAcquisitionAndFlightInBattleTransports(t *testing.T) {
	for _, chain := range []bool{false, true} {
		c, now, _ := enterBattle(t)
		c.InventoryState.Bag = game.Inventory{}
		gain := []byte{23, 6, 238, 179, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		flight := []byte{53, 4, 238, 179, 2, 2, 4, 2, 3, 2}
		if chain {
			c.combat.enabled = true
			deliverCombatRound(t, c, 3, []protocol.CombatStep{{Packets: [][]byte{gain, flight}}})
			c.battleFrame(false)
		} else {
			c.dispatch(gain)
			c.dispatch(flight)
		}
		if c.InventoryState.Bag[0].ID != 46062 || c.InventoryState.Bag[0].Count != 1 || len(c.itemFlights) != 2 {
			t.Fatal("native acquisition/flight rejected", chain, c.InventoryState.Bag, len(c.itemFlights))
		}
		if path := os.Getenv("BATTLE_ACQUISITION_SNAPSHOT"); path != "" && !chain {
			*now = now.Add(300 * time.Millisecond)
			c.battleFrame(true)
			savePNG(t, path, c)
		}
	}
}

func TestLegacyFloorDropPickupAndDestroyPrompt(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	c.World.Player.X, c.World.Player.Y = 2750, 2513
	c.dispatch([]byte{23, 3, 238, 179, 190, 10, 209, 9, 0, 0, 0, 0, 1})
	if len(c.floorItems) != 1 || c.floorItems[1].id != 46062 {
		t.Fatal("native floor spawn")
	}
	point := c.floorItemPoint(c.floorItems[1])
	if !c.pickupFloorItem(point.X, point.Y-10) {
		t.Fatal("floor item not clickable")
	}
	if packets := sent(); len(packets) != 1 || !bytes.Equal(packets[0], []byte{23, 2, 1, 0}) {
		t.Fatal("pickup request", packets)
	}
	c.dispatch([]byte{23, 2, 1, 0, 1})
	if len(c.floorItems) != 0 {
		t.Fatal("floor pickup retained sprite")
	}
	c.InventoryState.Bag[25] = game.Item{ID: 32280, Count: 1}
	c.dispatch([]byte{23, 212, 7, 26, 24, 126, 1})
	if c.UI.Modal == nil || c.InventoryState.Bag[25].Count != 1 {
		t.Fatal("native destroy prompt absent or removed item early")
	}
}
