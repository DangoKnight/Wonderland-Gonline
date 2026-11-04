package app

import (
	"os"
	"testing"

	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestVehicleStatusNativeProgressAndRobinsonHealth(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.World.Player.ID = 4617590
	c.InventoryState.Bag[11] = game.Item{ID: 48016, Count: 1, Damage: 50}
	c.inventoryPacket([]byte{23, 5})
	c.TeamState.BattlePet = 12032
	c.remote.activePet = 12032
	c.World.Companions = map[uint32]*world.Companion{4617590: {ID: 12032}}
	c.dispatch([]byte{15, 10, 12, 118, 117, 70, 0, 144, 187})
	if c.TeamState.BattlePet != 12032 || c.TeamState.MountPet != 0 || c.remote.activePet != 12032 || len(c.World.Companions) != 1 {
		t.Fatal("boarding changed battle selection or discarded companion")
	}
	// Raw captured progress reply. Robinson's health stays full even with wear.
	c.dispatch([]byte{15, 14, 12, 118, 117, 70, 0, 224, 0, 0, 0, 0, 0})
	i, ok := c.vehicleStatusInfo()
	if !ok || i.Maximum != 14400 || i.Health != i.Maximum || c.InventoryState.Bag[11].VehicleWear() != 224 {
		t.Fatal("Robinson raft health/progress", i)
	}
	c.Frame()
	if !c.MainStatus.Vehicle.Visible || c.MainStatus.Pet.Visible {
		t.Fatal("vehicle/pet HUD visibility")
	}
	if out := os.Getenv("VEHICLE_STATUS_SNAPSHOT"); out != "" {
		savePNG(t, out, c)
	}
	// Authored normal-raft progress: half health, quarter fuel.
	c.World.Player.VehicleID = 48010
	item := c.items[48010]
	item.Record[45] = 5
	c.items[48010] = item
	c.InventoryState.Bag[11] = game.Item{ID: 48010, Count: 1}
	c.dispatch([]byte{15, 14, 12, 118, 117, 70, 0, 32, 28, 0, 0, 16, 14})
	i, ok = c.vehicleStatusInfo()
	if !ok || i.Health != 7200 || i.Fuel != 3600 || i.Maximum != 14400 {
		t.Fatal("normal vehicle progress", i)
	}
	before := c.InventoryState.Bag
	c.dispatch([]byte{15, 14, 11, 118, 117, 70, 0, 0, 0, 0, 0, 0, 0})
	c.dispatch([]byte{15, 14, 12, 119, 117, 70, 0, 0, 0, 0, 0, 0, 0})
	c.dispatch([]byte{15, 14, 12})
	if c.InventoryState.Bag != before {
		t.Fatal("foreign, stale or truncated progress applied")
	}
	c.dispatch([]byte{15, 11, 12, 118, 117, 70, 0})
	c.Frame()
	if c.MainStatus.Vehicle.Visible {
		t.Fatal("dismount left vehicle HUD")
	}
}

func TestVehicleRidingAllowsBattlePetSelection(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.World.Player.VehicleID, c.World.Player.VehicleSlot = 48016, 1
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 12032, Amity: 80, Stats: world.Stats{HP: 100}}
	c.Team.Show()
	capture := wire(t, c)
	c.Team.SetMode(1, 1)
	c.Team.SetMode(1, 2)
	if len(capture()) != 1 || capture()[0][0] != protocol.CommandBattlePet {
		t.Fatal("Team blocked battle selection or allowed pet riding", capture())
	}
	c.TeamState.BattlePet = 12032
	c.Team.SetMode(1, 0)
	if len(capture()) != 2 || len(capture()[1]) != 2 || capture()[1][0] != 19 || capture()[1][1] != 2 {
		t.Fatal("Team blocked resting while riding", capture())
	}
	// The pet may be selected after dismount.
	c.World.Player.VehicleID, c.World.Player.VehicleSlot = 0, 0
	c.Team.SetMode(1, 1)
	packets := capture()
	if len(packets) != 3 || packets[2][0] != protocol.CommandBattlePet {
		t.Fatal("pet selection remained blocked after dismount", packets)
	}
}

func TestInventoryVehicleConditionUpdates(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.World.Player.VehicleID, c.World.Player.VehicleSlot = 48010, 1
	i := c.items[48010]
	i.Record[45] = 5
	c.items[48010] = i
	c.InventoryState.Bag = game.Inventory{{ID: 48010, Count: 1}}
	c.inventoryPacket([]byte{23, 5})
	c.Inventory.Show()
	c.Frame()
	a := c.Inventory.Slots[0].Abs()
	pixel := (a.Y+7)*c.Screen.W + a.X + 30
	full := c.Screen.Pix[pixel]
	c.dispatch(protocol.Builder{15, 14, 1}.U32(c.World.Player.ID).U32(7200).U16(3600))
	c.Frame()
	if c.Screen.Pix[pixel] == full {
		t.Fatal("inventory health bar did not update with progress reply")
	}
	condition, ok := c.vehicleStatusInfo()
	if !ok || condition.Health != 7200 || condition.Fuel != 3600 {
		t.Fatal("inventory and mounted HUD progress differ", condition)
	}
	if out := os.Getenv("INVENTORY_VEHICLE_SNAPSHOT"); out != "" {
		savePNG(t, out, c)
	}
	// Inventory still shows the item's saved condition after dismount.
	remaining := c.Screen.Pix[pixel]
	c.dispatch(protocol.Builder{15, 11, 1}.U32(c.World.Player.ID))
	c.Frame()
	if c.Screen.Pix[pixel] != remaining {
		t.Fatal("dismount reset inventory condition")
	}
}
