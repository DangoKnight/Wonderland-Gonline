package game

const PetStatFoodUseLimit = 1200
const ItemTypePetStatFood = 20

// AddFoodStat grants one permanent point without wrapping a capped attribute.
// The owner commits the item debit and this lifetime counter together in SQL.
func (p *Pet) AddFoodStat(t PetTemplate, known bool, roll func(int) (int, error), options ...PetGrowthOptions) (bool, error) {
	if p.StatFoodUses >= PetStatFoodUseLimit {
		return false, nil
	}
	added, err := p.addGrowthStat(t, known, roll, options...)
	if err != nil || !added {
		return false, err
	}
	p.StatFoodUses++
	return true, nil
}
