package server

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

func forgeRoll() (bool, error) {
	var value [1]byte
	_, err := rand.Read(value[:])
	return value[0]&1 == 0, err
}

// forgeReject clears the native pending selection before its explanation.
func forgeReject(c *Session, result byte, message string) error {
	if err := c.send([]byte{protocol.CommandMall, protocol.MallForgeResult, result}); err != nil {
		return err
	}
	return c.send(headBanner(message))
}
func (s *Server) forgeCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.MallForgeRequestBytes {
		return protocol.ErrMalformed
	}
	return s.forgeItem(ctx, c, p[2], forgeRoll)
}

// forgeItem ports MallForgingManager. Caller holds worldMu; point payment and
// metadata changes use the same authoritative transaction as mall purchases.
func (s *Server) forgeItem(ctx context.Context, c *Session, slot byte, roll func() (bool, error)) error {
	reject := func(message string) error { return forgeReject(c, protocol.MallForgeRejected, message) }
	if c.event != nil || c.storm || c.beach != nil {
		return reject("Forging is unavailable right now.")
	}
	if slot < 1 || slot > game.BagSize {
		return reject("Select equipment in your inventory.")
	}
	source := c.character.Bag[slot-1]
	if source.Empty() || source.Locked || source.Count != 1 {
		return reject("Select one equipment item in your inventory.")
	}
	if c.character.ActiveVehicle != 0 && slot == c.character.VehicleSlot {
		return reject("Land the vehicle before forging it.")
	}
	definition, known := s.Assets.Items[source.ID]
	if !known {
		return reject("Equipment data is unavailable.")
	}
	if upgrade, ok := s.Assets.Forging.Upgrades[source.ID]; ok {
		if upgrade.Next == 0 {
			return reject("This equipment is already at its maximum upgrade.")
		}
		target, known := s.Assets.Items[upgrade.Next]
		if !known || target.EquipSlot < 1 || int(target.EquipSlot) > len(c.character.Equipment) || target.EquipSlot != definition.EquipSlot {
			return reject("Upgrade data is unavailable. Equipment and scrolls retained.")
		}
		next := c.character.Clone()
		needed := int(upgrade.Scrolls)
		var costs []game.Addition
		for index, item := range next.Bag {
			if needed == 0 {
				break
			}
			if item.ID != game.StrongScrollItemID || item.Empty() || (next.ActiveVehicle != 0 && next.VehicleSlot == byte(index+1)) {
				continue
			}
			take := byte(min(needed, int(item.Count)))
			if err := next.Bag.Remove(byte(index+1), take); err != nil {
				return err
			}
			costs = append(costs, game.Addition{Slot: byte(index + 1), Count: take})
			needed -= int(take)
		}
		if needed != 0 {
			return forgeReject(c, protocol.MallForgeInsufficientScrolls, fmt.Sprintf("This upgrade requires %d Strong Scroll(s).", upgrade.Scrolls))
		}
		next.Bag[slot-1] = game.Item{ID: upgrade.Next, Count: 1, Damage: source.Damage}
		if err := s.commit(ctx, c, next); err != nil {
			return err
		}
		var packets [][]byte
		for _, cost := range costs {
			packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, cost.Slot, cost.Count})
		}
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, 1}, next.Bag.AdditionPacket([]game.Addition{{Slot: slot, Count: 1}}), []byte{protocol.CommandMall, protocol.MallForgeResult, protocol.MallForgeUpgradeSucceeded})
		return s.sendAll(c, packets)
	}
	if !s.Assets.Forging.PointItems[source.ID] || !definition.CanPointForge() {
		return reject("This equipment cannot be forged.")
	}
	if source.Forge() >= game.MaxForgeProgress {
		return reject("This equipment is already at its maximum forging progress.")
	}
	var success bool
	next, balances, err := s.Store.PurchaseMall(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, false, game.PointForgeCost, func(character *game.Character) error {
		if err := game.PreserveItemLocks(*c.character, character); err != nil {
			return err
		}
		if character.Bag[slot-1] != source {
			return game.ErrInvalidItem
		}
		var err error
		success, err = roll()
		if err != nil {
			return err
		}
		if success {
			character.Bag[slot-1].SetForge(source.Forge() + 1)
		}
		return nil
	})
	if errors.Is(err, store.ErrMallFunds) {
		if err := forgeReject(c, protocol.MallForgeInsufficientPoints, "Forging requires 3 IM Points."); err != nil {
			return err
		}
		return s.sendMallBalances(ctx, c)
	}
	if errors.Is(err, game.ErrInvalidItem) {
		return reject("The selected item changed. Please select it again.")
	}
	if err != nil {
		return err
	}
	*c.character = next
	c.account.IM, c.account.IMBonus = balances.Points, balances.Bonus
	reply := []byte{protocol.CommandMall, protocol.MallForgeResult, protocol.MallForgeRollFailed}
	if success {
		reply = []byte{protocol.CommandMall, protocol.MallForgeResult, protocol.MallForgeProgressSucceeded, slot, next.Bag[slot-1].Forge(), 0}
	}
	return s.sendAll(c, append([][]byte{reply}, mallBalancePackets(balances)...))
}
