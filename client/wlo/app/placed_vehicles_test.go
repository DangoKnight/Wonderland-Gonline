package app

import (
	"bytes"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/world"
)

func TestPlacedVehicleSailsFromShore(t *testing.T) {
	c, now, sent := shoreClient(2)
	c.placeVehiclePacket([]byte{15, 12, 1, 101, 0, 0, 0, 144, 187, 22, 0, 0, 0, 55, 0, 0, 0})
	if !c.requestPlacedVehicle(vehicleKey{101, 1}) {
		t.Fatal("could not request boarding")
	}
	c.vehiclePacket([]byte{15, 10, 1, 101, 0, 0, 0, 144, 187})
	c.walkWaterAware(82, 55, *now)
	if !c.World.Walking() || len(*sent) != 1 {
		t.Fatal("manual raft could not sail from shore")
	}
	*now = now.Add(time.Second)
	c.World.Step(*now)
	if !c.World.Scene.Water(c.World.Player.X, c.World.Player.Y) {
		t.Fatal("manual raft never reached water")
	}
}

func TestPlacedVehicleLegacyCapture(t *testing.T) {
	c, _, sent := shoreClient(2)
	c.World.Player.ID = 4617590
	c.InventoryState.Bag[11] = c.InventoryState.Bag[0]
	// Raw server placement from raft-reference-20261103-01.pcapng.
	packet := []byte{15, 12, 12, 118, 117, 70, 0, 144, 187, 232, 11, 0, 0, 79, 8, 0, 0}
	c.dispatch(packet)
	key := vehicleKey{4617590, 12}
	v, ok := c.placedVehicles[key]
	if !ok || v.x != 3048 || v.y != 2127 || v.id != 48016 || len(*sent) != 0 || c.World.Player.VehicleID != 0 {
		t.Fatal("manual placement boarded or lost coordinates", v, *sent)
	}
	if len(c.placedVehicleSprites()) != 1 {
		t.Fatal("placed raft not in scene draw queue")
	}
	if c.requestPlacedVehicle(key) {
		t.Fatal("boarded out of reach")
	}
	c.World.Player.X, c.World.Player.Y = 3048, 2127
	if !c.requestPlacedVehicle(key) || len(*sent) != 1 || !bytes.Equal((*sent)[0], []byte{15, 7, 12, 144, 187}) || c.World.Player.VehicleID != 0 {
		t.Fatal("boarding request or acknowledgment gate", *sent)
	}
	c.requestPlacedVehicle(key)
	if len(*sent) != 1 {
		t.Fatal("duplicate boarding")
	}
	c.dispatch([]byte{15, 10, 12, 118, 117, 70, 0, 144, 187})
	if c.World.Player.VehicleID != 48016 || len(c.placedVehicles) != 0 {
		t.Fatal("boarding left duplicate raft")
	}
	before := c.InventoryState.Bag
	c.dispatch([]byte{15, 11, 12, 118, 117, 70, 0})
	if c.World.Player.VehicleID != 0 || c.InventoryState.Bag != before {
		t.Fatal("landing consumed inventory")
	}
}

func TestPlacedVehicleRejectsStaleAndPeerBoarding(t *testing.T) {
	c, _, sent := shoreClient(2)
	p := []byte{15, 12, 1, 101, 0, 0, 0, 144, 187, 22, 0, 0, 0, 55, 0, 0, 0}
	c.placeVehiclePacket(p[:9])
	if len(c.placedVehicles) != 0 {
		t.Fatal("truncated placement accepted")
	}
	c.placeVehiclePacket(p)
	c.InventoryState.Bag[0].Locked = true
	if !c.requestPlacedVehicle(vehicleKey{101, 1}) || len(*sent) != 0 || len(c.placedVehicles) != 0 {
		t.Fatal("stale placement boarded")
	}
	p[3] = 102
	c.World.Peers = map[uint32]*world.Peer{}
	c.placeVehiclePacket(p)
	if c.requestPlacedVehicle(vehicleKey{102, 1}) || len(*sent) != 0 {
		t.Fatal("boarded another owner's raft")
	}
}
