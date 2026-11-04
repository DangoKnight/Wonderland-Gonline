package server

import (
	"fmt"
	"slices"

	"wonderland-gonline/internal/game"
)

const petFoodCompoundIngredients = 2

// Pet foods have their own guaranteed recipe. SQL food definitions own the
// result pool; normal rank, base, failure and skill rules do not apply.
func (s *Server) compoundPetFood(ids []uint16, roll game.AlchemyRoll) (game.AlchemyOutcome, bool, error) {
	foods := 0
	for _, id := range ids {
		_, defined := s.Assets.PetFoods[id]
		if defined || s.Assets.Items[id].Type == game.ItemTypePetStatFood {
			foods++
		}
	}
	if foods == 0 {
		return game.AlchemyOutcome{}, false, nil
	}
	if len(ids) != petFoodCompoundIngredients || foods != petFoodCompoundIngredients {
		return game.AlchemyOutcome{}, true, game.ErrAlchemyIngredients
	}
	for _, id := range ids {
		if _, ok := s.Assets.Items[id]; !ok {
			return game.AlchemyOutcome{}, true, game.ErrAlchemyMaterial
		}
		if _, ok := s.Assets.PetFoods[id]; !ok {
			return game.AlchemyOutcome{}, true, game.ErrAlchemyMaterial
		}
	}
	pool := make([]uint16, 0, len(s.Assets.PetFoods))
	for id := range s.Assets.PetFoods {
		if _, ok := s.Assets.Items[id]; ok {
			pool = append(pool, id)
		}
	}
	slices.Sort(pool)
	if len(pool) == 0 {
		return game.AlchemyOutcome{}, true, game.ErrAlchemyResult
	}
	pick, err := roll(len(pool))
	if err != nil {
		return game.AlchemyOutcome{}, true, err
	}
	if pick < 0 || pick >= len(pool) {
		return game.AlchemyOutcome{}, true, fmt.Errorf("pet food random value %d outside [0,%d)", pick, len(pool))
	}
	return game.AlchemyOutcome{ItemID: pool[pick]}, true, nil
}
