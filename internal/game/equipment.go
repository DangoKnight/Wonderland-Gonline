package game

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// Equipment holds worn items by native slot (index slot-1), keeping damage and forge.
type Equipment [6]Item

// UnmarshalJSON also accepts the schema v2 form, an array of item IDs.
func (e *Equipment) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw) != len(e) {
		return fmt.Errorf("equipment has %d slots", len(raw))
	}
	var out Equipment
	for i, v := range raw {
		if t := bytes.TrimSpace(v); len(t) > 0 && t[0] != '{' {
			var id uint16
			if err := json.Unmarshal(t, &id); err != nil {
				return err
			}
			if id != 0 {
				out[i] = Item{ID: id, Count: 1}
			}
			continue
		}
		if err := json.Unmarshal(v, &out[i]); err != nil {
			return err
		}
	}
	*e = out
	return nil
}

func (e Equipment) IDs() [6]uint16 {
	var ids [6]uint16
	for i, item := range e {
		ids[i] = item.ID
	}
	return ids
}

// Forge is the item's point-forging level, stored at InventoryMetadata byte 18.
func (i Item) Forge() byte { return i.Metadata[ForgeMetadataOffset] }

// Bonuses are equipment totals. Reference: Equip.ForgedStat and EquipManager.Equipped*.
// HP and SP are Int32 sums; the combat totals are Int16 sums read back as UInt16.
type Bonuses struct {
	HP, SP                  int32
	ATK, DEF, MAT, MDF, SPD uint16
}

func forgedStat(d ItemDefinition, forge byte, index int) int32 {
	value := int32(0)
	if d.Values[index] != 0 {
		value = d.Values[index] - 100
	}
	if forge == 0 || d.Status[index] == 0 || d.Status[index] == 250 || d.Values[index] < 100 {
		return value
	}
	count := 0
	for i := range d.Status {
		if d.Status[i] != 0 && d.Status[i] != 250 && d.Values[i] >= 100 {
			count++
		}
	}
	if count == 1 {
		return value + int32(forge)*2
	}
	return value + int32(forge)
}

func (e Equipment) Bonuses(items map[uint16]ItemDefinition) Bonuses {
	var b Bonuses
	var atk, def, mat, mdf, spd int16
	for _, item := range e {
		if item.ID == 0 {
			continue
		}
		d := items[item.ID]
		for i, status := range d.Status {
			v := forgedStat(d, item.Forge(), i)
			switch status {
			case 207, 205, 25:
				b.HP += v
			case 208, 206, 26:
				b.SP += v
			case 210, 41, 28:
				atk += int16(v)
			case 211, 42, 29:
				def += int16(v)
			case 215, 43, 27:
				mat += int16(v)
			case 216, 44, 33:
				mdf += int16(v)
			case 214, 45, 30:
				spd += int16(v)
			}
		}
	}
	b.ATK, b.DEF, b.MAT, b.MDF, b.SPD = uint16(atk), uint16(def), uint16(mat), uint16(mdf), uint16(spd)
	return b
}

// Element values follow the C# Affinity enum.
const (
	Earth byte = 1
	Water byte = 2
	Fire  byte = 3
	Wind  byte = 4
)

// Combat is EquipManager.Full*: rounded innate stats with reborn class bonuses,
// followed by equipment bonuses.
type Combat struct{ MaxHP, MaxSP, ATK, DEF, MAT, MDF, SPD int32 }

func (c Character) Combat(items map[uint16]ItemDefinition, growth ...ElementalGrowth) Combat {
	a := c.Attributes()
	level := float64(c.Level)
	g := characterGrowth(c.Element, growth)
	round := func(v float64) int32 { return int32(uint16(math.RoundToEven(v))) }
	b := c.Equipment.Bonuses(items)
	atk, def, mat, mdf, spd := c.classStats(round(g.ATK.value(level, a)), round(g.DEF.value(level, a)), round(g.MAT.value(level, a)), round(g.MDF.value(level, a)), round(g.SPD.value(level, a)))
	return Combat{
		MaxHP: int32(uint32(math.RoundToEven(g.HP.value(level, a)))) + b.HP,
		MaxSP: round(g.SP.value(level, a)) + b.SP,
		ATK:   atk + int32(b.ATK),
		DEF:   def + int32(b.DEF),
		MAT:   mat + int32(b.MAT),
		MDF:   mdf + int32(b.MDF),
		SPD:   spd + int32(b.SPD),
	}
}

// ChangeBanner is Player.SendEquipmentStatChanges' text, or "" when nothing changed.
func ChangeBanner(prefix string, before, after Combat) string {
	names := []string{"Max HP", "Max SP", "ATK", "DEF", "MAT", "MDF", "SPD"}
	b := []int32{before.MaxHP, before.MaxSP, before.ATK, before.DEF, before.MAT, before.MDF, before.SPD}
	a := []int32{after.MaxHP, after.MaxSP, after.ATK, after.DEF, after.MAT, after.MDF, after.SPD}
	text := ""
	for i := range names {
		if d := a[i] - b[i]; d != 0 {
			if text != "" {
				text += ", "
			}
			sign := ""
			if d > 0 {
				sign = "+"
			}
			text += fmt.Sprintf("%s %s%d", names[i], sign, d)
		}
	}
	if text == "" {
		return ""
	}
	return prefix + text
}

var ErrCannotEquip = errors.New("item cannot be equipped")

// Wear follows Inventory.TryEquip: a single bag item moves onto its native slot and any
// previously worn item takes its bag slot. Like C#, no level or body rule is checked.
func (c *Character) Wear(from byte, items map[uint16]ItemDefinition) error {
	if from < 1 || from > BagSize {
		return ErrCannotEquip
	}
	item := c.Bag[from-1]
	slot := items[item.ID].EquipSlot
	if item.Empty() || item.Locked || item.Count != 1 || slot < 1 || slot > 6 {
		return ErrCannotEquip
	}
	if c.Equipment[slot-1].Locked {
		return ErrCannotEquip
	}
	bag := c.Bag
	bag[from-1] = Item{}
	if !c.Equipment[slot-1].Empty() && !bag.CanPlace(from, c.Equipment[slot-1], items) {
		return ErrInventoryFull
	}
	bag[from-1] = c.Equipment[slot-1]
	c.Bag = bag
	c.Equipment[slot-1] = item
	return nil
}

// Unwear follows Inventory.TryUnequip: the destination bag slot must be empty.
func (c *Character) Unwear(from, to byte, definitions ...map[uint16]ItemDefinition) error {
	if from < 1 || from > 6 || to < 1 || to > BagSize || !c.Bag[to-1].Empty() || c.Bag[to-1].Locked || c.Equipment[from-1].Locked || c.Equipment[from-1].ID == 0 {
		return ErrCannotEquip
	}
	if !c.Bag.CanPlace(to, c.Equipment[from-1], inventoryDefinitions(definitions)) {
		return ErrInventoryFull
	}
	c.Bag[to-1] = c.Equipment[from-1]
	c.Bag[to-1].Count = 1
	c.Equipment[from-1] = Item{}
	return nil
}
