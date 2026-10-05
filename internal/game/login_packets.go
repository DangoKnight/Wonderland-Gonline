package game

import (
	"fmt"
	"math"
	"wonderland-go/internal/protocol"
)

func (c Character) WornEquipment() []byte {
	p := protocol.Builder{}
	for _, item := range c.Equipment {
		if item.ID != 0 {
			p = p.U16(item.ID)
		}
	}
	return p
}

// AppearancePacket serializes AC3 (self) or AC4 (other player).
func (c Character) AppearancePacket(other bool) ([]byte, error) {
	if e := c.Validate(); e != nil {
		return nil, e
	}
	p := protocol.Builder{protocol.CommandMapLoad}
	if other {
		p[0] = 4
	}
	p = p.U32(c.ID).U8(byte(c.Body))
	if other {
		p = p.U8(c.Element).U8(c.Level)
	}
	p = p.U16(c.Map).U16(c.X).U16(c.Y).U8(0).U16(c.Head).U32(c.Color1).U32(c.Color2)
	worn := c.WornEquipment()
	p = p.U8(byte(len(worn) / 2)).Bytes(worn).U32(0)
	if other {
		p = p.U8(0).U8(c.RebornByte()).U8(c.Job)
	}
	p, e := p.String(c.Name)
	if e != nil {
		return nil, e
	}
	p, e = p.String(c.Nickname)
	if e != nil {
		return nil, e
	}
	if other {
		p = p.U8(255).U8(c.BloodType).U8(c.BirthYearOffset).U8(c.BirthMonth).U8(c.BirthDay).U8(1)
	} else {
		p = p.U8(c.BloodType).U8(c.BirthYearOffset).U8(c.BirthMonth).U8(c.BirthDay)
	}
	return p, nil
}

func (c Character) BaseStatsPacket(tableOrder func(uint16) (uint16, bool)) ([]byte, error) {
	if e := c.Validate(); e != nil {
		return nil, e
	}
	if c.SP > 65535 || c.MaxHP > 65535 || c.MaxSP > 65535 || len(c.Skills) > 65535 {
		return nil, fmt.Errorf("base stats exceed native field limits")
	}
	a := c.Attributes()
	p := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateWireCode3, c.Element}.U32(c.HP).U16(uint16(c.SP)).U16(a.Constitution).U16(a.Intelligence).U16(a.Strength).U16(a.Agility).U16(a.Wisdom).U8(c.Level).U32(c.EXP).U16(uint16(c.MaxHP)).U16(uint16(c.MaxSP))
	p = p.U32(417).U16(0).U32(0).U32(240).U32(0).U32(0).U32(0).U32(0).U32(0).U16(uint16(len(c.Skills)))
	for _, skill := range c.Skills {
		id := skill.ID
		if id == StarterStunt(c.Body, c.Head) {
			id = 15003
		}
		order, ok := tableOrder(id)
		if !ok {
			return nil, fmt.Errorf("skill %d missing from native catalog", id)
		}
		p = p.U16(order).U8(skill.Grade).U32(skill.EXP)
	}
	return p.U16(0).U16(0).U8(0).U8(c.RebornByte()).U8(byte(c.Potential)).U8(c.Job), nil
}

func (c Character) EquipmentPacket() []byte {
	p := protocol.Builder{protocol.CommandInventory, protocol.InventoryEquip}
	for _, item := range c.Equipment {
		if item.ID != 0 {
			// The 18-byte tail carries forge at its byte 12 (item-record byte 15).
			var tail [18]byte
			tail[12] = item.Forge()
			p = p.U16(item.ID).U8(item.Damage).Bytes(tail[:])
		}
	}
	return p
}
func (c Character) WarpPacket(portal uint16) []byte {
	return protocol.Builder{protocol.CommandMapAcknowledgment}.U32(c.ID).U16(c.Map).U16(c.X).U16(c.Y).U16(portal).U8(0)
}
func (c Character) PositionPacket() []byte {
	return protocol.Builder{protocol.CommandPosition}.U32(c.ID).U16(c.Map).U16(c.X).U16(c.Y)
}

// StatPackets follows Equip.Send8_1 ordering: maxima follow CON/WIS, then current HP/SP.
func (c Character) StatPackets(items map[uint16]ItemDefinition, growth ...ElementalGrowth) [][]byte {
	a := c.Attributes()
	b := c.Equipment.Bonuses(items)
	// Native AC8 values are contributions: aLogin adds its own level formulas.
	// Send rounded differences from those native formulas to avoid double-counting.
	g := characterGrowth(c.Element, growth)
	native := nativeElementGrowth(c.Element)
	level := float64(c.Level)
	delta := func(x, y float64) int32 { return int32(math.RoundToEven(x)) - int32(math.RoundToEven(y)) }
	atk := delta(g.ATK.value(level, a), native.ATK.value(level, a))
	def := delta(g.DEF.value(level, a), native.DEF.value(level, a))
	mat := delta(g.MAT.value(level, a), native.MAT.value(level, a))
	mdf := delta(g.MDF.value(level, a), native.MDF.value(level, a))
	spd := delta(g.SPD.value(level, a), native.SPD.value(level, a))

	innate := func(v float64) int32 { return int32(uint16(math.RoundToEven(v))) }
	baseATK, baseDEF, baseMAT, baseMDF, baseSPD := innate(g.ATK.value(level, a)), innate(g.DEF.value(level, a)), innate(g.MAT.value(level, a)), innate(g.MDF.value(level, a)), innate(g.SPD.value(level, a))
	classATK, classDEF, classMAT, classMDF, classSPD := c.classStats(baseATK, baseDEF, baseMAT, baseMDF, baseSPD)
	atk += classATK - baseATK
	def += classDEF - baseDEF
	mat += classMAT - baseMAT
	mdf += classMDF - baseMDF
	spd += classSPD - baseSPD
	hp := delta(g.HP.value(level, a), native.HP.value(level, a))
	sp := delta(g.SP.value(level, a), native.SP.value(level, a))
	stats := []struct {
		id    byte
		value int32
	}{{StatAttack, int32(a.Strength)*2 + int32(b.ATK) + atk}, {StatDefense, int32(a.Constitution)*2 + int32(b.DEF) + def}, {StatMagicAttack, int32(a.Intelligence)*2 + int32(b.MAT) + mat}, {StatMagicDefense, int32(a.Wisdom)*2 + int32(b.MDF) + mdf}, {StatSpeed, int32(a.Agility)*2 + int32(b.SPD) + spd}, {StatSTR, int32(a.Strength)}, {StatCON, int32(a.Constitution)}, {StatINT, int32(a.Intelligence)}, {StatWIS, int32(a.Wisdom)}, {StatAGI, int32(a.Agility)}, {StatUnallocatedPoints, int32(c.StatPoints)}, {StatHPBonus, b.HP + hp}, {StatSPBonus, b.SP + sp}, {StatCurrentHP, int32(c.HP)}, {StatCurrentSP, int32(c.SP)}}
	out := make([][]byte, 0, len(stats))
	for _, v := range stats {
		out = append(out, protocol.Builder{protocol.CommandStats, protocol.StatsStatUpdate, v.id, protocol.StatsValueAbsolute}.U32(uint32(v.value)).U32(0))
	}
	return out
}
