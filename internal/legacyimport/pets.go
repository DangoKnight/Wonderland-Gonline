package legacyimport

import (
	"encoding/base64"
	"fmt"
	"gorm.io/gorm"
	"math"
	"strings"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

const legacyPetMetadataBytes = 12

func readPets(tx *gorm.DB, catalog *assets.Catalog, plan *Plan, get func(*decoder) (*game.Character, error)) error {
	values, err := rows(tx, "character_pets")
	if err != nil {
		return err
	}
	for _, raw := range values {
		d := decoder{row: raw}
		c, err := get(&d)
		if err != nil {
			return err
		}
		id := uint32(d.n("petID", math.MaxUint32))
		template, known := catalog.NPCs[uint16(game.BroadcastID(id))]
		if id == 0 || game.BroadcastID(id) > math.MaxUint16 || !known {
			return fmt.Errorf("legacy pet %d: unavailable template", id)
		}
		if d.n("potential", math.MaxUint16) != 0 {
			return fmt.Errorf("legacy pet %d: potential mapping remains unresolved", id)
		}
		p := game.Pet{ID: id, Slot: byte(d.n("slot", math.MaxUint8)), Name: d.s("petName"), Level: byte(d.n("level", game.MaxLevel)), Exp: uint32(d.n("exp", math.MaxUint32)), HP: int32(d.n("hp", math.MaxInt32)), SP: int32(d.n("sp", math.MaxInt32)), Amity: byte(d.n("amity", 100)), StatPoints: uint16(d.n("skillPoints", math.MaxUint16)), Reborn: d.n("reborn", 1) != 0, Job: byte(d.n("job", math.MaxUint8)), Battle: d.n("isBattle", 1) != 0, Base: game.Attributes{Strength: uint16(d.n("str", math.MaxUint16)), Constitution: uint16(d.n("con", math.MaxUint16)), Intelligence: uint16(d.n("int_", math.MaxUint16)), Wisdom: uint16(d.n("wis", math.MaxUint16)), Agility: uint16(d.n("agi", math.MaxUint16))}}
		if p.Slot == 0 || p.Level == 0 || len(p.Name) > 16 {
			return fmt.Errorf("legacy pet %d: invalid slot/level/name", id)
		}
		for _, b := range []byte(p.Name) {
			if b < 32 || b > 126 {
				return fmt.Errorf("legacy pet %d: incompatible native name", id)
			}
		}
		meta := make([]byte, legacyPetMetadataBytes)
		if saved := d.s("equipment_meta"); saved != "" {
			meta, err = base64.StdEncoding.DecodeString(saved)
			if err != nil || len(meta) != legacyPetMetadataBytes {
				return fmt.Errorf("legacy pet %d: malformed equipment metadata", id)
			}
		}
		for i, key := range []string{"eq_head", "eq_body", "eq_weapon", "eq_wrist", "eq_shoes", "eq_special"} {
			item := uint16(d.n(key, math.MaxUint16))
			if item != 0 {
				if _, ok := catalog.Items[item]; !ok {
					return fmt.Errorf("legacy pet %d: unavailable equipment %d", id, item)
				}
				p.Equipment[i] = game.Item{ID: item, Count: 1, Damage: meta[i*2]}
				p.Equipment[i].SetForge(meta[i*2+1])
			} else if meta[i*2] != 0 || meta[i*2+1] != 0 {
				return fmt.Errorf("legacy pet %d: metadata without equipment", id)
			}
		}
		if data := d.s("skills"); data != "" {
			for _, entry := range strings.Split(data, ";") {
				parts := strings.Split(entry, ":")
				if len(parts) != 3 {
					return fmt.Errorf("legacy pet %d: malformed skills", id)
				}
				sd := decoder{row: row{"id": parts[0], "grade": parts[1], "exp": parts[2]}}
				skill := game.PetSkill{ID: uint16(sd.n("id", math.MaxUint16)), Grade: byte(sd.n("grade", 10)), Exp: uint32(sd.n("exp", math.MaxUint32))}
				if _, ok := catalog.Skills[skill.ID]; !ok || skill.Grade == 0 || sd.err != nil {
					return fmt.Errorf("legacy pet %d: invalid skill", id)
				}
				for _, old := range p.Skills {
					if old.ID == skill.ID {
						return fmt.Errorf("legacy pet %d: duplicate skill", id)
					}
				}
				p.Skills = append(p.Skills, skill)
			}
		}
		p.EnsureSkills(game.PetTemplate{Type: template.Type, Skills: template.Skills}, func(id uint16) bool { _, ok := catalog.Skills[id]; return ok })
		combat := p.Combat(catalog.Items)
		p.MaxHP, p.MaxSP = combat.MaxHP, combat.MaxSP
		if p.HP > p.MaxHP || p.SP > p.MaxSP {
			return fmt.Errorf("legacy pet %d: vitals exceed growth limits", id)
		}
		hotel := d.n("isHotel", 2)
		riding := d.n("isRide", 1) != 0
		if d.err != nil {
			return fmt.Errorf("legacy pet %d: %w", id, d.err)
		}
		for _, group := range [][]game.Pet{c.Pets, c.ReservePets, c.HotelPets} {
			for _, old := range group {
				if game.SamePet(old.ID, id) {
					return fmt.Errorf("legacy character %d: duplicate companion identity", c.ID)
				}
			}
		}
		target := &c.Pets
		limit := game.MaxPets
		if hotel == 1 {
			target = &c.HotelPets
			limit = game.MaxHotelPets
		}
		if hotel == 2 {
			target = &c.ReservePets
			limit = math.MaxUint8
		}
		if int(p.Slot) > limit || len(*target) >= limit {
			return fmt.Errorf("legacy character %d: pet container full", c.ID)
		}
		for _, old := range *target {
			if old.Slot == p.Slot {
				return fmt.Errorf("legacy character %d: duplicate pet slot", c.ID)
			}
		}
		if hotel != 0 && (p.Battle || riding) {
			return fmt.Errorf("legacy pet %d: inactive pet has active flags", id)
		}
		if p.Battle {
			if c.ActivePet != 0 {
				return fmt.Errorf("legacy character %d: multiple active pets", c.ID)
			}
			c.ActivePet = game.BroadcastID(id)
		}
		if riding {
			if c.ActiveMount != 0 {
				return fmt.Errorf("legacy character %d: multiple mounts", c.ID)
			}
			c.ActiveMount = game.BroadcastID(id)
		}
		*target = append(*target, p)
		plan.Report.Pets++
	}
	return nil
}
