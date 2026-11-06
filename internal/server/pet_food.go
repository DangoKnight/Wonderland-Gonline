package server

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"slices"

	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
)

var errPetStatFood = errors.New("Select a matching pet with fewer than 1200 stat-food uses.")

func (s *Server) usePetStatFood(ctx context.Context, c *Session, slot, count, target byte) (bool, error) {
	return s.usePetStatFoodWithRoll(ctx, c, slot, count, target, func(n int) (int, error) {
		v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
		if err != nil {
			return 0, err
		}
		return int(v.Int64()), nil
	})
}

// Caller holds worldMu. Debit, attribute gain and lifetime cap commit together.
func (s *Server) usePetStatFoodWithRoll(ctx context.Context, c *Session, slot, count, target byte, roll func(int) (int, error)) (bool, error) {
	if slot < 1 || slot > game.BagSize {
		return false, nil
	}
	expected := c.character.Bag[slot-1]
	food, known := s.Assets.PetFoods[expected.ID]
	if !known {
		return false, nil
	}
	if !gmIdle(c) {
		return true, nil
	}
	i := s.clientPet(c, target)
	if count == 0 || target == 0 || i < 0 {
		return true, c.send(serverwire.HeadBanner(errPetStatFood.Error()))
	}
	petID := c.character.Pets[i].ID
	var used byte
	var result game.Pet
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		item := next.Bag[slot-1]
		i := slices.IndexFunc(next.Pets, func(p game.Pet) bool { return p.ID == petID })
		if i < 0 || item.Empty() || item.Locked || item.ID != expected.ID {
			return errPetStatFood
		}
		p := &next.Pets[i]
		if !slices.ContainsFunc(food.Targets, func(id uint32) bool { return game.SamePet(id, p.ID) }) || p.StatFoodUses >= game.PetStatFoodUseLimit {
			return errPetStatFood
		}
		limit := min(int(count), int(item.Count), game.PetStatFoodUseLimit-int(p.StatFoodUses))
		template, known := s.World.Template(game.BroadcastID(p.ID))
		old := p.Combat(s.Assets.Items, template)
		hp, sp := p.HP, p.SP
		for n := 0; n < limit; n++ {
			ok, err := p.AddFoodStat(template, known, roll, game.PetGrowthOptions{Formula: s.petGrowthFormula, Items: s.Assets.Items})
			if err != nil {
				return err
			}
			if !ok {
				break
			}
			used++
		}
		if used == 0 {
			return errPetStatFood
		}
		if err := next.Bag.Remove(slot, used); err != nil {
			return err
		}
		p.Normalize(s.Assets.Items, false, template)
		p.HP = int32(min(int64(p.MaxHP), max(0, int64(hp))+max(0, int64(p.MaxHP)-int64(old.MaxHP))))
		p.SP = int32(min(int64(p.MaxSP), max(0, int64(sp))+max(0, int64(p.MaxSP)-int64(old.MaxSP))))
		result = *p
		return nil
	}, c.character.Clone())
	if errors.Is(err, errPetStatFood) {
		return true, c.send(serverwire.HeadBanner(err.Error()))
	}
	if err != nil {
		return true, err
	}
	s.adoptSavedCharacter(c, next)
	packets := [][]byte{{protocol.CommandInventory, protocol.InventoryRemove, slot, used}}
	packets = append(packets, result.ProgressionPackets(target, s.Assets.Items, s.petTemplate(result.ID))...)
	packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryItemUse}, serverwire.HeadBanner(fmt.Sprintf("%s: stat food %d/%d", s.companionName(result.ID, result.Name), result.StatFoodUses, game.PetStatFoodUseLimit)))
	return true, s.sendAll(c, packets)
}
