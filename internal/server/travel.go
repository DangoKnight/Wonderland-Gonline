package server

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

// sendVehiclePackets sends the native owner/map sequence, without changing state.
// Caller holds worldMu. Receipt failures cannot undo an already committed change.
func (s *Server) sendVehiclePackets(c *Session, packets [][]byte) error {
	for _, p := range packets {
		if c.ready {
			s.broadcastWorld(c, p)
		}
		if err := c.send(p); err != nil {
			return err
		}
	}
	return nil
}

// vehicleCommand ports AC15:7/9 boarding, 14 placement, 10 landing and 13 ACK.
// Placement only announces coordinates; confirmation owns the active state.
func (s *Server) vehicleCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	if p[1] == protocol.PetControlVehicleNoOp {
		if len(p) != vehicleAckShortBytes && len(p) != vehicleAckOwnerBytes {
			return protocol.ErrMalformed
		}
		return nil
	}
	if len(p) != 5 {
		return protocol.ErrMalformed
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	slot, id := p[2], uint16(p[3])|uint16(p[4])<<8
	switch p[1] {
	case protocol.PetControlPlaceVehicle:
		if _, ok := c.character.Vehicle(slot, id, s.Assets.Items); !ok {
			return nil
		}
		return c.send(protocol.Builder{protocol.CommandPetControl, protocol.PetControlVehiclePosition, slot}.U32(c.character.ID).U16(id).U32(uint32(c.character.X)).U32(uint32(c.character.Y)))
	case protocol.PetControlBoardVehicle, protocol.PetControlBoardVehicleAlternate:
		if _, ok := c.character.Vehicle(slot, id, s.Assets.Items); !ok {
			return nil
		}
		if c.character.ActiveVehicle == id && c.character.VehicleSlot == slot {
			return nil
		}
		next := c.character.Clone()
		var unride [][]byte
		if next.ActiveMount != 0 {
			unride = serverwire.UnmountPackets(next.ID)
		}
		next.ActiveMount = 0
		next.ActiveVehicle, next.VehicleSlot = id, slot
		if err := s.commit(ctx, c, next); err != nil {
			return err
		}
		return s.sendVehiclePackets(c, append(unride, serverwire.VehicleMountPacket(c.character)))
	case protocol.PetControlDismountVehicle:
		// The discriminator is not an inventory slot. The owned slot controls removal.
		if id == 0 || c.character.ActiveVehicle != id {
			return nil
		}
		if id == game.DisposableRaftItemID {
			return s.wreckVehicle(ctx, c)
		}
		return s.dismountVehicle(ctx, c)
	default:
		return ErrUnsupported
	}
}

func (s *Server) dismountVehicle(ctx context.Context, c *Session) error {
	if c.character.ActiveVehicle == 0 {
		return nil
	}
	next := c.character.Clone()
	next.ActiveVehicle, next.VehicleSlot = 0, 0
	// commit emits the old slot's dismount only after the save.
	return s.commit(ctx, c, next)
}

// wreckVehicle destroys one item in the mounted slot. Only rafts are disposable;
// a stale slot or another vehicle dismounts without consuming any replacement.
func (s *Server) wreckVehicle(ctx context.Context, c *Session) error {
	old := *c.character
	if old.ActiveVehicle == 0 {
		return nil
	}
	if old.VehicleSlot < 1 || old.VehicleSlot > game.BagSize {
		return s.dismountVehicle(ctx, c)
	}
	item := old.Bag[old.VehicleSlot-1]
	if item.Empty() || item.ID != old.ActiveVehicle || s.Assets.Items[item.ID].Type != game.VehicleType || !game.Raft(item.ID) {
		return s.dismountVehicle(ctx, c)
	}
	next := old.Clone()
	next.ActiveVehicle, next.VehicleSlot = 0, 0
	if old.ActiveVehicle == game.DisposableRaftItemID && old.Map == game.MapID11016 {
		if terrain, exists := s.Assets.Terrains[old.Map]; exists {
			x, y, found := terrain.NearestWalkable(int(old.X), int(old.Y))
			if !found {
				return nil
			} // Keep the raft if no safe landing exists.
			next.X, next.Y = x, y
		}
	}
	if err := next.Bag.Remove(old.VehicleSlot, 1); err != nil {
		return err
	}
	// Commit position and consumption together, then publish recovery placement
	// (if needed), deletion, break, dismount and the scene-load gate.
	if err := s.commitState(ctx, c, next); err != nil {
		return err
	}
	if next.X != old.X || next.Y != old.Y {
		if err := s.sendVehiclePackets(c, [][]byte{next.PositionPacket()}); err != nil {
			return err
		}
	}
	if err := c.send([]byte{protocol.CommandInventory, protocol.InventoryRemove, old.VehicleSlot, 1}); err != nil {
		return err
	}
	packets := [][]byte{protocol.Builder{protocol.CommandPetControl, protocol.PetControlRemoveVehicle}.U32(old.ID).U16(old.ActiveVehicle), serverwire.VehicleDismountPacket(&old)}
	if err := s.sendVehiclePackets(c, packets); err != nil {
		return err
	}
	return c.send([]byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh})
}

// wearVehicle follows Inventory.ApplyVehicleWear and AC06's authored beach wreck.
// The reference has no active fuel consumption path. Only moved rafts wear.
func (s *Server) wearVehicle(ctx context.Context, c *Session) error {
	old := *c.character
	if old.ActiveVehicle == 0 {
		return nil
	}
	item, ok := old.Vehicle(old.VehicleSlot, old.ActiveVehicle, s.Assets.Items)
	if !ok {
		return s.dismountVehicle(ctx, c)
	}
	if !game.Raft(item.ID) {
		return nil
	}
	if item.Damage >= game.VehicleWreckDamage-1 || (item.ID == game.DisposableRaftItemID && old.Map == game.MapID11016 && old.X >= starterBeachRaftBreakX && old.Y >= starterBeachRaftBreakY) {
		return s.wreckVehicle(ctx, c)
	}
	next := old.Clone()
	next.Bag[old.VehicleSlot-1].Damage++
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	return c.send(next.Bag.Packet(protocol.CommandInventory, protocol.InventoryItems))
}

const starterBeachRaftBreakX = 280
const starterBeachRaftBreakY = 950

// Native AC15:13 may carry the acknowledged owner ID (FUN_0044b380).
const vehicleAckShortBytes = 2
const vehicleAckOwnerBytes = 6

// Authored destinations from GmManager.TownDirectory. These are the legacy
// command aliases and coordinates, not a table inferred from scene names.
var townDestinations = map[string]world.Destination{
	"welling":   {Map: 10001, X: 800, Y: 750},
	"kelan":     {Map: 10011, X: 1000, Y: 1000},
	"holy":      {Map: 10016, X: 1200, Y: 950},
	"kyoto":     {Map: 10041, X: 1300, Y: 1100},
	"changan":   {Map: 10051, X: 1400, Y: 1200},
	"chang_an":  {Map: 10051, X: 1400, Y: 1200},
	"rome":      {Map: 10061, X: 1100, Y: 1000},
	"maya":      {Map: 10071, X: 900, Y: 900},
	"inca":      {Map: 10081, X: 800, Y: 850},
	"bangkok":   {Map: 10091, X: 1200, Y: 1100},
	"southpole": {Map: 10021, X: 1000, Y: 1000},
	"ghostisle": {Map: 10026, X: 800, Y: 800},
	"carnie":    carnie,
	"pirate":    {Map: game.MapID10036, X: 1038, Y: 2235},
	"kaohsiung": {Map: 10003, X: 1000, Y: 1000},
	"jail":      {Map: game.MapID10000, X: 600, Y: 600},
}

// Chat remains available during gameplay interactions. Travel must not let a
// command bypass their owner or move a character before map acknowledgment.
func commandTravelAvailable(c *Session) bool {
	return c.character != nil && c.ready && c.battle == nil && c.event == nil && !c.storm && c.beach == nil
}

// visitCarnie is shared by AC5:17 and the public chat command. Keep the original
// return point when already visiting; don't change it on a failed durable warp.
func (s *Server) visitCarnie(ctx context.Context, c *Session) error {
	if !commandTravelAvailable(c) {
		return nil
	}
	if _, ok := s.World.Map(carnie.Map); !ok {
		return nil
	}
	origin := world.Destination{Map: c.character.Map, X: c.character.X, Y: c.character.Y}
	err := s.commandTeleport(ctx, c, carnie)
	if origin.Map != carnie.Map && c.character.Map == carnie.Map {
		c.carnieReturn = &origin
	}
	return err
}

// publicDismount ports RideVehicle("") and AC02's shore relocation. It does
// not consume a raft, reset its wear, or rest a mounted companion pet.
func (s *Server) publicDismount(ctx context.Context, c *Session) error {
	if err := s.dismountVehicle(ctx, c); err != nil {
		return err
	}
	if c.character.Map == game.MapID10036 {
		return s.commandTeleport(ctx, c, townDestinations["pirate"])
	}
	return nil
}

func (s *Server) gmTown(ctx context.Context, c *Session, words []string) error {
	if len(words) < 2 {
		return nil
	}
	if dst, ok := townDestinations[strings.ToLower(words[1])]; ok {
		return s.commandTeleport(ctx, c, dst)
	}
	return nil
}

// SummonAllPlayers moves a stable snapshot: each teleport removes its target
// from the published world. Each character commits independently as in C#;
// one failed target must not prevent later targets from being processed.
func (s *Server) summonAll(ctx context.Context, gm *Session) error {
	dst := world.Destination{Map: gm.character.Map, X: gm.character.X, Y: gm.character.Y}
	targets := make([]*Session, 0, len(s.world))
	for _, c := range s.world {
		if c != gm {
			targets = append(targets, c)
		}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].info.ID < targets[j].info.ID })
	for _, c := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !commandTravelAvailable(c) {
			continue
		}
		if err := s.commandTeleport(ctx, c, dst); err != nil {
			s.Log.Error("GM summon failed", "gm", gm.character.ID, "character", c.character.ID, "error", err)
			// Reconnect after a failed transition for a fresh durable snapshot.
			c.conn.Close()
		}
	}
	return nil
}

func teleportPointPacket(point byte, l game.Location) []byte {
	return protocol.Builder{protocol.CommandTeleportScroll, protocol.TeleportScrollRecord, point}.U16(l.Map).U16(l.X).U16(l.Y)
}

func (s *Server) teleportScrollCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != 4 {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.TeleportScrollRecord && p[1] != protocol.TeleportScrollTravel {
		return ErrUnsupported
	}
	slot, point := p[2], p[3]
	if slot < 1 || slot > game.BagSize || point < 1 || point > store.MaxTeleportPoints {
		return protocol.ErrMalformed
	}
	if !gmIdle(c) || c.tentOwner != 0 {
		return nil
	}
	ref := store.CharacterRef{Account: c.account.ID, ID: c.character.ID}
	if p[1] == protocol.TeleportScrollRecord {
		location := game.Location{Map: c.character.Map, X: c.character.X, Y: c.character.Y}
		if err := s.Store.RecordTeleportPoint(ctx, ref, point, slot, location, s.Assets.Travel.Scrolls, *c.character); err != nil {
			if errors.Is(err, game.ErrInvalidItem) {
				return c.send(serverwire.HeadBanner("A suitable unlocked Teleport Scroll is required."))
			}
			return err
		}
		return c.send(teleportPointPacket(point, location))
	}
	points, err := s.Store.TeleportPoints(ctx, ref)
	if err != nil {
		return err
	}
	dst, exists := points[point]
	if !exists {
		return c.send(serverwire.HeadBanner("No location recorded for this point."))
	}
	return s.travelWithItem(ctx, c, slot, false, world.Destination{Map: dst.Map, X: dst.X, Y: dst.Y}, func(item game.Item) bool { return s.Assets.Travel.Scrolls[item.ID] >= point })
}

func (s *Server) bambooDragonfly(ctx context.Context, c *Session, destination, slot byte) error {
	dst, ok := s.Assets.Travel.Destinations[destination]
	if !ok || !gmIdle(c) || c.tentOwner != 0 || c.character.Map == dst.Map {
		return nil
	}
	return s.travelWithItem(ctx, c, slot, true, world.Destination{Map: dst.Map, X: dst.X, Y: dst.Y}, func(item game.Item) bool { return slices.Contains(s.Assets.Travel.BambooItems, item.ID) })
}

// travelWithItem makes the debit and new recovery destination durable together.
// No warp or success receipt is published before commit. Old-map departure uses
// the previous location even though inventory and the checkpoint already changed.
func (s *Server) travelWithItem(ctx context.Context, c *Session, slot byte, search bool, dst world.Destination, valid func(game.Item) bool) error {
	if !gmIdle(c) || c.tentOwner != 0 || c.character.Map == game.TentMapID || dst.Map == game.TentMapID {
		return nil
	}
	if _, exists := s.World.Map(dst.Map); !exists {
		return c.send(serverwire.HeadBanner("Travel destination is unavailable."))
	}
	if c.openTent != nil {
		if err := s.closePlayerTent(ctx, c); err != nil {
			return err
		}
	}
	old := game.Location{Map: c.character.Map, X: c.character.X, Y: c.character.Y}
	invalid := errors.New("travel item unavailable")
	consumeSlot := slot
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		usable := func(slot byte) bool {
			if slot < 1 || slot > game.BagSize {
				return false
			}
			item := next.Bag[slot-1]
			_, known := s.Assets.Items[item.ID]
			return known && !item.Empty() && !item.Locked && valid(item)
		}
		if !usable(consumeSlot) && search {
			for i := range next.Bag {
				if usable(byte(i + 1)) {
					consumeSlot = byte(i + 1)
					break
				}
			}
		}
		if next.Map != old.Map || !usable(consumeSlot) {
			return invalid
		}
		if err := next.Bag.Remove(consumeSlot, 1); err != nil {
			return err
		}
		next.Map, next.X, next.Y = dst.Map, dst.X, dst.Y
		next.TentReturn = nil
		return nil
	}, *c.character)
	if errors.Is(err, invalid) {
		return c.send(serverwire.HeadBanner("A suitable unlocked travel item is required."))
	}
	if err != nil {
		return err
	}
	s.adoptSavedCharacter(c, next)
	if err := s.teleportPrelude(c); err != nil {
		return err
	}
	if err := c.send([]byte{protocol.CommandInventory, protocol.InventoryRemove, consumeSlot, 1}); err != nil {
		return err
	}
	c.character.Map, c.character.X, c.character.Y = old.Map, old.X, old.Y
	return s.teleportAfterSave(c, dst, 0)
}
