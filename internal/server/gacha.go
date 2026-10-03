package server

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// packContentsCommand is native AC91:1. Preview is read-only, requires no owned
// pack and always returns the current list rather than trusting the cache hint.
func (s *Server) packContentsCommand(c *Session, p []byte) error {
	if len(p) != protocol.PackContentsRequestBytes {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.PackContentsRequest {
		return ErrUnsupported
	}
	id := protocol.NewReader(p[2:]).U16()
	packet := protocol.Builder{protocol.CommandPackContents, protocol.PackContentsResponse}.U16(id).U8(protocol.PackContentsVersion)
	for _, reward := range s.Assets.GachaPacks[id].Rewards {
		packet = packet.U16(reward.ID).U8(byte(reward.Quantity))
	}
	return c.send(packet)
}

// openPackCommand ports native AC23:75/128. The operand is a full UInt16 slot;
// truncating to a byte would turn malformed large slots into valid inventory IDs.
func (s *Server) openPackCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.PackOpenRequestBytes {
		return protocol.ErrMalformed
	}
	slot := protocol.NewReader(p[2:]).U16()
	if slot < 1 || slot > game.BagSize {
		return nil
	}
	handled, err := s.openGachaPack(ctx, c, byte(slot))
	if err != nil || handled {
		return err
	}
	return c.send(headBanner("This pack has no configured rewards. Item retained."))
}

// openGachaPack is GachaManager.TryOpen with commit-before-receipt ordering.
// Caller holds worldMu. One draw consumes one pack; rewards use fresh metadata.
func (s *Server) openGachaPack(ctx context.Context, c *Session, slot byte) (bool, error) {
	if slot < 1 || slot > game.BagSize {
		return false, nil
	}
	item := c.character.Bag[slot-1]
	if !s.Assets.IsGachaPack(item.ID) {
		return false, nil
	}
	if item.Empty() || c.event != nil || c.battle != nil || c.storm || c.beach != nil || (c.character.ActiveVehicle != 0 && c.character.VehicleSlot == slot) {
		return true, nil
	}
	if !s.Assets.GachaAvailable(item.ID) {
		return true, c.send(headBanner("This pack's rewards are unavailable. Pack retained."))
	}
	// crypto/rand.Int uses rejection sampling: each weight slot is equally likely.
	draw, err := rand.Int(rand.Reader, big.NewInt(game.GachaWeightTotal))
	if err != nil {
		return true, err
	}
	reward, ok := s.Assets.GachaPacks[item.ID].RewardForRoll(int(draw.Int64()))
	if !ok {
		return true, c.send(headBanner("This pack's rewards are unavailable. Pack retained."))
	}
	limit, known := s.stackLimit(reward.ID)
	if !known || reward.Quantity <= 0 || reward.Quantity > game.GachaMaxRewardQuantity {
		return true, c.send(headBanner("This pack's rewards are unavailable. Pack retained."))
	}
	next := c.character.Clone()
	if err := next.Bag.Remove(slot, game.GachaPacksPerOpening); err != nil {
		return true, err
	}
	adds, err := next.Bag.Grant(game.Item{ID: reward.ID}, reward.Quantity, limit)
	if err != nil {
		return true, c.send(headBanner("Please free an inventory slot. Pack retained."))
	}
	if err := s.commit(ctx, c, next); err != nil {
		return true, err
	}
	return true, s.sendAll(c, [][]byte{
		{protocol.CommandInventory, protocol.InventoryRemove, slot, game.GachaPacksPerOpening},
		next.Bag.AdditionPacket(adds),
		{protocol.CommandInventory, protocol.InventoryItemUse},
		headBanner(fmt.Sprintf("Gacha: received [%s] x%d.", s.itemName(reward.ID), reward.Quantity)),
	})
}
