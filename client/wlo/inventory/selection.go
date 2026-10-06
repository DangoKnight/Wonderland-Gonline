package inventory

import (
	"encoding/binary"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	petStatPacketBytes  = 15
	petStatCollection   = 4
	petStatPositive     = 1
	petStatNegative     = 2
	petStatSlotOffset   = 3
	petStatCodeOffset   = 5
	petStatSignOffset   = 6
	petStatValueOffset  = 7
	petStatTargetOffset = 11
)

// Selection is local inventory presentation, independent of the battle pet.
func (f *Form) SelectNext(direction int) {
	slots := []byte{0}
	for i, p := range f.State.Pets {
		if p.ID != 0 {
			slots = append(slots, byte(i+1))
		}
	}
	current := 0
	for i, slot := range slots {
		if slot == f.Selected {
			current = i
			break
		}
	}
	f.Select(slots[(current+direction+len(slots))%len(slots)])
}
func (f *Form) Select(slot byte) {
	if slot > game.MaxPets || slot != 0 && f.State.Pets[slot-1].ID == 0 {
		return
	}
	f.Selected = slot
	f.drag = nil
	f.CancelPoints()
}
func (f *Form) syncSelection() {
	if f.Selected != 0 && f.State.Pets[f.Selected-1].ID == 0 {
		f.Select(0)
	}
	hasPets := false
	for _, p := range f.State.Pets {
		hasPets = hasPets || p.ID != 0
	}
	if f.RotateLeft != nil {
		f.RotateLeft.Enabled = hasPets
		f.RotateRight.Enabled = hasPets
	}
}
func (f *Form) DisplayStats() *world.Stats {
	if f.Selected != 0 && f.State.Pets[f.Selected-1].ID != 0 {
		stats := &f.State.Pets[f.Selected-1].Stats
		stats.Formula = f.Stats.Formula
		return stats
	}
	return f.Stats
}
func (f *Form) Equipment() game.Equipment {
	if f.Selected != 0 && f.State.Pets[f.Selected-1].ID != 0 {
		return f.State.Pets[f.Selected-1].Equipment
	}
	return f.State.Equipment
}
func (f *Form) equipBag(slot byte) {
	if f.Selected == 0 {
		f.send([]byte{protocol.CommandInventory, protocol.InventoryEquip, slot})
	} else {
		f.send([]byte{protocol.CommandInventory, protocol.InventoryPetEquip, f.Selected, slot})
	}
}
func (s *State) recomputePet(p *UsePet) {
	defs := make(map[uint16]game.ItemDefinition, EquipmentSlots)
	for _, it := range p.Equipment {
		if !it.Empty() {
			defs[it.ID] = s.Items[it.ID].Definition
		}
	}
	stats := &p.Stats
	pet := game.Pet{ID: uint32(p.ID), Level: stats.Level, Base: game.Attributes{Strength: stats.STR, Constitution: stats.CON, Intelligence: stats.INT, Wisdom: stats.WIS, Agility: stats.AGI}, Equipment: p.Equipment}
	c := pet.Combat(defs)
	stats.MaxHP, stats.MaxSP = uint32(c.MaxHP), uint16(c.MaxSP)
	stats.Combat = [5]uint16{uint16(c.ATK), uint16(c.DEF), uint16(c.MAT), uint16(c.MDF), uint16(c.SPD)}
}

func (s *State) ApplyPetStat(p []byte) bool {
	if len(p) != petStatPacketBytes || p[0] != protocol.CommandStats || p[1] != protocol.StatsWireCode2 || p[2] != petStatCollection || p[4] != 0 || p[petStatSlotOffset] < 1 || p[petStatSlotOffset] > game.MaxPets || p[petStatSignOffset] < petStatPositive || p[petStatSignOffset] > petStatNegative {
		return false
	}
	pet := &s.Pets[p[petStatSlotOffset]-1]
	if pet.ID == 0 {
		return false
	}
	v := binary.LittleEndian.Uint32(p[petStatValueOffset:])
	if p[petStatCodeOffset] == game.StatSkillGrade || p[petStatCodeOffset] == game.StatSkillEXP {
		id := binary.LittleEndian.Uint32(p[petStatTargetOffset:])
		if id == 0 || id > 65535 || p[petStatSignOffset] != petStatPositive || p[petStatCodeOffset] == game.StatSkillGrade && v > game.MaxSkillGrade {
			return false
		}
		for i := range pet.Skills {
			if pet.Skills[i].ID == uint16(id) {
				if p[petStatCodeOffset] == game.StatSkillGrade {
					pet.Skills[i].Grade = byte(v)
				} else {
					pet.Skills[i].Exp = v
				}
				return true
			}
		}
		return false
	}
	if p[petStatSignOffset] == petStatNegative {
		v = uint32(-int32(v))
	}
	pet.Stats.Apply(p[petStatCodeOffset], v)
	switch p[petStatCodeOffset] {
	case world.StatCON, world.StatWIS, world.StatLevel:
		s.recomputePet(pet)
	case itemAmityStatus:
		pet.Amity = byte(v)
	}
	return true
}
