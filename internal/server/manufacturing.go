package server

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

const (
	manufactureChanceScale      = 100
	manufactureSparkleEffect    = 60018
	manufactureDefaultWorkbench = "Forge"
)

type gatheringRun struct {
	Kind   byte
	Map    uint16
	X, Y   uint16
	NextAt time.Time
}

// planManufacturing removes exact recipe costs from a copy and grants output
// before any save. Repeated input IDs consume their combined requirement.
func planManufacturing(c game.Character, r assets.ManufacturingRecipe, items map[uint16]game.ItemDefinition) (game.Character, []game.Addition, []game.Addition, error) {
	next := c.Clone()
	var removes []game.Addition
	for _, input := range r.Inputs {
		if input.ItemID == 0 {
			continue
		}
		if _, known := items[input.ItemID]; !known {
			return game.Character{}, nil, nil, game.ErrInvalidItem
		}
		remaining := int(input.Count)
		for i := range next.Bag {
			item := next.Bag[i]
			if item.ID != input.ItemID || item.Empty() || item.Locked {
				continue
			}
			if next.ActiveVehicle == item.ID {
				return game.Character{}, nil, nil, game.ErrInvalidItem
			}
			count := byte(min(remaining, int(item.Count)))
			if count == 0 {
				continue
			}
			if err := next.Bag.Remove(byte(i+1), count); err != nil {
				return game.Character{}, nil, nil, err
			}
			removes = append(removes, game.Addition{Slot: byte(i + 1), Count: count})
			remaining -= int(count)
		}
		if remaining != 0 {
			return game.Character{}, nil, nil, game.ErrInvalidItem
		}
	}
	output, known := items[r.Output.ItemID]
	if !known {
		return game.Character{}, nil, nil, game.ErrInvalidItem
	}
	adds, err := next.Bag.Grant(game.Item{ID: r.Output.ItemID}, int(r.Output.Count), output.StackLimit(), items)
	return next, removes, adds, err
}
func (s *Server) manufacture(ctx context.Context, c *Session, bench string, inputs [2]assets.ManufacturingInput) (bool, error) {
	if !gmIdle(c) || c.stall != nil {
		return false, nil
	}
	for _, r := range s.Assets.Economy.Manufacturing {
		if !strings.EqualFold(r.Workbench, bench) || r.Inputs[0].ItemID != inputs[0].ItemID || r.Inputs[1].ItemID != inputs[1].ItemID || inputs[0].Count < r.Inputs[0].Count || inputs[1].Count < r.Inputs[1].Count {
			continue
		}

		var removes, adds []game.Addition
		var succeeded bool
		next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(stored *game.Character) error {
			if stored.Gold < r.Fee {
				return fmt.Errorf("not enough gold")
			}
			var err error
			removes, err = store.RemoveManufacturingInputs(stored, r.Inputs[:])
			if err != nil {
				return err
			}
			stored.Gold -= r.Fee
			chance := float64(manufactureChanceScale)
			if r.SuccessPercent != nil {
				chance = *r.SuccessPercent
			}
			succeeded = rand.Float64()*manufactureChanceScale < chance
			if !succeeded {
				return nil
			}
			definition, known := s.Assets.Items[r.Output.ItemID]
			if !known {
				return game.ErrInvalidItem
			}
			adds, err = stored.Bag.Grant(game.Item{ID: r.Output.ItemID}, int(r.Output.Count), definition.StackLimit(), s.Assets.Items)
			return err
		}, *c.character)
		if err != nil {
			return false, s.chatFeedback(c, "Manufacturing failed: "+err.Error()+".")
		}
		s.adoptSavedCharacter(c, next)

		for _, remove := range removes {
			s.sendOrClose(c, []byte{protocol.CommandInventory, protocol.InventoryRemove, remove.Slot, remove.Count})
		}
		if r.Fee > 0 {
			s.sendOrClose(c, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(c.character.Gold))
		}
		if !succeeded {
			return false, s.chatFeedback(c, "Manufacturing attempt failed; materials and fee were consumed.")
		}
		s.sendOrClose(c, c.character.Bag.AdditionPacket(adds))
		sparkle := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateRepairEffect}.U32(c.character.ID).U16(manufactureSparkleEffect)
		s.broadcastWorld(c, sparkle)
		s.sendOrClose(c, sparkle)
		return true, nil
	}
	return false, s.chatFeedback(c, "No matching manufacturing recipe.")
}
func (s *Server) manufactureCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.ManufactureRequestBytes {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	inputs := [2]assets.ManufacturingInput{{ItemID: r.U16(), Count: r.U8()}, {ItemID: r.U16(), Count: r.U8()}}
	success, err := s.manufacture(ctx, c, manufactureDefaultWorkbench, inputs)
	if err != nil {
		return err
	}
	result := byte(protocol.ManufactureFailed)
	if success {
		result = protocol.ManufactureSucceeded
	}
	return c.send([]byte{protocol.CommandManufacture, p[1], result})
}
func (s *Server) craftingChatCommand(ctx context.Context, c *Session, name string, args []string, text string) (bool, error) {
	switch name {
	case "manufacture":
		if len(args) < 5 {
			return true, s.chatFeedback(c, "Usage: /manufacture <workbench> <item1> <count1> <item2> <count2>")
		}
		var values [4]uint64
		for i := range values {
			bits := 16
			if i%2 != 0 {
				bits = 8
			}
			value, err := strconv.ParseUint(args[len(args)-4+i], 10, bits)
			if err != nil {
				return true, s.chatFeedback(c, "Invalid manufacturing item/count fields.")
			}
			values[i] = value
		}
		_, err := s.manufacture(ctx, c, strings.Join(args[:len(args)-4], " "), [2]assets.ManufacturingInput{{ItemID: uint16(values[0]), Count: byte(values[1])}, {ItemID: uint16(values[2]), Count: byte(values[3])}})
		return true, err
	case "compound", "synthesize":
		if len(args) != 2 {
			return true, s.chatFeedback(c, "Usage: /compound <bag slot1> <bag slot2>")
		}
		a, ea := strconv.ParseUint(args[0], 10, 8)
		b, eb := strconv.ParseUint(args[1], 10, 8)
		if ea != nil || eb != nil {
			return true, s.chatFeedback(c, "Invalid bag slots.")
		}
		if !gmIdle(c) {
			return true, s.chatFeedback(c, "Finish active interactions before compounding.")
		}
		return true, s.synthesize(ctx, c, byte(a), byte(b))
	case "fish":
		if len(args) != 0 {
			return true, s.chatFeedback(c, "Usage: /fish")
		}
		return true, s.startFishing(ctx, c, 0)
	case "mine", "chop":
		if len(args) != 0 {
			return true, s.chatFeedback(c, "Usage: /"+name)
		}
		if !gmIdle(c) || c.stall != nil || c.character.HP == 0 {
			return true, s.chatFeedback(c, "Finish active interactions before gathering.")
		}
		kind := assets.GatheringFishing
		if name == "mine" {
			kind = assets.GatheringMining
		}
		if name == "chop" {
			kind = assets.GatheringWoodcutting
		}
		pool := s.gatheringPool(kind)
		if pool == nil {
			return true, s.chatFeedback(c, "No gathering pool is configured.")
		}
		if !s.validGatheringPool(pool) {
			return true, s.chatFeedback(c, "The gathering pool contains unavailable item definitions.")
		}
		s.stopFishing(c)
		c.gathering = &gatheringRun{Kind: kind, Map: c.character.Map, X: c.character.X, Y: c.character.Y, NextAt: time.Now().Add(time.Duration(pool.IntervalSeconds) * time.Second)}
		// The reference's short AC5:12 fishing animation conflicts with the
		// verified native model-transform payload. Keep animations pending until
		// their real native layout is known; gathering rewards do not depend on it.

		return true, s.chatFeedback(c, "Gathering started. Use /stop to stop.")
	case "stop":
		if len(args) != 0 {
			return true, s.chatFeedback(c, "Usage: /stop")
		}
		c.gathering = nil
		s.stopFishing(c)
		return true, s.chatFeedback(c, "Gathering stopped.")
	}
	return false, nil
}
func (s *Server) gatheringPool(kind byte) *assets.GatheringPool {
	for i := range s.Assets.Economy.Gathering {
		if s.Assets.Economy.Gathering[i].Kind == kind {
			return &s.Assets.Economy.Gathering[i]
		}
	}
	return nil
}
func (s *Server) validGatheringPool(pool *assets.GatheringPool) bool {
	if pool.IntervalSeconds == 0 || len(pool.Items) == 0 {
		return false
	}
	for _, id := range pool.Items {
		if _, known := s.Assets.Items[id]; !known {
			return false
		}
	}
	return true
}

// Called by the existing maintenance tick under worldMu. Grant one item per
// elapsed tick; a delayed scheduler cannot accumulate a burst of free rewards.
func (s *Server) tickGathering(ctx context.Context, now time.Time) {
	for _, c := range s.world {
		run := c.gathering
		if run == nil {
			continue
		}
		if !c.ready || !gmIdle(c) || c.stall != nil || c.character.HP == 0 || run.Map != c.character.Map || run.X != c.character.X || run.Y != c.character.Y {
			c.gathering = nil
			continue
		}
		if now.Before(run.NextAt) {
			continue
		}
		pool := s.gatheringPool(run.Kind)
		if pool == nil || !s.validGatheringPool(pool) {
			c.gathering = nil
			continue
		}
		next := c.character.Clone()
		id := pool.Items[rand.IntN(len(pool.Items))]
		adds, err := next.Bag.Grant(game.Item{ID: id}, 1, s.Assets.Items[id].StackLimit(), s.Assets.Items)
		if err != nil {
			c.gathering = nil
			s.sendOrClose(c, tradeMessage("Gathering stopped: "+err.Error()+"."))
			continue
		}
		if err = s.commit(ctx, c, next); err != nil {
			c.gathering = nil
			s.Log.Error("gathering save failed", "character", c.character.ID, "error", err)
			continue
		}
		run.NextAt = now.Add(time.Duration(pool.IntervalSeconds) * time.Second)
		s.sendOrClose(c, c.character.Bag.AdditionPacket(adds), tradeMessage(fmt.Sprintf("Gathered item #%d.", id)))
	}
}

// Gathering keeps its one-second cadence independently of checkpoint frequency.
func (s *Server) maintainGathering(ctx context.Context, now time.Time) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, autosaveTimeout)
	defer cancel()
	s.tickGathering(ctx, now)
	s.tickFishing(ctx, now)
	s.tickManufacturing(ctx, now)
}
