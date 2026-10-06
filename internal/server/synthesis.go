package server

import (
	"context"
	"fmt"
	"math/rand/v2"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

const synthesisPercentScale = 100
const synthesisLightEffect = 60020

// Public /compound follows AlchemyManager's chance-based synthesis. Native
// AC23 and AC40 retain their independently verified deterministic behavior.
func (s *Server) synthesize(ctx context.Context, c *Session, first, second byte) error {
	if first < 1 || first > game.BagSize || second < 1 || second > game.BagSize || first == second {
		return s.chatFeedback(c, "Choose two different occupied bag slots.")
	}
	if c.character.ActiveVehicle != 0 && (first == c.character.VehicleSlot || second == c.character.VehicleSlot) {
		return s.chatFeedback(c, "Dismount before compounding this vehicle.")
	}
	a, b := c.character.Bag[first-1], c.character.Bag[second-1]
	if a.Empty() || b.Empty() {
		return s.chatFeedback(c, "Choose two occupied bag slots.")
	}
	if _, known := s.Assets.Items[a.ID]; !known {
		return s.chatFeedback(c, "Unknown synthesis ingredient.")
	}
	if _, known := s.Assets.Items[b.ID]; !known {
		return s.chatFeedback(c, "Unknown synthesis ingredient.")
	}
	rules := s.Assets.Economy.Synthesis
	output, matched := s.Assets.AlchemyResult(a.ID, b.ID)
	chance := rules.DefaultSuccessPercent
	var fee uint32
	for _, r := range rules.Rates {
		if r.Output == output && ((r.Input1 == a.ID && r.Input2 == b.ID) || (r.Input1 == b.ID && r.Input2 == a.ID)) {
			chance = r.SuccessPercent
			fee = r.Fee
			break
		}
	}
	// Verify both potential outputs before spending either ingredient.
	if _, known := s.Assets.Items[rules.FailureItemID]; !known {
		return s.chatFeedback(c, "The synthesis failure item definition is unavailable.")
	}
	if matched {
		if _, known := s.Assets.Items[output]; !known {
			return s.chatFeedback(c, "The synthesis output definition is unavailable.")
		}
	}
	success := matched && rand.Float64()*synthesisPercentScale < chance
	if !success {
		output = rules.FailureItemID
	}

	var adds []game.Addition
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		if next.Bag[first-1] != a || next.Bag[second-1] != b {
			return game.ErrTradeChanged
		}
		if next.Gold < fee {
			return fmt.Errorf("not enough gold")
		}
		if err := next.Bag.Remove(first, alchemyUnitsPerIngredient); err != nil {
			return err
		}
		if err := next.Bag.Remove(second, alchemyUnitsPerIngredient); err != nil {
			return err
		}
		next.Gold -= fee
		var err error
		adds, err = next.Bag.Grant(game.Item{ID: output}, alchemyOutputQuantity, s.Assets.Items[output].StackLimit(), s.Assets.Items)
		return err
	}, *c.character)
	if err != nil {
		return s.chatFeedback(c, "Synthesis failed: "+err.Error()+".")
	}
	s.adoptSavedCharacter(c, next)
	if fee > 0 {
		s.sendOrClose(c, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold))
	}

	if err = s.sendAll(c, [][]byte{{protocol.CommandInventory, protocol.InventoryRemove, first, alchemyUnitsPerIngredient}, {protocol.CommandInventory, protocol.InventoryRemove, second, alchemyUnitsPerIngredient}, next.Bag.AdditionPacket(adds)}); err != nil {
		return err
	}
	if success {
		p := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateRepairEffect}.U32(c.character.ID).U16(synthesisLightEffect)
		s.broadcastWorld(c, p)
		s.sendOrClose(c, p)
		return s.chatFeedback(c, "Synthesis succeeded.")
	}
	return s.chatFeedback(c, "Synthesis failed; the materials turned into charcoal.")
}
