package app

import (
	"encoding/binary"
	"image"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	waterVehicleReplyTimeout = 5 * time.Second
	waterVehicleBoat         = 48003
	waterVehicleJalor        = 48005
	waterVehicleSteamShip    = 48006
	waterVehicleYacht        = 48012
	waterVehicleLifeboat     = 48015
	waterVehicleSubmarine    = 48017
)

// Known native water vehicle families, FUN_00154a64 -> FUN_001549e8.
// Capsule IDs use the same travel class as their unpacked vehicle.
func nativeWaterVehicle(id uint16) bool {
	switch game.BaseVehicleID(id) {
	case waterVehicleBoat, waterVehicleJalor, waterVehicleSteamShip, game.RaftItemID, waterVehicleYacht, waterVehicleLifeboat, game.DisposableRaftItemID, waterVehicleSubmarine:
		return true
	}
	return false
}

type waterTravelState struct {
	goal        *image.Point
	requestID   uint16
	requestSlot byte
	requested   time.Time
	landing     bool
	stage       byte
	shore       image.Point
	origin      image.Point
}

func (c *Client) walkWaterAware(x, y int, now time.Time) {
	w := c.World
	target := image.Pt(x, y)
	c.waterTravel.goal = nil
	if c.waterTravel.requestID != 0 {
		c.waterTravel.goal = &target
		return
	}
	if (w.Scene.Water(x, y) && !w.Scene.WaterTravel) || (w.Scene.Land(x, y) && w.Scene.WaterTravel) {
		c.waterTravel.goal = &target
	}
	w.StopWalk()
	w.WalkTo(x, y, now)
	c.waterTravelTick()
}

// FUN_0041897c -> AC15:14 -> FUN_0044a6dc -> AC15:7 -> AC15:10.
// Placement moves the avatar to a saved water cell; boarding confirmation
// enables water pathing. Landing moves to clear land before AC15:10.
func (c *Client) waterTravelTick() {
	w := c.World
	if w == nil || c.held || c.event.active || c.movie != nil || c.sport != nil || c.sceneFrozen() {
		return
	}
	t := &c.waterTravel
	if t.requestID != 0 {
		if c.Now().Sub(t.requested) >= waterVehicleReplyTimeout {
			if (!t.landing && w.Player.VehicleID == 0) || (t.landing && w.Player.VehicleID != 0) {
				w.Relocate(t.origin)
			}
			*t = waterTravelState{}
		}
		return
	}
	if t.goal == nil || w.Walking() {
		return
	}
	if w.Scene.WaterTravel {
		if !w.Scene.Land(t.goal.X, t.goal.Y) || !w.Scene.Water(w.Player.X, w.Player.Y) {
			t.goal = nil
			return
		}
		shore, ok := w.Scene.ShorePoint(w.Player.X, w.Player.Y, false)
		if !ok {
			t.goal = nil
			return
		}
		t.requestID, t.requestSlot, t.requested, t.landing = w.Player.VehicleID, w.Player.VehicleSlot, c.Now(), true
		t.stage = protocol.PetControlDismountVehicle
		t.origin = image.Pt(w.Player.X, w.Player.Y)
		w.Relocate(shore)
		// The server tracks announced endpoints. Report land while still mounted,
		// before the dismount transaction snapshots the recovery position.
		position := protocol.Builder{protocol.CommandMovement, protocol.MovementStop, byte(w.Player.Direction)}.U16(uint16(shore.X)).U16(uint16(shore.Y))
		c.Inventory.Send(position.Bytes(make([]byte, movementCheckBytes)))
		c.Inventory.Send(protocol.Builder{protocol.CommandPetControl, protocol.PetControlDismountVehicle, t.requestSlot}.U16(t.requestID))
		return
	}
	if !w.Scene.Water(t.goal.X, t.goal.Y) || !w.Scene.WaterEdge(w.Player.X, w.Player.Y) {
		t.goal = nil
		return
	}
	shore, ok := w.Scene.ShorePoint(w.Player.X, w.Player.Y, true)
	if !ok {
		t.goal = nil
		return
	}
	path := w.Scene.PlanWater(shore.X, shore.Y, t.goal.X, t.goal.Y)
	if len(path) == 0 && !w.Scene.SameCell(shore, *t.goal) {
		t.goal = nil
		return
	}
	for i, it := range c.InventoryState.Bag {
		d, known := c.items[it.ID]
		if !known || d.Definition.Type != game.VehicleType || !nativeWaterVehicle(it.ID) || it.Empty() || it.Locked || it.Damage >= game.VehicleWreckDamage {
			continue
		}
		t.requestID, t.requestSlot, t.requested = it.ID, byte(i+1), c.Now()
		t.stage, t.shore, t.origin = protocol.PetControlPlaceVehicle, shore, image.Pt(w.Player.X, w.Player.Y)
		c.Inventory.Send(protocol.Builder{protocol.CommandPetControl, protocol.PetControlPlaceVehicle, t.requestSlot}.U16(it.ID))
		return
	}
	t.goal = nil
}

// Only the matching owner placement receipt may advance pending boarding.
// Its coordinates locate the placed vehicle, while the player's shore cell is
// retained from the terrain scan, as in FUN_0044a6dc.
func (c *Client) vehiclePlacement(p []byte) {
	const placementPacketBytes = 17
	if len(p) != placementPacketBytes {
		return
	}
	t := &c.waterTravel
	if t.stage != protocol.PetControlPlaceVehicle || t.requestID == 0 ||
		p[2] != t.requestSlot || binary.LittleEndian.Uint32(p[3:]) != c.World.Player.ID ||
		binary.LittleEndian.Uint16(p[7:]) != t.requestID {
		return
	}
	c.World.Relocate(t.shore)
	t.stage, t.requested = protocol.PetControlBoardVehicle, c.Now()
	c.Inventory.Send(protocol.Builder{protocol.CommandPetControl, protocol.PetControlBoardVehicle, t.requestSlot}.U16(t.requestID))
}

func (c *Client) applyWaterVehicle() {
	if c.World == nil {
		return
	}
	p := &c.World.Player
	c.World.Scene.WaterTravel = nativeWaterVehicle(p.VehicleID)
	c.setVehicleLook(c.World.Body, p.VehicleID)
}

// Owner and public-scene mount/dismount replies use the existing native layouts.
func (c *Client) vehiclePacket(p []byte) {
	if c.World == nil {
		return
	}
	var owner uint32
	var id uint16
	var slot byte
	if len(p) < 2 {
		return
	}
	switch p[1] {
	case protocol.PetControlVehiclePosition:
		c.vehiclePlacement(p)
		return
	case protocol.PetControlVehicleMount:
		if len(p) != 9 {
			return
		}
		slot, owner, id = p[2], binary.LittleEndian.Uint32(p[3:]), binary.LittleEndian.Uint16(p[7:])
		if slot < 1 || slot > game.BagSize || c.items[id].Definition.Type != game.VehicleType {
			return
		}
	case protocol.PetControlWireCode11:
		if len(p) != 7 {
			return
		}
		owner = binary.LittleEndian.Uint32(p[3:])
	case protocol.PetControlRemoveVehicle:
		if len(p) != 8 {
			return
		}
		owner = binary.LittleEndian.Uint32(p[2:])
		c.breakVehicle(owner, binary.LittleEndian.Uint16(p[6:]))
	default:
		return
	}
	if owner != c.World.Player.ID {
		if peer := c.World.Peers[owner]; peer != nil {
			peer.VehicleID, peer.VehicleSlot = id, slot
			c.setVehicleLook(peer.Role, id)
		}
		return
	}
	t := &c.waterTravel
	c.World.Player.VehicleID, c.World.Player.VehicleSlot = id, slot
	c.applyWaterVehicle()
	t.requestID, t.requestSlot, t.landing, t.stage = 0, 0, false, 0
	if id == 0 {
		c.World.StopWalk()
	}
	if t.goal != nil {
		goal := *t.goal
		t.goal = nil
		c.walkWaterAware(goal.X, goal.Y, c.Now())
		c.World.MarkWalk()
	}
}

func (c *Client) setVehicleLook(body any, id uint16) {
	if rider, ok := body.(interface{ SetVehiclePose(uint16, bool) }); ok {
		rider.SetVehiclePose(c.items[id].Sprites[0], nativeWaterVehicle(id))
	} else if rider, ok := body.(interface{ SetVehicle(uint16) }); ok {
		rider.SetVehicle(c.items[id].Sprites[0])
	}
}
