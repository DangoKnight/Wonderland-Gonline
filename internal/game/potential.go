package game

import (
	"errors"
	"math"
)

const (
	PotentialMaximum               = 12
	ItemPotentialPill       uint16 = 34269
	ItemGoldenPotentialPill uint16 = 34289
	ItemSuperPotentialPill  uint16 = 34350
	GoldenPotentialMinimum         = 10
	PotentialPillQuantity   byte   = 1
)

var (
	ErrPotentialPill          = errors.New("invalid potential pill or target")
	ErrPotentialMaximum       = errors.New("potential is already at its maximum")
	ErrPotentialGoldenMinimum = errors.New("golden potential pills require potential level 10")
	ErrPotentialRoll          = errors.New("invalid potential pill probability roll")
)

// PotentialBonus is the cumulative bonus to each attribute, recovered from
// aLogin's uint16 table at 0x4b7850. Preserve out-of-range saved metadata while
// bounding its stat contribution to the native maximum.
func PotentialBonus(level uint16) uint16 {
	bonuses := [...]uint16{0, 1, 2, 3, 4, 6, 9, 13, 18, 24, 31, 39, 48}
	return bonuses[min(level, PotentialMaximum)]
}

func (a Attributes) withPotential(level uint16) Attributes {
	bonus := uint32(PotentialBonus(level))
	add := func(v uint16) uint16 { return uint16(min(uint32(v)+bonus, math.MaxUint16)) }
	return Attributes{Strength: add(a.Strength), Constitution: add(a.Constitution), Intelligence: add(a.Intelligence), Wisdom: add(a.Wisdom), Agility: add(a.Agility)}
}

func (p Pet) Attributes() Attributes { return p.Base.withPotential(p.Potential) }

// PotentialSuccessPercent is indexed by the current level: the official WLRI
// table describes the next level being attempted. These are compiled enhancement
// coefficients, like the native cumulative attribute bonus above.
func PotentialSuccessPercent(itemID, level uint16) (int, error) {
	switch itemID {
	case ItemPotentialPill, ItemGoldenPotentialPill, ItemSuperPotentialPill:
	default:
		return 0, ErrPotentialPill
	}
	if level >= PotentialMaximum {
		return 0, ErrPotentialMaximum
	}
	if itemID == ItemGoldenPotentialPill && level < GoldenPotentialMinimum {
		return 0, ErrPotentialGoldenMinimum
	}
	if itemID == ItemSuperPotentialPill {
		return PotentialChanceScale, nil
	}
	chances := [...]int{100, 100, 100, 70, 60, 55, 55, 50, 35, 20, 18, 15}
	return chances[level], nil
}

const PotentialChanceScale = 100

// NextPotential consumes a zero-based uniform percentage roll. A valid attempt
// consumes a pill on either outcome. Normal failure loses one level; golden
// failure preserves it. Guaranteed attempts need no random number generation.
func NextPotential(itemID, level uint16, roll int) (uint16, error) {
	chance, err := PotentialSuccessPercent(itemID, level)
	if err != nil {
		return level, err
	}
	if roll < 0 || roll >= PotentialChanceScale {
		return level, ErrPotentialRoll
	}
	if roll < chance {
		return level + 1, nil
	}
	if itemID == ItemPotentialPill {
		return level - 1, nil
	}
	return level, nil
}
