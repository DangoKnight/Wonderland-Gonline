package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
)

const (
	manufactureChanceScale      = 100
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
		s.clearShapeshift(c)
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
		if len(args) < 2 || len(args) > game.AlchemyMaximumIngredients {
			return true, s.chatFeedback(c, "Usage: /compound <slot1> <slot2> [slot3] [slot4] [slot5]")
		}
		slots := make([]byte, len(args))
		for i, arg := range args {
			v, err := strconv.ParseUint(arg, 10, 8)
			if err != nil || v < 1 || v > game.BagSize {
				return true, s.chatFeedback(c, "Choose valid bag slots.")
			}
			slots[i] = byte(v)
		}
		return true, s.synthesize(ctx, c, slots...)

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
			s.sendOrClose(c, serverwire.TradeMessage("Gathering stopped: "+err.Error()+"."))
			continue
		}
		if err = s.commit(ctx, c, next); err != nil {
			c.gathering = nil
			s.Log.Error("gathering save failed", "character", c.character.ID, "error", err)
			continue
		}
		run.NextAt = now.Add(time.Duration(pool.IntervalSeconds) * time.Second)
		s.sendOrClose(c, c.character.Bag.AdditionPacket(adds), serverwire.TradeMessage(fmt.Sprintf("Gathered item #%d.", id)))
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

// tentManufactureBench reads the bench a continue/stop request addresses. The
// native form repeats it as u16 after padding; the legacy form uses one byte or
// omits it and reuses the active job.
func tentManufactureBench(p []byte, current byte) byte {
	switch {
	case len(p) >= protocol.TentManufactureNativeControlBytes:
		return byte(protocol.NewReader(p[2:]).U16())
	case len(p) == protocol.TentManufactureControlBytes:
		return p[2]
	}
	return current
}

// parseTentManufactureStart reads the legacy five-byte start plus the native
// forms. The native client pads the request with an 18-byte placement tail that
// this server does not need: bench and formula keep their offsets.
func parseTentManufactureStart(p []byte) (bench uint16, id uint16, batch byte, native bool, err error) {
	batch = 1
	if len(p) < protocol.TentManufactureStartBytes {
		return 0, 0, 0, false, protocol.ErrMalformed
	}
	if len(p) == protocol.TentManufactureStartBytes {
		bench = uint16(p[2])
		id = protocol.NewReader(p[3:]).U16()
		return bench, id, batch, false, nil
	}
	r := protocol.NewReader(p[2:])
	bench, id = r.U16(), r.U16()
	if r.Err() != nil {
		return 0, 0, 0, false, protocol.ErrMalformed
	}
	if len(p) > protocol.TentManufactureNativeStartBytes {
		if v := r.U8(); v > 0 {
			batch = v
		}
	}
	return bench, id, batch, true, nil
}

func (s *Server) tentManufactureCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	ref := store.CharacterRef{Account: c.account.ID, ID: c.character.ID}
	now := time.Now()
	switch p[1] {
	case protocol.TentManufactureStart:
		// The native request also carries an 18-byte tail (placement/bench
		// metadata); bench and formula keep their payload offsets.
		if len(p) < protocol.TentManufactureStartBytes {
			return protocol.ErrMalformed
		}
		if !gmIdle(c) || c.tentOwner != c.character.ID {
			s.Log.Info("tent manufacture rejected", "character", c.character.ID, "idle", gmIdle(c), "tent_owner", c.tentOwner)
			return s.chatFeedback(c, "Manufacture inside your own tent.")
		}
		bench, id, batch, native, parseErr := parseTentManufactureStart(p)
		if parseErr != nil {
			return protocol.ErrMalformed
		}
		formula, known := s.Assets.Manufacturing[id]
		if !known {
			return s.chatFeedback(c, "This manufacturing formula is unavailable.")
		}
		var next game.Character
		var job *store.ManufactureJob
		var removes []game.Addition
		var err error
		if native {
			surface := s.Assets.TentSurfaces[formula.Output.ItemID]
			definition := s.Assets.Items[formula.Output.ItemID]
			w, h := definition.Footprint()
			formula.TentOutput = surface == 0 && (definition.Type == game.ItemTypeAppliance || definition.Type == game.ItemTypeTentDecoration) && (w > 1 || h > 1)
			next, job, removes, err = s.Store.StartNativeManufacturing(ctx, ref, bench, formula, batch, surface, now, s.Assets.Items, *c.character)
		} else {
			next, job, removes, err = s.Store.StartManufacturing(ctx, ref, byte(bench), formula, now, s.Assets.Items, *c.character)
		}
		if err != nil {
			s.Log.Info("tent manufacture start failed", "character", c.character.ID, "formula", id, "bench", bench, "error", err)
			return s.chatFeedback(c, "Manufacturing could not start: "+err.Error())
		}
		s.adoptSavedCharacter(c, next)
		c.manufacturing = job
		c.manufacturingLoaded = true
		c.gathering = nil
		s.stopFishing(c)
		for _, remove := range removes {
			s.sendOrClose(c, []byte{protocol.CommandInventory, protocol.InventoryRemove, remove.Slot, remove.Count})
		}
		// The native form opens its progress window on the continue/refresh pair,
		// not on the start packet alone.
		s.sendOrClose(c, []byte{protocol.CommandTentManufacture, protocol.TentManufactureContinue, job.Bench})
		s.sendOrClose(c, serverwire.ManufactureStartPacket(job, now))
		c.manufactureSync = true
		return s.completeManufacture(ctx, c, now)
	case protocol.TentManufactureContinue, protocol.TentManufactureStop:
		// The native control request repeats the bench as u16 and pads the
		// packet; the legacy form is a bare command or bench byte.
		if len(p) != 2 && len(p) != protocol.TentManufactureControlBytes && len(p) != protocol.TentManufactureNativeControlBytes {
			return protocol.ErrMalformed
		}
		if !gmIdle(c) {
			return nil
		}
		current, err := s.Store.ManufacturingJob(ctx, ref)
		if err != nil {
			return err
		}
		if current == nil {
			return nil
		}
		bench := tentManufactureBench(p, current.Bench)
		if current.Native && p[1] == protocol.TentManufactureStop {
			next, adds, err := s.Store.CancelNativeManufacturing(ctx, ref, bench, s.Assets.Items, *c.character)
			if err != nil {
				return s.chatFeedback(c, "Manufacturing could not be cancelled: "+err.Error())
			}
			s.adoptSavedCharacter(c, next)
			c.manufacturing = nil
			c.manufacturingLoaded = true
			if len(adds) > 0 {
				s.sendOrClose(c, c.character.Bag.AdditionPacket(adds))
			}
			return c.send([]byte{protocol.CommandTentManufacture, protocol.TentManufactureStop, bench})
		}
		job, err := s.Store.PauseManufacturing(ctx, ref, bench, p[1] == protocol.TentManufactureStop, now)
		if err != nil {
			return s.chatFeedback(c, err.Error())
		}
		c.manufacturing = job
		c.manufacturingLoaded = true
		sub := byte(protocol.TentManufactureContinue)
		if job.Paused {
			sub = protocol.TentManufactureStopped
		}
		if err = c.send([]byte{protocol.CommandTentManufacture, sub, job.Bench}); err != nil {
			return err
		}
		if !job.Paused {
			s.sendOrClose(c, serverwire.ManufactureStartPacket(job, now))
			return s.completeManufacture(ctx, c, now)
		}
		return nil
	}
	return ErrUnsupported
}
func (s *Server) completeManufacture(ctx context.Context, c *Session, now time.Time) error {
	job := c.manufacturing
	if job == nil || job.Paused || now.UnixMilli() < job.DueAt {
		return nil
	}
	next, completed, adds, err := s.Store.CompleteManufacturing(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, now, s.Assets.Items, *c.character)
	if errors.Is(err, store.ErrManufacturingNotDue) {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		c.manufacturing = nil
		return nil
	}
	if errors.Is(err, game.ErrInventoryFull) {
		paused, pauseErr := s.Store.PauseManufacturing(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, job.Bench, true, now)
		if pauseErr != nil {
			return pauseErr
		}
		c.manufacturing = paused
		s.sendOrClose(c, []byte{protocol.CommandTentManufacture, protocol.TentManufactureStopped, job.Bench})
		return s.chatFeedback(c, "Make room for the completed item, then continue manufacturing. Your output is retained.")
	}
	if err != nil {
		return err
	}
	s.adoptSavedCharacter(c, next)
	c.manufacturing = nil
	if completed.TentOutput || completed.Surface != 0 {
		if err = s.refreshTent(ctx, c.character.ID); err != nil {
			return err
		}
	}
	if len(adds) > 0 {
		s.sendOrClose(c, c.character.Bag.AdditionPacket(adds), []byte{protocol.CommandTentManufacture, protocol.TentManufactureBagComplete})
	}
	s.sendOrClose(c, []byte{protocol.CommandTentManufacture, protocol.TentManufactureContinue, completed.Bench}, []byte{protocol.CommandTentManufacture, protocol.TentManufactureStopped, completed.Bench})
	s.clearShapeshift(c)
	return nil
}
func (s *Server) tickManufacturing(ctx context.Context, now time.Time) {
	for _, c := range s.world {
		if !c.ready || !gmIdle(c) {
			continue
		}
		if c.tentOccupantsSync && time.Now().After(c.tentSyncAt) {
			c.tentOccupantsSync = false
			if c.tentOwner != 0 {
				s.syncTentOccupants(ctx, c)
			}
		}
		if c.tentSignSync {
			c.tentSignSync = false
			for _, peer := range s.world {
				if peer.openTent != nil && peer.openTent.Map == c.character.Map &&
					(peer.tentOwner != 0 || !s.World.HideOtherPlayers(c.character.Map)) {
					s.Log.Debug("tent sign resend", "to", c.character.ID, "owner", peer.character.ID)
					s.sendOrClose(c, tentSign(peer))
				}
			}
		}
		if c.manufactureSync {
			c.manufactureSync = false
			if job := c.manufacturing; job != nil {
				s.sendOrClose(c, serverwire.ManufactureStartPacket(job, now))
			}
		}
		if !c.manufacturingLoaded {
			job, err := s.Store.ManufacturingJob(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID})
			if err != nil {
				s.Log.Error("manufacturing recovery failed", "character", c.character.ID, "error", err)
				continue
			}
			c.manufacturing = job
			c.manufacturingLoaded = true
			if job != nil && c.tentOwner == c.character.ID {
				if job.Paused {
					s.sendOrClose(c, []byte{protocol.CommandTentManufacture, protocol.TentManufactureStopped, job.Bench})
				} else {
					s.sendOrClose(c, serverwire.ManufactureStartPacket(job, now))
				}
			}
		}
		if err := s.completeManufacture(ctx, c, now); err != nil {
			s.Log.Error("manufacturing completion failed", "character", c.character.ID, "error", err)
		}
	}
}

// ForgeGem in the reference consumes a gem but never changes equipment. WLRI
// also maps its hardcoded gem IDs to oil. A truthful failed reply preserves items
// until a real socket representation and verified consumables are available.
func (s *Server) gemSocketCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.GemSocketRequestBytes {
		return protocol.ErrMalformed
	}
	if err := c.send([]byte{protocol.CommandGemSocket, p[1], p[2], protocol.ManufactureFailed}); err != nil {
		return err
	}
	return s.chatFeedback(c, "Gem socketing is pending verified gem and equipment effects; no items were consumed.")
}
