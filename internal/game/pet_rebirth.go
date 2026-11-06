package game

import "math"

const (
	PetRebirthRequiredLevel            = 100
	PetRebirthRequiredAmity            = 100
	PetRebirthBonusStatPoints          = 50
	PetRebirthStartingLevel            = 1
	CompanionRebirthRequiredAmity      = 60
	CompanionRebirthEligible      byte = 1
	CompanionRebirthLowLevel      byte = 2
	CompanionRebirthNoFellow      byte = 5
	CompanionRebirthAlreadyReborn byte = 6
)

// CompanionRebirthState is EVE condition 21. The following companion takes
// precedence over the battle selection, matching the altar's native form.
func (c Character) CompanionRebirthState() byte {
	index, found := c.Pet(c.ActivePet)
	if !found || c.ActivePet == 0 {
		found = false
		for i, p := range c.Pets {
			if p.ID != 0 && p.Battle {
				index, found = i, true
				break
			}
		}
	}
	if !found {
		return CompanionRebirthNoFellow
	}
	p := c.Pets[index]
	if p.Reborn {
		return CompanionRebirthAlreadyReborn
	}
	if p.Level < PetRebirthRequiredLevel {
		return CompanionRebirthLowLevel
	}
	return CompanionRebirthEligible
}

// RebirthAtAltar preserves learned skills and allocated points. Unlike AC69,
// the native AC44 operation requires bare equipment and grants no point bonus.
func (p *Pet) RebirthAtAltar(items map[uint16]ItemDefinition, templates ...PetTemplate) bool {
	if p.ID == 0 || p.Reborn || p.Level < PetRebirthRequiredLevel || p.Amity < CompanionRebirthRequiredAmity {
		return false
	}
	for _, item := range p.Equipment {
		if item.ID != 0 {
			return false
		}
	}
	p.Reborn, p.Level, p.Exp = true, PetRebirthStartingLevel, 0
	p.Normalize(items, true, templates...)
	return true
}

// Rebirth follows AC69's ascension policy. Keep learned skills, base attributes,
// equipment, job and battle selection; refill derived vitals at the new level.
// Reject bonus overflow rather than wrapping away existing stat points.
func (p *Pet) Rebirth(items map[uint16]ItemDefinition, templates ...PetTemplate) bool {
	if p.ID == 0 || p.Reborn || p.Level < PetRebirthRequiredLevel || p.Amity < PetRebirthRequiredAmity || p.StatPoints > math.MaxUint16-PetRebirthBonusStatPoints {
		return false
	}
	p.Reborn = true
	p.Level = PetRebirthStartingLevel
	p.Exp = 0
	p.StatPoints += PetRebirthBonusStatPoints
	p.Amity = PetRebirthRequiredAmity
	p.Normalize(items, true, templates...)
	return true
}
