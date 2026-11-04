package server

import (
	"context"
	"errors"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/world"
)

// samePlayerScene prevents starter-scene players from discovering each other.
// Shared tent interiors retain their existing owner-based visibility.
func (s *Server) samePlayerScene(a, b *Session) bool {
	return sameScene(a, b) && (a == b || a.tentOwner != 0 || !s.World.HideOtherPlayers(a.character.Map))
}

// peers must be called with worldMu held. Only acknowledged characters are visible.
func (s *Server) peers(c *Session) []*Session {
	var peers []*Session
	for _, peer := range s.world {
		if peer != c && s.samePlayerScene(peer, c) {
			peers = append(peers, peer)
		}
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].info.ID < peers[j].info.ID })
	return peers
}

// acknowledgeWorld publishes a character only after its map load has completed.
// Packet writes are ordered under worldMu so movement cannot overtake a spawn,
// and a disconnect cannot leave a stale appearance after a despawn.
func (s *Server) acknowledgeWorld(c *Session) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if err := c.outputError(); err != nil {
		return err
	}
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
		if err := s.sendAll(c, serverwire.MallBalancePackets(balances)); err != nil {
			return err
		}
		c.account.IM, c.account.IMBonus = balances.Points, balances.Bonus
	}
	if next := c.character.Clone(); next.NormalizeVehicle(s.Assets.Items) {
		if err := s.commit(context.Background(), c, next); err != nil {
			return err
		}
	}
	arrival, err := serverwire.PeerPackets(*c.character, true, !c.warped)
	if err != nil {
		return err
	}
	arrival = append(arrival, s.peerPetPackets(c.character)...)
	if c.shapeshift != 0 {
		arrival = append(arrival, shapeshiftPacket(c))
		if err := c.send(shapeshiftPacket(c)); err != nil {
			return err
		}
	}
	peers := s.peers(c)
	// Finish the entrant's snapshot before publishing it to existing players.
	for _, peer := range peers {
		if peer.invisible {
			continue
		}
		packets, err := serverwire.PeerPackets(*peer.character, false, false)
		if err != nil {
			return err
		}
		// A peer's battle pet follows its appearance, before its position.
		position := packets[len(packets)-1]
		packets = append(append(packets[:len(packets)-1], s.peerPetPackets(peer.character)...), position)
		if peer.fishing != nil {
			packets = append(packets, fishingStartedPacket(peer))
		}
		if peer.emote != 0 {
			// SendMapInfo replays a peer's held pose.
			packets = append(packets, protocol.Builder{protocol.CommandPose, protocol.PoseBroadcast}.U32(peer.character.ID).U8(peer.emote))
		}
		if peer.shapeshift != 0 {
			packets = append(packets, shapeshiftPacket(peer))
		}
		for _, packet := range packets {
			if err := c.send(packet); err != nil {
				return err
			}
		}
	}
	if c.tentOwner == 0 {
		if err := c.send([]byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh}); err != nil {
			return err
		}
		if err := s.restoreMapLoadUI(c); err != nil {
			return err
		}
	}
	if c.character.ActiveVehicle != 0 {
		if err := c.send(serverwire.VehicleMountPacket(c.character)); err != nil {
			return err
		}
	}
	if err := s.syncMapProps(context.Background(), c, time.Now()); err != nil {
		return err
	}
	if err := s.syncBazaars(context.Background(), c); err != nil {
		return err
	}
	s.world[c.info.ID] = c
	c.ready = true
	s.Log.Info("map load acknowledged", "session", c.info.ID, "character", c.character.ID, "map", c.character.Map)
	for _, peer := range peers {
		if c.invisible {
			break
		}
		for _, packet := range arrival {
			if err := peer.send(packet); err != nil {
				// A failed recipient must not disconnect the player causing the update.
				peer.conn.Close()
				break
			}
		}
	}
	if c.tentOwner != 0 {
		if err := s.tentSnapshot(context.Background(), c); err != nil {
			return err
		}
		if err := c.send([]byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh}); err != nil {
			return err
		}
		if err := s.restoreMapLoadUI(c); err != nil {
			return err
		}
		for _, peer := range peers {
			s.sendOrClose(peer, protocol.Builder{protocol.CommandInventory, protocol.InventoryTentPlayer}.U32(c.character.ID))
		}
	}
	if err := s.syncGuild(context.Background(), c); err != nil {
		return err
	}
	for _, peer := range peers {
		guild, err := s.Store.GuildForCharacter(context.Background(), peer.character.ID)
		if err != nil {
			return err
		}
		if guild != nil && !peer.invisible {
			if err := c.send(serverwire.GuildBadge(peer.character.ID, guild)); err != nil {
				return err
			}
		}

	}
	if err := s.guildPresence(context.Background(), c.character.ID); err != nil {
		return err
	}
	if c.tentOwner == 0 {
		for _, peer := range s.world {
			if peer.openTent != nil && peer.openTent.Map == c.character.Map && s.samePlayerScene(peer, c) {
				if err := c.send(tentSign(peer)); err != nil {
					return err
				}
			}
		}
	}
	if err := s.sendAll(c, s.stallList(c)); err != nil {
		return err
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
	if err := s.deliverAdminMail(context.Background(), c); err != nil {
		return err
	}
	// AC12:1 also starts the authored arrival script.
	return s.arrivalScript(c)
}

// Clear the pending release once after the scene-load gate is acknowledged.
// An interaction already running on the new map owns its own movement/UI lock.
func (s *Server) restoreMapLoadUI(c *Session) error {
	if !c.restoreUIAfterMapLoad {
		return nil
	}
	c.restoreUIAfterMapLoad = false
	if c.event != nil {
		return nil
	}
	return s.sendAll(c, [][]byte{{protocol.CommandMovement, protocol.MovementMovementLock, 0}, {protocol.CommandEvent, protocol.EventResume}})
}

// broadcastWorld sends to map peers, excluding the actor. Caller holds worldMu.
func (s *Server) broadcastWorld(c *Session, packet []byte) {
	if c.invisible {
		return
	}
	for _, peer := range s.peers(c) {
		if err := peer.send(packet); err != nil {
			peer.conn.Close()
		}
	}
}

func (s *Server) leaveWorld(c *Session) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if len(c.pendingMail) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), mailReceiptTimeout)
		s.finishMailReceipts(ctx, c)
		cancel()
	}
	delete(s.deliveries, c)
	if c.openTent != nil {
		if err := s.closePlayerTent(context.Background(), c); err != nil {
			s.Log.Error("tent close on logout failed", "error", err)
		}
	}
	published := c.friendOnline || s.world[c.info.ID] == c
	c.friendOnline = false
	if c.character != nil && s.friendSessions[c.character.ID] == c {
		delete(s.friendSessions, c.character.ID)
	}
	c.marriageProposal = nil
	for _, peer := range s.world {
		if peer.marriageProposal != nil && peer.marriageProposal.From == c {
			peer.marriageProposal = nil
		}
	}
	c.guildInvitation = nil
	for _, peer := range s.world {
		if peer.guildInvitation != nil && peer.guildInvitation.From == c {
			peer.guildInvitation = nil
		}
	}
	s.clearFriendRequests(c)
	c.gathering = nil
	s.stopFishing(c)
	s.closeStall(c)
	s.endTrade(c, 0, false, protocol.TradeResultDisconnected)
	s.abandonBattle(c)
	// Battle vitals must be staged before the final session checkpoint. A failed
	// SQL write retains its snapshot/baseline, so this pass can retry it.
	if c.autosaveBaseline != nil || c.battleDisconnect != nil {
		ctx, cancel := context.WithTimeout(context.Background(), autosaveTimeout)
		if err := s.autosaveSession(ctx, c); err != nil {
			s.Log.Error("disconnect autosave failed", "session", c.info.ID, "error", err)
		} else {
			c.autosaveBaseline = nil
		}
		cancel()
	}
	s.endEvent(c)
	// C# keeps a disconnected player in its team; it leaves here.
	s.partyLeave(c, false)
	s.instanceLeave(c)
	if s.world[c.info.ID] == c {
		s.depart(c, protocol.Builder{protocol.CommandMapAcknowledgment}.U32(c.character.ID).U16(0).U16(0).U16(0).U16(0).U8(0))
	}
	if c.character != nil && c.character.TentReturn != nil {
		next := c.character.Clone()
		recoverTentCharacter(&next)
		if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(stored *game.Character) error { *stored = next; return nil }); err != nil {
			s.Log.Error("tent return save failed", "error", err)
		} else {
			*c.character = next
		}
	}
	c.tentOwner = 0
	if published {
		if err := s.guildPresence(context.Background(), c.character.ID); err != nil {
			s.Log.Error("guild logout presence failed", "character", c.character.ID, "error", err)
		}
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
	for r := range s.World.StoryRules(assets.StoryArrivalPackets, &char, view, world.StoryContext{Map: char.Map}) {
		for _, cue := range r.Cues {
			if p, err := serverwire.StoryCuePacket(char, cue); err == nil {
				packets = append(packets, p)
			}
		}
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
	if err := c.outputError(); err != nil {
		return err
	}
	if c.tentOwner != 0 {
		if !gmIdle(c) {
			return nil
		}
		if portal == 1 {
			return s.exitTent(ctx, c)
		}
		return nil
	}
	if c.battle != nil || c.event != nil {
		return nil
	}
	if !c.ready {
		return c.send([]byte{protocol.CommandEvent, protocol.EventResume})
	}
	if s.starterBeachQuestRaft(c) {
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
	if s.starterBeachQuestRaft(c) {
		return unfreeze()
	}
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
	dst, ok := s.portalDestination(c, portal)
	s.Log.Debug("portal", "map", char.Map, "portal", portal, "x", char.X, "y", char.Y, "to", dst.Map, "tx", dst.X, "ty", dst.Y, "found", ok)
	if ok {
		_, ok = s.World.Map(dst.Map)
	}
	if !ok {
		s.Log.Debug("portal not found", "map", char.Map, "portal", portal, "x", char.X, "y", char.Y)
		return unfreeze()
	}
	from := char.Map
	if e := s.teleport(ctx, c, dst, byte(portal)); e != nil {
		return e
	}
	return s.partyFollow(ctx, c, from)
}

// portalDestination applies session return points to both regular and scripted
// Carnie exits. Its authored warp points at Underground Maze, not the visit origin.
func (s *Server) portalDestination(c *Session, portal uint16) (world.Destination, bool) {
	char := c.character
	if char.Map == carnie.Map && c.carnieReturn != nil && c.carnieReturn.Map != 0 {
		return *c.carnieReturn, true
	}
	return s.World.Portal(char.Map, portal, char.X, char.Y)
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
	if c.openTent != nil && (c.tentOwner == 0 || dst.Map != c.openTent.Map) {
		if err := s.closePlayerTent(ctx, c); err != nil {
			return err
		}
	}
	var saved game.Character
	var reward *assets.MapTransition
	var adds []game.Addition
	if e := s.Store.UpdateCharacter(ctx, c.account.ID, char.ID, func(stored *game.Character) error {
		var err error
		reward, adds, err = s.mapTransitionReward(stored, dst.Map)
		if err != nil {
			return err
		}
		stored.Map, stored.X, stored.Y = dst.Map, dst.X, dst.Y
		stored.TentReturn = nil
		saved = stored.Clone()
		return nil
	}); e != nil {
		if errors.Is(e, game.ErrInventoryFull) {
			return s.chatFeedback(c, "Make room for the exit reward before leaving.")
		}
		return e
	}
	if reward != nil {
		char.Bag = saved.Bag
		char.Quests = saved.Quests
	}
	// The narrow SQL warp committed a new location; retain its full durable
	// checkpoint without treating other pending session fields as already saved.
	c.autosaveBaseline = &saved
	if err := s.teleportAfterSave(c, dst, portal); err != nil {
		return err
	}
	if reward != nil {
		s.sendOrClose(c, char.Bag.AdditionPacket(adds))
		if reward.Talk > 0 {
			s.sendOrClose(c, serverwire.SpeechFrame(3, 0, reward.Talk, 1))
		}
	}
	return nil
}

// teleportAfterSave publishes an already durable destination. Caller holds worldMu.
func (s *Server) teleportAfterSave(c *Session, dst world.Destination, portal byte) error {
	char := c.character
	c.gathering = nil
	s.stopFishing(c)
	s.closeStall(c)
	s.cancelTrade(c)
	// Old-map peers see the departure as a load command toward the destination.
	s.depart(c, protocol.Builder{protocol.CommandMapAcknowledgment}.U32(char.ID).U16(dst.Map).U16(dst.X).U16(dst.Y).U16(uint16(portal)).U8(0))
	c.tentOwner = 0
	char.TentReturn = nil
	char.Map, char.X, char.Y = dst.Map, dst.X, dst.Y
	c.bazaarQuotes = nil
	s.mu.Lock()
	c.info.Map = dst.Map
	s.mu.Unlock()
	c.restMap = 0
	c.ready, c.warped = false, true
	c.arrived = true
	c.restoreUIAfterMapLoad = c.restoreUIAfterMapLoad || c.event != nil
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

const maxMonsterBookIndex = 5500

// Constellation membership belongs to SQL definitions; native receipt marks
// remain active after completion, unlike a removed/completed quest mark.
func (s *Server) storyConstellations(c *game.Character) []byte {
	var owned [assets.MaxStoryConstellationID + 1]bool
	for reward := range s.World.StoryRules(assets.StoryConstellation, nil, nil, world.StoryContext{}) {
		q := c.Quests[reward.Mark]
		if reward.Value > 0 && reward.Value <= assets.MaxStoryConstellationID && q.State == game.InProgress && q.Step > 0 {
			owned[reward.Value] = true
		}
	}
	out := protocol.Builder{protocol.CommandStoryConstellation, protocol.StoryConstellationList, 0}
	for id, present := range owned {
		if present {
			out = out.U8(byte(id))
		}
	}
	out[2] = byte(len(out) - 3)
	return out
}

func (s *Server) bookMonster(id uint16) bool {
	npc, ok := s.Assets.NPCs[id]
	return ok && npc.BookIndex > 0 && npc.BookIndex <= maxMonsterBookIndex
}

func (s *Server) monsterBookPackets(c *game.Character) [][]byte {
	ids := append([]uint16(nil), c.DiscoveredMonsters...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var out [][]byte
	for i, id := range ids {
		if s.bookMonster(id) && (i == 0 || ids[i-1] != id) {
			out = append(out, protocol.Builder{protocol.CommandMonsterBook, protocol.MonsterBookDiscover}.U32(uint32(id)))
		}
	}
	return out
}

// Stage discoveries in the same character transaction as victory rewards.
// Return receipts separately so a failed save never publishes a discovery.
func (s *Server) discoverBattleMonsters(c *game.Character, run *battleRun) [][]byte {
	seen := make(map[uint16]bool, len(c.DiscoveredMonsters))
	for _, id := range c.DiscoveredMonsters {
		seen[id] = true
	}
	var out [][]byte
	for _, monster := range run.b.Defenders {
		if monster.Template == 0 || monster.Template > math.MaxUint16 {
			continue
		}
		id := uint16(monster.Template)
		if seen[id] || !s.bookMonster(id) {
			continue
		}
		seen[id] = true
		c.DiscoveredMonsters = append(c.DiscoveredMonsters, id)
		out = append(out, protocol.Builder{protocol.CommandMonsterBook, protocol.MonsterBookDiscover}.U32(uint32(id)))
	}
	return out
}

// AC89:0 requests the scene status. AC92:1 acknowledges it. Both can arrive
// before AC12:1 and must not publish world presence or mark the map ready.
func (s *Server) sceneReadyCommand(c *Session, p []byte) error {
	if len(p) < protocol.SceneReadyShortRequestBytes {
		return protocol.ErrMalformed
	}
	// FUN_002c2394 at 002dcab0..002dcb3a appends a four-byte native
	// encoded value to AC89:0. Private Server ignores it; it is synchronization
	// metadata, never an authenticated character selector. Retain the short
	// request supported by Go clients and require the exact native layout.
	switch p[0] {
	case protocol.CommandSceneReady:
		if len(p) != protocol.SceneReadyShortRequestBytes && len(p) != protocol.SceneReadyNativeRequestBytes {
			return protocol.ErrMalformed
		}
	case protocol.CommandSceneReadyAck:
		// FUN_002c2394 at 002dcd61..002dcda5 appends a boolean via
		// FUN_00113534. It is client metadata, not a map-ready flag.
		if len(p) != protocol.SceneReadyAckRequestBytes && len(p) != protocol.SceneReadyNativeAckRequestBytes {
			return protocol.ErrMalformed
		}
		if len(p) == protocol.SceneReadyNativeAckRequestBytes && p[2] > 1 {
			return protocol.ErrMalformed
		}
	default:
		return ErrUnsupported
	}
	if (p[0] == protocol.CommandSceneReady && p[1] != protocol.SceneReadyLoaded) ||
		(p[0] == protocol.CommandSceneReadyAck && p[1] != protocol.SceneReadyAcknowledged) {
		return ErrUnsupported
	}
	if p[0] == protocol.CommandSceneReady {
		// Native itemmall capture, packet 2888, as documented in AC89.cs.
		if err := c.send([]byte{protocol.CommandSceneReadyReply, protocol.SceneReadyStatus, 0, 1, 1, 3, 2, 3}); err != nil {
			return err
		}
	}
	if c.motdSent {
		return nil
	}
	c.motdSent = true
	if strings.TrimSpace(s.motd) == "" {
		return nil
	}
	out, err := protocol.Builder{protocol.CommandInventory, protocol.InventoryMessage, 0}.String(s.motd)
	if err != nil {
		return err
	}
	return c.send(out)
}

// movementCommand runs with worldMu held by worldCommand.
func (s *Server) movementCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	stopped := p[1] == protocol.MovementStop
	// Native stop reports have the same coordinate prefix as AC6:1, followed
	// by eight movement-check bytes. Outbound AC6:2 is a different lock packet.
	if stopped {
		if len(p) != protocol.MovementNativeRequestBytes {
			return protocol.ErrMalformed
		}
	} else if len(p) < protocol.MovementRequestMinBytes || p[1] != protocol.MovementMove {
		return protocol.ErrMalformed
	}

	r := protocol.NewReader(p[2:])
	direction, x, y := r.U8(), r.U16(), r.U16()
	validDirection := direction <= protocol.MovementDirectionMax
	if stopped {
		// Stops copy current pose/facing, including values beyond walking's 0..7.
		validDirection = direction <= protocol.MovementStopPoseMax || direction == protocol.MovementStopWirePose99
	}
	if !validDirection {
		return protocol.ErrMalformed
	}
	// AC06 ignores movement while an event holds the player.
	if c.event != nil {
		return nil
	}
	prevX, prevY := c.character.X, c.character.Y
	valid := s.World.CanMove(c.character.Map, prevX, prevY, x, y)
	nativeDestination := len(p) == protocol.MovementNativeRequestBytes
	if nativeDestination {
		// Native AC6 announces a destination before walking and resolves its
		// route locally. The previously announced destination is not the current
		// rendered position, so it cannot define a straight collision leg.
		valid = s.World.CanMove(c.character.Map, x, y, x, y)
	}
	// Land collision bits cannot reject legitimate water/air travel. Preserve
	// native vehicle movement within scene bounds for a validated active item;
	// unowned or stale riding flags never grant the exception.
	if _, mounted := c.character.Vehicle(c.character.VehicleSlot, c.character.ActiveVehicle, s.Assets.Items); mounted {
		valid = s.World.Contains(c.character.Map, x, y)
	}
	if !valid {
		s.Log.Debug("movement destination rejected", "session", c.info.ID, "map", c.character.Map, "x", x, "y", y, "previous_x", prevX, "previous_y", prevY, "native_destination", nativeDestination, "stopped", stopped)
		if nativeDestination {
			// AC7 resets the local avatar immediately. Sending the old destination
			// here teleports a still-walking client; leave invalid clicks to its UI.
			return nil
		}
		return c.send(c.character.PositionPacket())
	}
	// Keep recoverable walking in memory until a guarded checkpoint.
	if c.autosaveBaseline == nil {
		baseline := c.character.Clone()
		c.autosaveBaseline = &baseline
	}
	if x != prevX || y != prevY {
		// Moving away invalidates the NPC shop catalog and sale authority.
		c.saleMode, c.saleMap = -1, 0
		if c.view != nil {
			c.view.ShopItems = nil
		}
		// Walking ends a held pose locally without an AC32 report.
		c.emote = 0
		s.stopFishing(c)
		s.closeStall(c)
		s.cancelTrade(c)
		c.arrived = false
	}
	s.Log.Debug("movement destination accepted", "session", c.info.ID, "map", c.character.Map, "x", x, "y", y, "previous_x", prevX, "previous_y", prevY, "native_destination", nativeDestination, "stopped", stopped)
	c.character.X, c.character.Y = x, y
	// Resolve quest-raft landing before any land region can start an event.
	// Stop reports may supply the final shore coordinate too.
	if s.starterBeachQuestRaft(c) {
		if err := s.wearVehicle(ctx, c); err != nil {
			return err
		}
	}
	packet := protocol.Builder{protocol.CommandMovement, protocol.MovementMove}.U32(c.character.ID).U8(direction).U16(x).U16(y)
	s.broadcastWorld(c, packet)
	if stopped {
		// Peers understand AC6:1 with the stopped endpoint. The sender has
		// already stopped locally: echoing AC6:1 could restart its movement.
		// A stop correction is not another walking step; do not charge vehicle
		// wear, roll encounters, or activate destination regions a second time.
		return nil
	}
	if e := c.send(packet); e != nil {
		return e
	}
	if c.tentOwner != 0 {
		return nil
	}
	if ran, e := s.tryRegion(ctx, c, prevX, prevY, true); ran || e != nil {
		return e
	}
	if x != prevX || y != prevY {
		if err := s.wearVehicle(ctx, c); err != nil {
			return err
		}
	}
	return s.stepEncounter(c, prevX, prevY)
}

func (s *Server) mapTransitionReward(c *game.Character, to uint16) (*assets.MapTransition, []game.Addition, error) {
	for _, rule := range s.Assets.MapTransitions {
		if c.Map != rule.FromMap || to != rule.Destination.Map || rule.Quest == 0 || len(rule.Items) == 0 || c.Quests[rule.Quest].State == game.Completed {
			continue
		}
		for _, item := range c.Bag {
			if !item.Empty() && slices.Contains(rule.SkipIfOwned, item.ID) {
				return nil, nil, nil
			}
		}
		for _, grant := range rule.Items {
			if _, known := s.Assets.Items[grant.ItemID]; !known {
				return nil, nil, nil
			}
		}
		var adds []game.Addition
		for _, grant := range rule.Items {
			definition, known := s.Assets.Items[grant.ItemID]
			if !known {
				return nil, nil, nil
			}
			added, err := c.Bag.Grant(game.Item{ID: grant.ItemID}, int(grant.Quantity), definition.StackLimit(), s.Assets.Items)
			if err != nil {
				return nil, nil, err
			}
			adds = append(adds, added...)
		}
		now := time.Now().UTC()
		if c.Quests == nil {
			c.Quests = map[uint32]game.Quest{}
		}
		c.Quests[rule.Quest] = game.Quest{ID: rule.Quest, State: game.Completed, Step: 1, StartedAt: now, CompletedAt: &now}
		return &rule, adds, nil
	}
	return nil, nil, nil
}
