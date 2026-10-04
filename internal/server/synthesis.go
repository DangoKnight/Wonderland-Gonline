package server

import (
	"context"
	"math/rand/v2"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
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
	for _, r := range rules.Rates {
		if r.Output == output && ((r.Input1 == a.ID && r.Input2 == b.ID) || (r.Input1 == b.ID && r.Input2 == a.ID)) {
			chance = r.SuccessPercent
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
	next := c.character.Clone()
	if err := next.Bag.Remove(first, alchemyUnitsPerIngredient); err != nil {
		return err
	}
	if err := next.Bag.Remove(second, alchemyUnitsPerIngredient); err != nil {
		return err
	}
	adds, err := next.Bag.Grant(game.Item{ID: output}, alchemyOutputQuantity, s.Assets.Items[output].StackLimit())
	if err != nil {
		return s.chatFeedback(c, "Synthesis failed: "+err.Error()+".")
	}
	if err = s.commit(ctx, c, next); err != nil {
		return err
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
