package app

import (
	"encoding/binary"
	"strconv"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/internal/game"
)

const vehicleProgressPacketBytes = 13

func (c *Client) initVehicleStatus() {
	c.loadPictures("icon_HP2", "icon_SP2", "icon_rail2", "icon_HP3", "icon_SP3", "icon_rail3")
	c.MainStatus.Vehicle.Read = c.vehicleStatusInfo
}

func (c *Client) vehicleStatusInfo() (hud.VehicleInfo, bool) {
	if c.World == nil || c.InventoryState == nil {
		return hud.VehicleInfo{}, false
	}
	p := c.World.Player
	if p.VehicleID == 0 || p.VehicleSlot < 1 || p.VehicleSlot > game.BagSize {
		return hud.VehicleInfo{}, false
	}
	item := c.InventoryState.Bag[p.VehicleSlot-1]
	def, known := c.items[p.VehicleID]
	if !known || item.Empty() || item.ID != p.VehicleID || def.Definition.Type != game.VehicleType {
		return hud.VehicleInfo{}, false
	}
	condition := def.VehicleCondition(item)
	w, h := def.Definition.Footprint()
	c.loadPictures(strconv.Itoa(int(def.Icon)))
	return hud.VehicleInfo{Name: def.Definition.Name, Icon: def.Icon, Columns: w, Rows: h, Health: condition.Health, Maximum: condition.Maximum, Fuel: condition.Fuel}, true
}

func (c *Client) vehicleProgress(p []byte) {
	if len(p) != vehicleProgressPacketBytes || c.World == nil || c.InventoryState == nil {
		return
	}
	pl := c.World.Player
	if pl.VehicleID == 0 || p[2] != pl.VehicleSlot || p[2] < 1 || p[2] > game.BagSize || binary.LittleEndian.Uint32(p[3:]) != pl.ID {
		return
	}
	item := &c.InventoryState.Bag[p[2]-1]
	if item.ID != pl.VehicleID || item.Empty() || c.items[item.ID].Definition.Type != game.VehicleType {
		return
	}
	item.SetVehicleProgress(binary.LittleEndian.Uint32(p[7:]), binary.LittleEndian.Uint16(p[11:]))
}
