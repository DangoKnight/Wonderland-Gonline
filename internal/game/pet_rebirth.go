package game

import "math"

const (
	PetRebirthRequiredLevel   = 100
	PetRebirthRequiredAmity   = 100
	PetRebirthBonusStatPoints = 50
	PetRebirthStartingLevel   = 1
)

// Rebirth follows AC69's ascension policy. Keep learned skills, base attributes,
// equipment, job and battle selection; refill derived vitals at the new level.
// Reject bonus overflow rather than wrapping away existing stat points.
func (p *Pet) Rebirth(items map[uint16]ItemDefinition) bool {
	if p.ID == 0 || p.Reborn || p.Level < PetRebirthRequiredLevel || p.Amity < PetRebirthRequiredAmity || p.StatPoints > math.MaxUint16-PetRebirthBonusStatPoints {
		return false
	}
	p.Reborn = true
	p.Level = PetRebirthStartingLevel
	p.Exp = 0
	p.StatPoints += PetRebirthBonusStatPoints
	p.Amity = PetRebirthRequiredAmity
	p.Normalize(items, true)
	return true
}
