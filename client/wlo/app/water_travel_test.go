package app

import (
	"bytes"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
)

func shoreClient(waterColumn int) (*Client, *time.Time, *[][]byte) {
	now := time.Unix(0, 0)
	cells := make([]byte, 100)
	for x := waterColumn; x < 10; x++ {
		for y := 0; y < 10; y++ {
			cells[x*10+y] = 2
		}
	}
	item := assets.NativeItem{Definition: game.ItemDefinition{ID: 48016, Type: 39}, Sprites: [4]uint16{6005, 6005, 6005, 6005}}
	items := map[uint16]assets.NativeItem{48016: item}
	c := &Client{World: &world.World{Player: world.Player{ID: 101, Map: 10003, X: 22, Y: 55}, Scene: &world.Scene{Ground: clientassets.GroundPrefix{GridWidth: 10, GridHeight: 10, Cells: cells}}}, items: items}
	c.Now = func() time.Time { return now }
	c.InventoryState = &inventory.State{Items: items}
	c.InventoryState.Bag[0] = game.Item{ID: 48016, Count: 1}
	var sent [][]byte
	c.Inventory = &inventory.Form{Send: func(p []byte) { sent = append(sent, append([]byte(nil), p...)) }}
	return c, &now, &sent
}

func TestWaterBoardingWaitsForNativeReceipt(t *testing.T) {
	c, now, sent := shoreClient(2)
	c.walkWaterAware(82, 55, *now)
	if len(*sent) != 1 || !bytes.Equal((*sent)[0], []byte{15, 14, 1, 0x90, 0xbb}) {
		t.Fatal("wrong boarding request", *sent)
	}
	if c.World.Walking() || c.World.Scene.WaterTravel || c.World.Player.VehicleID != 0 {
		t.Fatal("optimistic water travel")
	}
	c.walkWaterAware(82, 55, *now)
	if len(*sent) != 1 {
		t.Fatal("duplicate boarding request")
	}
	c.vehiclePacket([]byte{15, 18, 1, 102, 0, 0, 0, 0x90, 0xbb, 22, 0, 0, 0, 55, 0, 0, 0})
	c.vehiclePacket([]byte{15, 18, 1, 101, 0, 0, 0, 0x90, 0xbb})
	if len(*sent) != 1 {
		t.Fatal("foreign or malformed placement boarded", *sent)
	}
	c.vehiclePacket([]byte{15, 18, 1, 101, 0, 0, 0, 0x90, 0xbb, 22, 0, 0, 0, 55, 0, 0, 0})
	if len(*sent) != 2 || !bytes.Equal((*sent)[1], []byte{15, 7, 1, 0x90, 0xbb}) || !c.World.Scene.Water(c.World.Player.X, c.World.Player.Y) || c.World.Walking() {
		t.Fatal("placement did not relocate and confirm boarding", *sent)
	}
	c.vehiclePacket([]byte{15, 18, 1, 101, 0, 0, 0, 0x90, 0xbb, 22, 0, 0, 0, 55, 0, 0, 0})
	if len(*sent) != 2 {
		t.Fatal("duplicate placement replayed boarding")
	}
	c.vehiclePacket([]byte{15, 10, 1, 102, 0, 0, 0, 0x90, 0xbb})
	c.vehiclePacket([]byte{15, 10, 1, 101, 0, 0, 0})
	if c.World.Scene.WaterTravel || c.waterTravel.requestID == 0 {
		t.Fatal("foreign or malformed reply accepted")
	}
	c.vehiclePacket([]byte{15, 10, 1, 101, 0, 0, 0, 0x90, 0xbb})
	if !c.World.Scene.WaterTravel || !c.World.Walking() || c.World.Player.VehicleID != 48016 {
		t.Fatal("confirmed boarding did not resume water walk")
	}
	*now = now.Add(time.Second)
	c.World.Step(*now)
	c.waterTravelTick()
	if !c.World.Scene.Water(c.World.Player.X, c.World.Player.Y) {
		t.Fatal("raft did not reach water")
	}
	c.walkWaterAware(22, 55, *now)
	*now = now.Add(time.Second)
	c.World.Step(*now)
	c.waterTravelTick()
	if len(*sent) != 4 || (*sent)[2][0] != 6 || (*sent)[2][1] != 2 || !bytes.Equal((*sent)[3], []byte{15, 10, 1, 0x90, 0xbb}) {
		t.Fatal("landing did not request dismount", *sent)
	}
	if !c.World.Scene.Land(c.World.Player.X, c.World.Player.Y) {
		t.Fatal("landing did not relocate to land")
	}
	if c.World.Player.VehicleID != 48016 {
		t.Fatal("optimistic dismount")
	}
	c.vehiclePacket([]byte{15, 11, 1, 101, 0, 0, 0})
	if c.World.Player.VehicleID != 0 || c.World.Scene.WaterTravel {
		t.Fatal("dismount not applied")
	}
	if c.InventoryState.Bag[0].Count != 1 {
		t.Fatal("client consumed disposable raft without inventory receipt")
	}
}

func TestWaterBoardingApproachesShoreAndValidatesBag(t *testing.T) {
	c, now, sent := shoreClient(5)
	c.walkWaterAware(142, 55, *now)
	if !c.World.Walking() || len(*sent) != 0 {
		t.Fatal("boarded away from shoreline")
	}
	*now = now.Add(time.Second)
	c.World.Step(*now)
	c.waterTravelTick()
	if len(*sent) != 1 || c.World.Scene.WaterTravel {
		t.Fatal("shore arrival did not request boarding")
	}
	for _, bad := range []game.Item{{ID: 48016, Count: 1, Locked: true}, {ID: 48016, Count: 1, Damage: 100}, {ID: 48016}, {ID: 48001, Count: 1}} {
		c, now, sent = shoreClient(2)
		c.InventoryState.Bag[0] = bad
		c.walkWaterAware(82, 55, *now)
		if len(*sent) != 0 || c.World.Scene.WaterTravel {
			t.Fatal("invalid water vehicle used", bad)
		}
	}
	c, now, sent = shoreClient(2)
	c.InventoryState.Bag[0].Locked = true
	c.InventoryState.Bag[3] = game.Item{ID: 48016, Count: 1}
	c.walkWaterAware(82, 55, *now)
	if len(*sent) != 1 || (*sent)[0][2] != 4 {
		t.Fatal("wrong eligible bag slot", *sent)
	}
	*now = now.Add(waterVehicleReplyTimeout)
	c.waterTravelTick()
	if c.waterTravel.requestID != 0 || c.World.Walking() {
		t.Fatal("timed-out boarding retained action")
	}
}

func TestWaterTravelRejectsObstaclesAndNonWaterClicks(t *testing.T) {
	c, now, sent := shoreClient(2)
	for y := 0; y < 10; y++ {
		c.World.Scene.Ground.Cells[2*10+y] = 4
	}
	c.walkWaterAware(82, 55, *now)
	if len(*sent) != 0 {
		t.Fatal("boarded through impassable terrain")
	}
	c, now, sent = shoreClient(2)
	c.World.Scene.Ground.Cells[4*10+2] = 4
	c.walkWaterAware(82, 55, *now)
	if len(*sent) != 0 {
		t.Fatal("blocked land click boarded a raft")
	}
}

func TestWaterLandingReportsPositionBeforeDismount(t *testing.T) {
	c, now, sent := shoreClient(2)
	c.World.Player.X = 82
	c.World.Player.VehicleID, c.World.Player.VehicleSlot = 48016, 1
	c.applyWaterVehicle()
	c.World.OnLeg = func(facing, x, y int) { *sent = append(*sent, []byte{6, 1, byte(facing), byte(x), 0, byte(y), 0}) }
	c.walkWaterAware(22, 55, *now)
	if len(*sent) != 1 || (*sent)[0][0] != 6 {
		t.Fatal("water approach not announced", *sent)
	}
	*now = now.Add(time.Second)
	c.World.Step(*now)
	if !c.World.Scene.Water(c.World.Player.X, c.World.Player.Y) {
		t.Fatal("vehicle walked onto land")
	}
	c.waterTravelTick()
	if len(*sent) != 3 || (*sent)[1][0] != 6 || (*sent)[1][1] != 2 || len((*sent)[1]) != 15 || (*sent)[2][0] != 15 || (*sent)[2][1] != 10 {
		t.Fatal("landing order", *sent)
	}
	if !c.World.Scene.Land(c.World.Player.X, c.World.Player.Y) {
		t.Fatal("no land relocation")
	}
	c.vehiclePacket([]byte{15, 11, 1, 101, 0, 0, 0})
	if c.World.Scene.WaterTravel || c.World.Player.VehicleID != 0 {
		t.Fatal("landing retained water travel")
	}
}

func TestPlacedVehicleTimeoutRestoresLand(t *testing.T) {
	c, now, sent := shoreClient(2)
	c.walkWaterAware(82, 55, *now)
	c.vehiclePacket([]byte{15, 18, 1, 101, 0, 0, 0, 0x90, 0xbb, 22, 0, 0, 0, 55, 0, 0, 0})
	*now = now.Add(waterVehicleReplyTimeout)
	c.waterTravelTick()
	if !c.World.Scene.Land(c.World.Player.X, c.World.Player.Y) || c.World.Walking() || c.waterTravel.requestID != 0 || len(*sent) != 2 {
		t.Fatal("boarding timeout stranded character on water")
	}
}

func TestRaftRecoveryPositionIsAuthoritative(t *testing.T) {
	c, now, _ := shoreClient(2)
	c.World.Player.VehicleID, c.World.Player.VehicleSlot = 48016, 1
	c.World.Player.X = 82
	c.applyWaterVehicle()
	c.walkWaterAware(142, 55, *now)
	c.dispatch([]byte{7, 101, 0, 0, 0, 0x13, 0x27, 22, 0, 55, 0}) // 10003, clear land.
	if c.World.Walking() || c.World.Player.X != 22 || c.waterTravel.goal != nil {
		t.Fatal("authoritative landing ignored")
	}
	c.vehiclePacket([]byte{15, 15, 101, 0, 0, 0, 0x90, 0xbb})
	if c.World.Player.VehicleID != 0 || c.World.Scene.WaterTravel || !c.World.Scene.Land(c.World.Player.X, c.World.Player.Y) {
		t.Fatal("broken raft stranded player")
	}
}
