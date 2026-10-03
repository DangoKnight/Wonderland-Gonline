package game

import "math"

const PetTrainingPointsPerRequest = 1

// AllocatePoint spends a training point without wrapping a full attribute.
// The legacy bulk AC8 allocator retains its separate compatibility behavior.
func (p *Pet) AllocatePoint(stat byte) bool {
	var value *uint16
	switch stat {
	case StatSTR:
		value = &p.Base.Strength
	case StatCON:
		value = &p.Base.Constitution
	case StatINT:
		value = &p.Base.Intelligence
	case StatWIS:
		value = &p.Base.Wisdom
	case StatAGI:
		value = &p.Base.Agility
	default:
		return false
	}
	if *value == math.MaxUint16 {
		return false
	}
	return p.Allocate([]StatAllocation{{Stat: stat, Amount: PetTrainingPointsPerRequest}})
}
