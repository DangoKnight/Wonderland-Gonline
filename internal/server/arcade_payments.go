package server

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"time"

	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

const (
	arcadeFunTokenItemID  = 34007
	arcadePlayTokenItemID = 34008
	arcadeVoucherItemID   = 34360
	arcadeVoucherCount    = 1
)

// Arcade ownership and the animation throttle are transient. Durable payment
// and rewards use Store.PurchaseMall's single gameplay database transaction.
// Every caller holds worldMu; AC57 results never award paid prizes.
type arcadeSession struct {
	kind  byte
	mapID uint16
	event *eventSession
}

type arcadePayment struct{ mode, ticketSlot byte }

func parseArcadePayment(p []byte) (arcadePayment, error) {
	if len(p) < 2 {
		return arcadePayment{}, protocol.ErrMalformed
	}
	payment := arcadePayment{mode: protocol.ArcadePayPoints}
	switch p[1] {
	case protocol.ArcadeEgg, protocol.ArcadeEgg2:
		if len(p) != protocol.ArcadeEggRequestBytes {
			return payment, protocol.ErrMalformed
		}
		payment.mode = p[2]
	case protocol.ArcadeSlots, protocol.ArcadeSlots3:
		if len(p) != protocol.ArcadeSlotsRequestBytes {
			return payment, protocol.ErrMalformed
		}
		payment.mode, payment.ticketSlot = p[2], p[3]
	case protocol.ArcadeSlots2:
		if len(p) != protocol.ArcadeEggRequestBytes {
			return payment, protocol.ErrMalformed
		}
		payment.ticketSlot = p[2]
	default:
		return payment, ErrUnsupported
	}
	if payment.mode < protocol.ArcadePayPoints || payment.mode > protocol.ArcadePayPlayTokens || payment.ticketSlot > game.BagSize || (payment.ticketSlot != 0 && payment.mode != protocol.ArcadePayPoints) {
		return payment, protocol.ErrMalformed
	}
	return payment, nil
}

func (s *Server) arcadeCommand(ctx context.Context, c *Session, p []byte) error {
	payment, err := parseArcadePayment(p)
	if err != nil {
		return err
	}
	reject := func(message string) error {
		return s.sendAll(c, [][]byte{headBanner("Arcade: " + message), {protocol.CommandArcadeGame, p[1], protocol.ArcadeRejected}})
	}
	active := c.arcade
	if active == nil || active.kind != p[1] || active.mapID != c.character.Map || active.event != c.event || (active.event != nil && active.event.onMinigame == nil) {
		return reject("Reopen the machine before playing.")
	}
	if !c.ready || c.battle != nil || c.trade != nil || c.storm || c.beach != nil || c.tentOwner != 0 {
		return reject("Finish the current interaction before playing.")
	}
	if time.Now().Before(c.arcadeNextPurchaseAt) {
		return reject("Wait for the current play to finish.")
	}
	var definition *assets.ArcadeGame
	for i := range s.Assets.Arcades {
		if s.Assets.Arcades[i].Kind == p[1] {
			definition = &s.Assets.Arcades[i]
			break
		}
	}
	if definition == nil || !definition.Enabled || definition.TotalWeight() <= 0 {
		return reject("This machine is unavailable. No payment taken.")
	}
	roll, err := rand.Int(rand.Reader, big.NewInt(definition.TotalWeight()))
	if err != nil {
		return err
	}
	reward, ok := definition.RewardForRoll(roll.Int64())
	item, known := s.Assets.Items[reward.ItemID]
	if !ok || !known || reward.ItemID != assets.ArcadePrizeItem(definition.Kind, reward.Index) {
		return reject("This reward is unavailable. No payment taken.")
	}
	cost := definition.PointCost
	tokenID := uint16(0)
	if payment.mode != protocol.ArcadePayPoints {
		if definition.TokenCount == 0 {
			return reject("This machine does not accept tokens.")
		}
		tokenID = arcadeFunTokenItemID
		if payment.mode == protocol.ArcadePayPlayTokens {
			tokenID = arcadePlayTokenItemID
		}
		cost = 0
	}
	if payment.ticketSlot != 0 {
		cost = 0
	}
	var removals []game.Addition
	var additions []game.Addition
	next, balances, err := s.Store.PurchaseMall(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, false, cost, func(character *game.Character) error {
		if err := game.PreserveItemLocks(*c.character, character); err != nil {
			return err
		}
		remove := func(slot, count byte) error {
			if err := character.Bag.Remove(slot, count); err != nil {
				return err
			}
			removals = append(removals, game.Addition{Slot: slot, Count: count})
			return nil
		}
		if payment.ticketSlot != 0 {
			if character.Bag[payment.ticketSlot-1].ID != arcadeVoucherItemID {
				return game.ErrInvalidItem
			}
			if err := remove(payment.ticketSlot, arcadeVoucherCount); err != nil {
				return err
			}
		}
		if tokenID != 0 {
			remaining := definition.TokenCount
			for i, token := range character.Bag {
				if token.ID != tokenID || token.Locked || token.Empty() {
					continue
				}
				count := min(remaining, token.Count)
				if count > 0 {
					if err := remove(byte(i+1), count); err != nil {
						return err
					}
					remaining -= count
				}
			}
			if remaining != 0 {
				return game.ErrInvalidItem
			}
		}
		// Require room for every possible outcome. Otherwise a full bag with
		// one matching stack could force free rerolls until that prize wins.
		for _, candidate := range definition.Rewards {
			if candidate.Weight == 0 {
				continue
			}
			candidateItem, known := s.Assets.Items[candidate.ItemID]
			if !known {
				return game.ErrInvalidItem
			}
			planned := character.Bag
			if _, err := planned.Grant(game.Item{ID: candidate.ItemID}, int(candidate.Quantity), candidateItem.StackLimit()); err != nil {
				return err
			}
		}
		var err error
		additions, err = character.Bag.Grant(game.Item{ID: reward.ItemID}, int(reward.Quantity), item.StackLimit())
		return err
	})
	switch {
	case errors.Is(err, store.ErrMallFunds):
		return reject("Insufficient points. No payment taken.")
	case errors.Is(err, game.ErrInventoryFull):
		return reject("Make room in your inventory. No payment taken.")
	case errors.Is(err, game.ErrInvalidItem), errors.Is(err, game.ErrItemLocked), errors.Is(err, store.ErrMallPurchase):
		return reject("Check your payment items and inventory. No payment taken.")
	case err != nil:
		return err
	}
	// Commit precedes all success packets. A failed socket cannot undo the durable
	// play or enable a second delivery from the same cached session state.
	s.adoptSavedCharacter(c, next)
	c.account.IM, c.account.IMBonus = balances.Points, balances.Bonus
	c.arcadeNextPurchaseAt = time.Now().Add(time.Duration(definition.CooldownMilliseconds) * time.Millisecond)
	packets := [][]byte{}
	for _, r := range removals {
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, r.Slot, r.Count})
	}
	packets = append(packets, next.Bag.AdditionPacket(additions))
	packets = append(packets, mallBalancePackets(balances)...)
	outcome := []byte{protocol.CommandArcadeGame, definition.Kind, protocol.ArcadeAccepted, reward.Index}
	if definition.Kind != protocol.ArcadeEgg && definition.Kind != protocol.ArcadeEgg2 {
		outcome = append(outcome, reward.Reels[:]...)
	}
	outcome = append(outcome, reward.Quantity)
	packets = append(packets, outcome)
	return s.sendAll(c, packets)
}
