package game

import "encoding/binary"

// Native item instance fields +0x20/+0x24 map to these bag metadata offsets.
const (
	VehicleWearMetadataOffset = 12
	VehicleFuelMetadataOffset = 16
)

func (i Item) VehicleWear() uint32 {
	return binary.LittleEndian.Uint32(i.Metadata[VehicleWearMetadataOffset:])
}
func (i Item) VehicleFuel() uint16 {
	return binary.LittleEndian.Uint16(i.Metadata[VehicleFuelMetadataOffset:])
}
func (i *Item) SetVehicleProgress(wear uint32, fuel uint16) {
	binary.LittleEndian.PutUint32(i.Metadata[VehicleWearMetadataOffset:], wear)
	binary.LittleEndian.PutUint16(i.Metadata[VehicleFuelMetadataOffset:], fuel)
}

// VehicleType is eItemType.Vehicle in the native item table.
const VehicleType byte = 39

// BaseVehicleID groups capsules with their unpacked item (VehicleManager).
func BaseVehicleID(id uint16) uint16 {
	switch {
	case id >= 48019 && id <= 48032:
		return id - 18
	case id == 48033:
		return 48017
	case id == 48034:
		return 48018
	case id == 48036 || id == 48038 || id == 48040 || id == 48042:
		return id - 1
	}
	return id
}

func SameVehicle(actual uint32, required uint16) bool {
	return actual > 0 && actual <= 0xffff && BaseVehicleID(uint16(actual)) == BaseVehicleID(required)
}

func Raft(id uint16) bool { return BaseVehicleID(id) == RaftItemID || id == DisposableRaftItemID }

// Vehicle validates the exact bag slot and item, never a same-family replacement.
func (c Character) Vehicle(slot byte, id uint16, items map[uint16]ItemDefinition) (Item, bool) {
	if slot < 1 || slot > BagSize || id == 0 {
		return Item{}, false
	}
	item := c.Bag[slot-1]
	def, known := items[id]
	return item, known && !item.Locked && !item.Empty() && item.ID == id && def.Type == VehicleType && item.Damage < VehicleWreckDamage
}

// NormalizeVehicle clears stale riding state after a bag mutation or on login.
// Riding a valid item vehicle excludes a companion mount, but preserves the
// selected battle pet. Hiding its following sprite is presentation state.
func (c *Character) NormalizeVehicle(items map[uint16]ItemDefinition) bool {
	beforeID, beforeSlot, beforeMount := c.ActiveVehicle, c.VehicleSlot, c.ActiveMount
	if _, ok := c.Vehicle(c.VehicleSlot, c.ActiveVehicle, items); !ok {
		c.ActiveVehicle, c.VehicleSlot = 0, 0
	} else {
		c.ActiveMount = 0
	}
	return beforeID != c.ActiveVehicle || beforeSlot != c.VehicleSlot || beforeMount != c.ActiveMount
}
