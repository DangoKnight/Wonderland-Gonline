// Package inventory ports the native bag/equipment objects and their AC23 updates.
package inventory

import (
	"encoding/binary"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	BagColumns           = 5
	BagRows              = 10
	EquipmentSlots       = 6
	bagRecordBytes       = 5 + game.ItemMetadataBytes
	equipmentRecordBytes = 21
	equipmentForgeOffset = 15
)

// State is the client's server-confirmed view. Opening a form or sending a
// request never changes it. FUN_003dc9d0 adds 31-byte records; FUN_003da044
// removes counts; FUN_003dce4c moves stacks; FUN_003fc308 restores worn items.
type State struct {
	Bag       game.Inventory
	Equipment game.Equipment
	Items     map[uint16]assets.NativeItem
	Pets      [game.MaxPets]UsePet
}

func (s *State) Reset(worn []uint16) {
	s.Bag, s.Equipment = game.Inventory{}, game.Equipment{}
	s.Pets = [game.MaxPets]UsePet{}
	for _, id := range worn {
		if slot := s.equipmentSlot(id); slot != 0 {
			s.Equipment[slot-1] = game.Item{ID: id, Count: 1}
		}
	}
}
func (s *State) equipmentSlot(id uint16) byte {
	slot := s.Items[id].Definition.EquipSlot
	if slot < 1 || slot > EquipmentSlots {
		return 0
	}
	return byte(slot)
}
func (s *State) FirstFree() byte {
	for i, it := range s.Bag {
		if it.Empty() {
			return byte(i + 1)
		}
	}
	return 0
}

// Apply validates the whole packet before publishing any changes. Unsupported
// commands return handled=false; malformed known layouts return valid=false.
func (s *State) Apply(p []byte) (handled, valid bool) {
	if len(p) < 2 || p[0] != protocol.CommandInventory {
		return false, false
	}
	bag, eq := s.Bag, s.Equipment
	switch p[1] {
	case protocol.InventoryItems:
		if (len(p)-2)%bagRecordBytes != 0 {
			return true, false
		}
		for o := 2; o < len(p); o += bagRecordBytes {
			r := p[o : o+bagRecordBytes]
			slot := int(r[0])
			id := binary.LittleEndian.Uint16(r[1:])
			if slot < 1 || slot > game.BagSize || id == 0 || r[3] == 0 || r[3] > game.MaxItemStack {
				return true, false
			}
			item := game.Item{ID: id, Count: r[3], Damage: r[4]}
			copy(item.Metadata[:], r[5:])
			old := bag[slot-1]
			if !old.Empty() {
				if old.ID != item.ID || old.Damage != item.Damage || old.Metadata != item.Metadata || int(old.Count)+int(item.Count) > game.MaxItemStack {
					return true, false
				}
				item.Count += old.Count
			}
			bag[slot-1] = item
		}
	case protocol.InventoryCompoundResult:
		if len(p) != 6+protocol.CompoundResultReservedBytes || p[2] < 1 || p[2] > game.BagSize || p[5] == 0 || p[5] > game.MaxItemStack || binary.LittleEndian.Uint16(p[3:]) == 0 || !bag[p[2]-1].Empty() {
			return true, false
		}
		item := game.Item{ID: binary.LittleEndian.Uint16(p[3:]), Count: p[5]}
		copy(item.Metadata[:], p[6:])
		bag[p[2]-1] = item
	case protocol.InventoryRemove:
		if len(p) != 4 || bag.Remove(p[2], p[3]) != nil {
			return true, false
		}
	case protocol.InventoryMove:
		if len(p) != 5 || p[2] < 1 || p[2] > game.BagSize {
			return true, false
		}
		limit := byte(game.MaxItemStack)
		if it, ok := s.Items[bag[p[2]-1].ID]; ok {
			limit = it.Definition.StackLimit()
		}
		moved, err := bag.Move(p[2], p[4], p[3], limit)
		if err != nil || moved != p[3] {
			return true, false
		}
	case protocol.InventoryEquip: // 23/11: complete worn-item list, not a wear acknowledgment.
		if (len(p)-2)%equipmentRecordBytes != 0 {
			return true, false
		}
		eq = game.Equipment{}
		for o := 2; o < len(p); o += equipmentRecordBytes {
			r := p[o : o+equipmentRecordBytes]
			id := binary.LittleEndian.Uint16(r)
			slot := s.equipmentSlot(id)
			if slot == 0 || eq[slot-1].ID != 0 {
				return true, false
			}
			item := game.Item{ID: id, Count: 1, Damage: r[2]}
			item.Metadata[game.ForgeMetadataOffset] = r[equipmentForgeOffset]
			eq[slot-1] = item
		}
	case protocol.InventoryPetEquip: // 23/17: native player's wear/swap acknowledgment (FUN_003dcd04).
		if len(p) != 4 || p[2] < 1 || p[2] > game.BagSize || p[3] < 1 || p[3] > game.BagSize {
			return true, false
		}
		item := bag[p[2]-1]
		slot := s.equipmentSlot(item.ID)
		if slot == 0 || item.Count != 1 {
			return true, false
		}
		old := eq[slot-1]
		if p[2] != p[3] && !bag[p[3]-1].Empty() {
			return true, false
		}
		bag[p[2]-1] = game.Item{}
		bag[p[3]-1] = old
		eq[slot-1] = item
	case protocol.InventoryWireCode23: // Native pet equip acknowledgement.
		if len(p) != 4 || p[2] < 1 || p[2] > game.MaxPets || p[3] < 1 || p[3] > game.BagSize {
			return true, false
		}
		pet := &s.Pets[p[2]-1]
		item := bag[p[3]-1]
		equipSlot := s.equipmentSlot(item.ID)
		if pet.ID == 0 || item.Count != 1 || equipSlot == 0 {
			return true, false
		}
		bag[p[3]-1], pet.Equipment[equipSlot-1] = pet.Equipment[equipSlot-1], item
		s.recomputePet(pet)
	case protocol.InventoryWireCode22: // Native pet unequip acknowledgement.
		if len(p) != 5 || p[2] < 1 || p[2] > game.MaxPets || p[3] < 1 || p[3] > EquipmentSlots || p[4] < 1 || p[4] > game.BagSize {
			return true, false
		}
		pet := &s.Pets[p[2]-1]
		if pet.ID == 0 || pet.Equipment[p[3]-1].Empty() || !bag[p[4]-1].Empty() {
			return true, false
		}
		bag[p[4]-1], pet.Equipment[p[3]-1] = pet.Equipment[p[3]-1], game.Item{}
		s.recomputePet(pet)
	case protocol.InventoryEquipmentChanged: // 23/16: native unequip into a bag slot (FUN_003fbcd0).
		if len(p) != 4 || p[2] < 1 || p[2] > EquipmentSlots || p[3] < 1 || p[3] > game.BagSize || eq[p[2]-1].Empty() || !bag[p[3]-1].Empty() {
			return true, false
		}
		bag[p[3]-1], eq[p[2]-1] = eq[p[2]-1], game.Item{}
	default:
		return false, false
	}
	s.Bag, s.Equipment = bag, eq
	return true, true
}
