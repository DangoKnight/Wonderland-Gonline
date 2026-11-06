package app

import (
	"encoding/binary"
	"sort"

	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

type vehicleKey struct {
	owner uint32
	slot  byte
}
type placedVehicle struct {
	id, mapID uint16
	x, y      int
	sprite    *role.Vehicle
}

const (
	vehiclePlacementPacketBytes = 17
	vehicleBoardHorizontalReach = 100 // FUN_0044aac0: strict X/Y bounds.
	vehicleBoardReachAbove      = 150
	vehicleBoardReachBelow      = 100
)

// AC15:12 is manual placement, not the AC15:18 automatic shore handshake.
func (c *Client) placeVehiclePacket(p []byte) {
	if c.World == nil || len(p) != vehiclePlacementPacketBytes || p[2] < 1 || p[2] > game.BagSize {
		return
	}
	key := vehicleKey{binary.LittleEndian.Uint32(p[3:]), p[2]}
	id := binary.LittleEndian.Uint16(p[7:])
	item, known := c.items[id]
	if !known || item.Definition.Type != game.VehicleType {
		return
	}
	if key.owner == c.World.Player.ID {
		if c.InventoryState == nil || c.InventoryState.Bag[key.slot-1].ID != id {
			return
		}
	}
	v := placedVehicle{id: id, mapID: c.World.Player.Map, x: int(binary.LittleEndian.Uint32(p[9:])), y: int(binary.LittleEndian.Uint32(p[13:]))}
	v.sprite = &role.Vehicle{Lib: c.lib, Sprite: item.Sprites[0], Water: nativeWaterVehicle(id)}
	if c.placedVehicles == nil {
		c.placedVehicles = map[vehicleKey]placedVehicle{}
	}
	c.placedVehicles[key] = v
}

func (c *Client) placedVehicleKeys() []vehicleKey {
	keys := make([]vehicleKey, 0, len(c.placedVehicles))
	for key, v := range c.placedVehicles {
		if c.World != nil && v.mapID == c.World.Player.Map {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].owner != keys[j].owner {
			return keys[i].owner < keys[j].owner
		}
		return keys[i].slot < keys[j].slot
	})
	return keys
}

func (c *Client) placedVehicleSprites() []world.SceneSprite {
	var out []world.SceneSprite
	for _, key := range c.placedVehicleKeys() {
		v := c.placedVehicles[key]
		out = append(out, world.SceneSprite{X: v.x, Y: v.y, Draw: func(dst *surface.Surface, x, y int) { v.sprite.Draw(dst, x, y) }})
	}
	return out
}

func (c *Client) boardPlacedVehicle(x, y int) bool {
	if c.World == nil || c.Inventory == nil || c.Inventory.Send == nil {
		return false
	}
	cx, cy := c.World.Camera()
	for _, key := range c.placedVehicleKeys() {
		v := c.placedVehicles[key]
		if key.owner != c.World.Player.ID || !v.sprite.Hit(v.x-cx, v.y-cy, x, y) {
			continue
		}
		return c.requestPlacedVehicle(key)
	}
	return false
}

func (c *Client) requestPlacedVehicle(key vehicleKey) bool {
	v, exists := c.placedVehicles[key]
	if !exists || c.World == nil || c.InventoryState == nil || c.Inventory == nil || c.Inventory.Send == nil || key.owner != c.World.Player.ID || v.mapID != c.World.Player.Map {
		return false
	}
	if c.waterTravel.requestID != 0 || c.World.Player.VehicleID != 0 {
		return true
	}
	it := c.InventoryState.Bag[key.slot-1]
	if it.Empty() || it.ID != v.id || it.Locked || it.Damage >= game.VehicleWreckDamage {
		delete(c.placedVehicles, key)
		return true
	}
	dx, dy := c.World.Player.X-v.x, c.World.Player.Y-v.y
	if dx <= -vehicleBoardHorizontalReach || dx >= vehicleBoardHorizontalReach || dy <= -vehicleBoardReachAbove || dy >= vehicleBoardReachBelow {
		return false
	}
	c.World.StopWalk()
	c.floorPendingPickup = 0
	c.pendingNPC, c.waterTravel.goal = nil, nil
	c.groundHeld = false
	c.waterTravel.requestID, c.waterTravel.requestSlot = v.id, key.slot
	c.waterTravel.stage, c.waterTravel.requested = protocol.PetControlBoardVehicle, c.Now()
	c.waterTravel.origin.X, c.waterTravel.origin.Y = c.World.Player.X, c.World.Player.Y
	c.Inventory.Send(protocol.Builder{protocol.CommandPetControl, protocol.PetControlBoardVehicle, key.slot}.U16(v.id))
	return true
}
