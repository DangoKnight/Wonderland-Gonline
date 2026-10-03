package server

import (
	"context"
	"sort"
	"time"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

// peers must be called with worldMu held. Only acknowledged characters are visible.
func (s *Server) peers(c *Session) []*Session {
	var peers []*Session
	for _, peer := range s.world {
		if peer != c && peer.character.Map == c.character.Map {
			peers = append(peers, peer)
		}
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].info.ID < peers[j].info.ID })
	return peers
}

// peerPackets describes char to another player. Only a login arrival refreshes the sprite.
func peerPackets(char game.Character, arriving, login bool) ([][]byte, error) {
	appearance, err := char.AppearancePacket(true)
	if err != nil {
		return nil, err
	}
	packets := [][]byte{appearance, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateEquipmentSnapshot}.U32(char.ID).Bytes(char.WornEquipment())}
	if arriving {
		packets = append(packets, protocol.Builder{protocol.CommandPresence, protocol.PresenceOnline}.U32(char.ID).U8(255))
		if login {
			packets = append(packets, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(char.ID).U8(0))
		}
	} else {
		packets = append(packets, char.PositionPacket())
	}
	return packets, nil
}

// acknowledgeWorld publishes a character only after its map load has completed.
// Packet writes are ordered under worldMu so movement cannot overtake a spawn,
// and a disconnect cannot leave a stale appearance after a despawn.
func (s *Server) acknowledgeWorld(c *Session) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if c.ready {
		return nil
	}
	// Grants during login or a warp wait for map load, keeping the initial
	// snapshot contiguous. Refresh only when the durable balance has changed.
	balances, err := s.Store.MallBalances(context.Background(), c.account.ID)
	if err != nil {
		return err
	}
	if balances.Points != c.account.IM || balances.Bonus != c.account.IMBonus {
		if err := s.sendAll(c, mallBalancePackets(balances)); err != nil {
			return err
		}
		c.account.IM, c.account.IMBonus = balances.Points, balances.Bonus
	}
	if next := c.character.Clone(); next.NormalizeVehicle(s.Assets.Items) {
		if err := s.commit(context.Background(), c, next); err != nil {
			return err
		}
	}
	arrival, err := peerPackets(*c.character, true, !c.warped)
	if err != nil {
		return err
	}
	arrival = append(arrival, s.peerPetPackets(c.character)...)
	peers := s.peers(c)
	// Finish the entrant's snapshot before publishing it to existing players.
	for _, peer := range peers {
		packets, err := peerPackets(*peer.character, false, false)
		if err != nil {
			return err
		}
		// A peer's battle pet follows its appearance, before its position.
		position := packets[len(packets)-1]
		packets = append(append(packets[:len(packets)-1], s.peerPetPackets(peer.character)...), position)
		if peer.emote != 0 {
			// SendMapInfo replays a peer's held pose.
			packets = append(packets, protocol.Builder{protocol.CommandPose, protocol.PoseBroadcast}.U32(peer.character.ID).U8(peer.emote))
		}
		for _, packet := range packets {
			if err := c.send(packet); err != nil {
				return err
			}
		}
	}
	if err := c.send([]byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh}); err != nil {
		return err
	}
	if c.character.ActiveVehicle != 0 {
		if err := c.send(vehicleMountPacket(c.character)); err != nil {
			return err
		}
	}
	s.world[c.info.ID] = c
	c.ready = true
	s.Log.Info("map load acknowledged", "session", c.info.ID, "character", c.character.ID, "map", c.character.Map)
	for _, peer := range peers {
		for _, packet := range arrival {
			if err := peer.send(packet); err != nil {
				// A failed recipient must not disconnect the player causing the update.
				peer.conn.Close()
				break
			}
		}
	}
	s.partyArrival(c)
	if s.friendSessions == nil {
		s.friendSessions = map[uint32]*Session{}
	}
	s.friendSessions[c.character.ID] = c
	if !c.warped {
		c.friendOnline = true
		s.friendPresence(c, true)
	}
	// Deliver queued mail after the map snapshot, including mail received while
	// loading a warp. Socket failure leaves undelivered messages queued.
	if err := s.deliverPendingTextMail(context.Background(), c); err != nil {
		return err
	}
	if c.warped && s.Assets.LuckyDraw.TotalWeight > 0 {
		if err := c.send(s.luckyDrawCatalogPacket(c.character, time.Now())); err != nil {
			return err
		}
	}
	// AC12:1 also starts the authored arrival script.
	return s.arrivalScript(c)
}

// broadcastWorld sends to map peers, excluding the actor. Caller holds worldMu.
func (s *Server) broadcastWorld(c *Session, packet []byte) {
	for _, peer := range s.peers(c) {
		if err := peer.send(packet); err != nil {
			peer.conn.Close()
		}
	}
}

func (s *Server) leaveWorld(c *Session) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if c.autosaveBaseline != nil {
		ctx, cancel := context.WithTimeout(context.Background(), autosaveTimeout)
		if err := s.autosaveSession(ctx, c); err != nil {
			s.Log.Error("disconnect autosave failed", "session", c.info.ID, "error", err)
		}
		cancel()
		c.autosaveBaseline = nil
	}
	published := c.friendOnline || s.world[c.info.ID] == c
	c.friendOnline = false
	if c.character != nil && s.friendSessions[c.character.ID] == c {
		delete(s.friendSessions, c.character.ID)
	}
	s.clearFriendRequests(c)
	s.cancelTrade(c)
	s.abandonBattle(c)
	s.endEvent(c)
	// C# keeps a disconnected player in its team; it leaves here.
	s.partyLeave(c, false)
	if s.world[c.info.ID] == c {
		s.depart(c, protocol.Builder{protocol.CommandMapAcknowledgment}.U32(c.character.ID).U16(0).U16(0).U16(0).U16(0).U8(0))
	}
	if published {
		s.friendPresence(c, false)
	}
}

// depart removes a published character, telling its map peers. Caller holds worldMu.
func (s *Server) depart(c *Session, packet []byte) {
	if s.world[c.info.ID] != c {
		return
	}
	delete(s.world, c.info.ID)
	s.broadcastWorld(c, packet)
	c.ready = false
}

// arrivalPackets follows Map.Warp_In for the entrant: load command, position, worn
// equipment, sprite refresh, SendMapInfo with its visibility sync, then the quest
// journal. Peers are exchanged after AC12:1. It resets the view's scene state.
func (s *Server) arrivalPackets(char game.Character, view *world.View, pets *petRoster, portal byte) [][]byte {
	var packets [][]byte
	if char.Map == game.MapID60002 {
		// The territory emblem must resolve to no bitmap; no owner is persisted.
		packets = append(packets, protocol.Builder{protocol.CommandTerritory, protocol.TerritoryWireCode1, 23, 0}.U32(0))
	}
	packets = append(packets, char.WarpPacket(uint16(portal)), char.PositionPacket(), protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateEquipmentSnapshot}.U32(char.ID).Bytes(char.WornEquipment()), protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(char.ID).U8(0))
	packets = append(packets, s.World.MapInfo(&char, view, []uint32{char.ID})...)
	packets = append(packets, s.World.Sync(&char, view, false)...)
	packets = append(packets, s.rosterPackets(&char, pets)...)
	return append(packets, s.World.Journal(&char, view)...)
}

// usePortal handles AC20:8. Reference: AC20.Recv8 and GameMap.Teleport(Regular).
func (s *Server) usePortal(ctx context.Context, c *Session, p []byte) error {
	// The C# reader ignores bytes after the portal ID.
	if len(p) < 4 {
		return protocol.ErrMalformed
	}
	portal := uint16(p[2]) | uint16(p[3])<<8
	// A GM summon may move this character from another session; read it under worldMu.
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if c.battle != nil || c.event != nil {
		return nil
	}
	if !c.ready {
		return c.send([]byte{protocol.CommandEvent, protocol.EventResume})
	}
	// Walk-in regions take precedence over the step itself (AC20.Recv8).
	if ran, e := s.tryRegion(ctx, c, 0, 0, false); ran || e != nil {
		return e
	}
	return s.enterPortalStep(ctx, c, portal, true)
}

// enterPortal is a door object's Teleport(Regular), without an EVE entry script.
func (s *Server) enterPortal(ctx context.Context, c *Session, portal uint16) error {
	return s.enterPortalStep(ctx, c, portal, false)
}

// enterPortalStep is GameMap.Teleport(Regular) for a published character; a client step
// first tries the EVE entry script with that ID. Caller holds worldMu.
func (s *Server) enterPortalStep(ctx context.Context, c *Session, portal uint16, scripted bool) error {
	unfreeze := func() error { return c.send([]byte{protocol.CommandEvent, protocol.EventResume}) }
	char := c.character
	// Do not bounce a player back through the area it arrived in: the client
	// reports it on landing (its last area is cleared by the map load,
	// FUN_00304898) and does not report it again until it has left.
	if c.arrived {
		return unfreeze()
	}
	if scripted {
		if ran, e := s.tryEntry(ctx, c, portal); ran || e != nil {
			return e
		}
	}
	if e := s.teleportPrelude(c); e != nil {
		return e
	}
	dst, ok := s.World.Portal(char.Map, portal, char.X, char.Y)
	if char.Map == carnie.Map && c.carnieReturn != nil {
		dst, ok = *c.carnieReturn, true // Carnie's exit returns where the visit began.
	}
	if ok {
		_, ok = s.World.Map(dst.Map)
	}
	if !ok {
		s.Log.Debug("portal not found", "map", char.Map, "portal", portal, "x", char.X, "y", char.Y)
		return unfreeze()
	}
	from := char.Map
	landQuestRaft := scripted && from == 11016 && char.ActiveVehicle == game.DisposableRaftItemID
	if e := s.teleport(ctx, c, dst, byte(portal)); e != nil {
		return e
	}
	if landQuestRaft {
		if err := s.wreckVehicle(ctx, c); err != nil {
			return err
		}
	}
	return s.partyFollow(ctx, c, from)
}

// teleportPrelude is GameMap.Teleport's freeze and companion/vehicle reset for regular
// and command warps.
func (s *Server) teleportPrelude(c *Session) error {
	id := c.character.ID
	for _, packet := range [][]byte{{protocol.CommandEvent, protocol.EventClose}, protocol.Builder{protocol.CommandInventory, protocol.InventoryWireCode32}.U32(id), protocol.Builder{protocol.CommandInventory, protocol.InventoryWireCode112}.U32(id), protocol.Builder{protocol.CommandInventory, protocol.InventoryWireCode132}.U32(id)} {
		if e := c.send(packet); e != nil {
			return e
		}
	}
	return nil
}

// teleport follows Warp_Out then Warp_In for a resolved destination. The destination is
// persisted before any packet; the character is republished after its AC12:1. Caller
// holds worldMu.
func (s *Server) teleport(ctx context.Context, c *Session, dst world.Destination, portal byte) error {
	char := c.character
	if e := s.Store.UpdateCharacter(ctx, c.account.ID, char.ID, func(stored *game.Character) error {
		stored.Map, stored.X, stored.Y = dst.Map, dst.X, dst.Y
		return nil
	}); e != nil {
		return e
	}
	s.cancelTrade(c)
	// Old-map peers see the departure as a load command toward the destination.
	s.depart(c, protocol.Builder{protocol.CommandMapAcknowledgment}.U32(char.ID).U16(dst.Map).U16(dst.X).U16(dst.Y).U16(uint16(portal)).U8(0))
	char.Map, char.X, char.Y = dst.Map, dst.X, dst.Y
	s.mu.Lock()
	c.info.Map = dst.Map
	s.mu.Unlock()
	c.ready, c.warped = false, true
	c.arrived = true
	s.endEvent(c)
	c.resumeAt = time.Time{}
	c.encounter.enterMap()
	for _, packet := range s.arrivalPackets(*char, c.view, c.pets, portal) {
		if e := c.send(packet); e != nil {
			return e
		}
	}
	return nil
}
