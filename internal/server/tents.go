package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

const (
	tentFloorClickCooldown = 2 * time.Second
	tentFloorClickDistance = 48
	tentOccupantSyncDelay  = 1500 * time.Millisecond

	tentFurnitureType  byte = 28
	tentWorkbenchType  byte = 29
	tentDecorationType byte = 30
	tentRotationMax    byte = 3
)

type openTent struct {
	Map, X, Y uint16
	Slot      byte
}

func sameScene(a, b *Session) bool {
	if a == nil || b == nil || a.character == nil || b.character == nil {
		return false
	}
	if a.character.Map != b.character.Map || a.tentOwner != b.tentOwner {
		return false
	}
	// Levels inside one tent are separate scenes.
	return a.tentOwner == 0 || a.tentFloor == b.tentFloor
}
func tentSign(c *Session) []byte {
	t := c.openTent
	return protocol.Builder{protocol.CommandTent, protocol.TentEnter}.U32(c.character.ID).U16(tentItem).U32(uint32(t.X)).U32(uint32(t.Y)).U16(0)
}
func (s *Server) ensurePlayerTent(ctx context.Context, c *Session) (store.Tent, error) {
	if existing, err := s.Store.Tent(ctx, c.character.ID); err == nil {
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return store.Tent{}, err
	}
	rules := s.Assets.Tents
	var defaults []store.TentItem
	for _, row := range rules.Furniture {
		if _, known := s.Assets.Items[row.ItemID]; !known {
			return store.Tent{}, fmt.Errorf("default tent item %d is unavailable", row.ItemID)
		}
		defaults = append(defaults, store.TentItem{ItemID: row.ItemID, X: row.X, Y: row.Y, Floor: row.Floor, Rotation: row.Rotation})
	}
	return s.Store.EnsureTent(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, rules.Floor, rules.Wallpaper, defaults)
}
func (s *Server) openPlayerTent(ctx context.Context, c *Session, slot byte) error {
	if !gmIdle(c) || c.tentOwner != 0 || c.invisible {
		return c.send(serverwire.SystemLine("Finish active interactions before opening a tent."))
	}
	if c.openTent != nil {
		return nil
	}
	if slot < 1 || slot > game.BagSize || c.character.Bag[slot-1].ID != tentItem || c.character.Bag[slot-1].Empty() || c.character.Bag[slot-1].Locked {
		return nil
	}
	if _, err := s.ensurePlayerTent(ctx, c); err != nil {
		return c.send(serverwire.SystemLine(err.Error()))
	}
	c.openTent = &openTent{Map: c.character.Map, X: c.character.X, Y: c.character.Y, Slot: slot}
	c.character.Bag[slot-1].Locked = true
	c.gathering = nil
	s.closeStall(c)
	s.cancelTrade(c)
	sign := tentSign(c)
	s.broadcastWorld(c, sign)
	return s.sendAll(c, [][]byte{sign, {protocol.CommandTentFurniture, protocol.TentOpened, protocol.TentOpenedStatus}})
}

// syncTentOccupants exchanges appearances with everyone on the same tent level,
// because entering the interior does not run the overworld map snapshot.

// tentPeers lists everyone sharing the tent level, including sessions that are
// still loading their map and therefore absent from the world registry. The
// caller holds worldMu; s.mu guards the loading sessions and their character
// assignment in enterWorld.
func (s *Server) tentPeers(c *Session) []*Session {
	seen := map[*Session]bool{}
	var peers []*Session
	add := func(peer *Session) {
		if peer == nil || peer == c || seen[peer] || peer.character == nil || c.character == nil {
			return
		}
		if peer.character.Map != c.character.Map || peer.tentOwner != c.tentOwner {
			return
		}
		if peer.tentOwner != 0 && peer.tentFloor != c.tentFloor {
			return
		}
		seen[peer] = true
		peers = append(peers, peer)
	}
	for _, peer := range s.world {
		add(peer)
	}
	s.mu.Lock()
	for _, peer := range s.sessions {
		add(peer)
	}
	s.mu.Unlock()
	sort.Slice(peers, func(i, j int) bool { return peers[i].info.ID < peers[j].info.ID })
	return peers
}

func (s *Server) syncTentOccupants(ctx context.Context, c *Session) {
	peers := s.tentPeers(c)
	s.Log.Info("tent occupants sync", "character", c.character.ID, "tent", c.tentOwner, "floor", c.tentFloor, "peers", len(peers))
	for _, peer := range peers {
		if packets, err := serverwire.PeerPackets(*peer.character, false, false); err == nil {
			packets = append(packets, peer.character.PositionPacketFloor(c.tentFloor))
			if err := s.sendAll(c, packets); err != nil {
				return
			}
		}
		if packets, err := serverwire.PeerPackets(*c.character, false, false); err == nil {
			packets = append(packets, c.character.PositionPacketFloor(c.tentFloor))
			for _, packet := range packets {
				s.sendOrClose(peer, packet)
			}
		}
	}
}

func (s *Server) tentSnapshot(ctx context.Context, c *Session) error {
	tent, err := s.Store.Tent(ctx, c.tentOwner)
	if err != nil {
		return err
	}
	floor := c.tentFloor
	floorSurface, wallpaper := tent.Floor, tent.Wallpaper
	if floor == 1 {
		floorSurface, wallpaper = tent.Floor2, tent.Wallpaper2
	}
	s.Log.Info("tent snapshot", "character", c.character.ID, "floor", floor, "ready", c.ready, "occupants", len(s.tentPeers(c)))
	packets := [][]byte{
		{protocol.CommandTent, protocol.TentInteriorLayout, max(byte(1), tent.Type), floor},
		protocol.Builder{protocol.CommandTentFurniture, protocol.TentFloorSurface}.U16(floorSurface),
		protocol.Builder{protocol.CommandTentFurniture, protocol.TentWallpaperSurface}.U16(wallpaper),
	}
	if tent.Enlarged && floor == 0 {
		packets = append(packets, []byte{protocol.CommandTent, protocol.TentInteriorLayout, max(byte(1), tent.Type), 1})
	}
	if c.ready {
		tree := serverwire.NativeTentTreeFloor(tent, int(floor))
		if len(tree) > protocol.MaxPayload {
			return errors.New("tent furniture exceeds the native snapshot limit")
		}
		packets = append(packets, tree)
	}
	packets = append(packets, c.character.PositionPacketFloor(floor))
	packets = append(packets, []byte{protocol.CommandTent, protocol.TentInteriorStatus, tentLockedByte(tent.Locked), 0}, []byte{protocol.CommandInventory, protocol.InventoryTentPlayers})
	occupants := append(s.tentPeers(c), c)
	for _, peer := range occupants {
		if peer.invisible {
			continue
		}
		if peer.emote != 0 {
			packets = append(packets, protocol.Builder{protocol.CommandPose, protocol.PoseBroadcast}.U32(peer.character.ID).U8(peer.emote))
		}
		packets = append(packets, protocol.Builder{protocol.CommandInventory, protocol.InventoryTentPlayer}.U32(peer.character.ID), protocol.Builder{protocol.CommandPresence, protocol.PresenceOnline}.U32(peer.character.ID).U8(protocol.PresenceMapAvailable), protocol.Builder{protocol.CommandInventory, protocol.InventoryTentPlayerComplete}.U32(peer.character.ID))
	}
	packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryStallListComplete}, []byte{protocol.CommandEvent, protocol.EventPortal})
	return s.sendAll(c, packets)
}

func tentLockedByte(locked bool) byte {
	if locked {
		return 1
	}
	return 0
}

func (s *Server) refreshTent(ctx context.Context, owner uint32) error {
	for _, peer := range s.world {
		if peer.tentOwner == owner && peer.ready {
			if err := s.tentSnapshot(ctx, peer); err != nil {
				peer.conn.Close()
			}
		}
	}
	return nil
}
func (s *Server) enterTent(ctx context.Context, c *Session, owner uint32) error {
	if !gmIdle(c) || c.tentOwner != 0 {
		return nil
	}
	c.tentFloor = 0
	host := s.onlineByID(owner)
	if host == nil || host.openTent == nil || host.openTent.Map != c.character.Map || host.invisible ||
		(owner != c.character.ID && s.World.HideOtherPlayers(c.character.Map)) {
		return c.send(serverwire.SystemLine("This tent is unavailable."))
	}
	tent, err := s.Store.Tent(ctx, owner)
	if err != nil {
		return err
	}
	if tent.Locked && owner != c.character.ID {
		return c.send(serverwire.SystemLine("This tent is locked."))
	}
	// Entering someone else's tent leaves the visitor's own tent standing.
	if err = s.teleportPrelude(c); err != nil {
		return err
	}
	next := c.character.Clone()
	next.TentReturn = &game.Location{Map: next.Map, X: next.X, Y: next.Y}
	next.Map, next.X, next.Y = game.TentMapID, s.Assets.Tents.SpawnX, s.Assets.Tents.SpawnY
	if err = s.commit(ctx, c, next); err != nil {
		return err
	}
	// Depart while the old scene identity remains intact; adopt the private owner
	// only after the old scene's peers have received the warp.
	oldMap := next.TentReturn.Map
	c.character.Map = oldMap
	warp := protocol.Builder{protocol.CommandMapAcknowledgment}.U32(c.character.ID).U16(game.TentMapID).U16(next.X).U16(next.Y).U16(0).U8(0).U16(1).U8(1).U8(1)
	s.depart(c, warp)
	c.character.Map = game.TentMapID
	c.tentOwner = owner
	c.restMap = 0
	c.ready, c.warped, c.arrived = false, true, true
	c.gathering = nil
	s.cancelTrade(c)
	s.endEvent(c)
	c.view = world.NewView()
	s.mu.Lock()
	c.info.Map = game.TentMapID
	s.mu.Unlock()
	if err = c.send(warp); err != nil {
		return err
	}
	// The client finishes the interior on the map acknowledgment; the occupant
	// exchange waits for it because packets sent during the load are dropped.
	c.tentOccupantsSync = true
	c.tentSyncAt = time.Now().Add(tentOccupantSyncDelay)
	return s.tentSnapshot(ctx, c)
}
func (s *Server) exitTent(ctx context.Context, c *Session) error {
	if c.tentOwner == 0 {
		return nil
	}
	dst, ok := s.tentExitDestination(c)
	if !ok {
		return errors.New("tent has no valid overworld return point")
	}
	return s.teleport(ctx, c, world.Destination{Map: dst.Map, X: dst.X, Y: dst.Y}, 0)
}

// Prefer each visitor's durable return point. Recover missing/invalid map data
// from the host's open sign, never from the character's interior coordinates.
func (s *Server) tentExitDestination(c *Session) (game.Location, bool) {
	usable := func(dst game.Location) bool {
		_, known := s.World.Map(dst.Map)
		return known && !game.IsTentMap(dst.Map)
	}
	if dst := c.character.TentReturn; dst != nil && usable(*dst) {
		return *dst, true
	}
	if host := s.onlineByID(c.tentOwner); host != nil && host.openTent != nil {
		dst := game.Location{Map: host.openTent.Map, X: host.openTent.X, Y: host.openTent.Y}
		if usable(dst) {
			return dst, true
		}
	}
	return game.Location{}, false
}
func (s *Server) closePlayerTent(ctx context.Context, c *Session) error {
	if c.openTent == nil {
		return nil
	}
	seen := map[*Session]bool{}
	for _, peer := range s.world {
		seen[peer] = true
	}
	for _, peer := range s.friendSessions {
		seen[peer] = true
	}
	var failures []error
	for peer := range seen {
		if peer.tentOwner == c.character.ID {
			if err := s.exitTent(ctx, peer); err != nil {
				failures = append(failures, err)
				peer.conn.Close()
			}
		}
	}
	t := c.openTent
	if t.Slot >= 1 && t.Slot <= game.BagSize {
		c.character.Bag[t.Slot-1].Locked = false
	}
	c.openTent = nil
	packet := protocol.Builder{protocol.CommandTent, protocol.TentClosed}.U32(c.character.ID)
	for _, peer := range s.world {
		if peer.character.Map == t.Map && peer.tentOwner == 0 && (peer == c || !s.World.HideOtherPlayers(t.Map)) {
			s.sendOrClose(peer, packet)
		}
	}
	return errors.Join(failures...)
}
func (s *Server) tentCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < protocol.TentHeaderRequestBytes {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.TentEnter:
		if len(p) != protocol.TentEnterRequestBytes {
			return protocol.ErrMalformed
		}
		return s.enterTent(ctx, c, protocol.NewReader(p[2:]).U32())
	case protocol.TentClose:
		// The native close request carries one extra byte after the command.
		if len(p) < protocol.TentHeaderRequestBytes {
			return protocol.ErrMalformed
		}
		return s.closePlayerTent(ctx, c)
	case protocol.TentExit:
		if !gmIdle(c) {
			return nil
		}
		if len(p) < protocol.TentHeaderRequestBytes {
			return protocol.ErrMalformed
		}
		if c.tentFloor == 1 {
			return s.floorDown(ctx, c)
		}
		return s.exitTent(ctx, c)
	default:
		return ErrUnsupported
	}
}
func (s *Server) tentFurnitureCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) >= 4 && p[1] == protocol.TentFloorSurface {
		return s.tentFloorClick(ctx, c, protocol.NewReader(p[2:]).U16())
	}
	if len(p) < protocol.TentHeaderRequestBytes {
		return protocol.ErrMalformed
	}
	if c.tentOwner == 0 || c.tentOwner != c.character.ID {
		return c.send(serverwire.SystemLine("Only the owner can edit furniture inside their tent."))
	}
	if !gmIdle(c) {
		return nil
	}
	r := protocol.NewReader(p[2:])
	ref := store.CharacterRef{Account: c.account.ID, ID: c.character.ID}
	switch p[1] {
	case protocol.TentAttachTool:
		if len(p) != protocol.TentNativeToolBytes {
			return protocol.ErrMalformed
		}
		slot, host := r.U8(), r.U16()
		next, tent, err := s.Store.AttachTentTool(ctx, ref, slot, host, s.Assets.TentToolHosts, s.Assets.Items, *c.character)
		if err != nil {
			return c.send(serverwire.SystemLine("Attachment failed: " + err.Error()))
		}
		s.adoptSavedCharacter(c, next)
		hostIndex := host
		if hostIndex == 0 {
			hostIndex = 1
		}
		root := tent.Items[hostIndex-1]
		accessory := len(tent.Items)
		for i, row := range tent.Items {
			accessory += len(row.Tools)
			if i == int(hostIndex)-1 {
				break
			}
		}
		confirmation := protocol.Builder{protocol.CommandTentFurniture, protocol.TentToolAttached}.U16(uint16(accessory)).U16(root.Tools[len(root.Tools)-1].ItemID).U16(hostIndex)
		s.broadcastWorld(c, confirmation)
		if err := s.sendAll(c, [][]byte{{protocol.CommandInventory, protocol.InventoryRemove, slot, 1}, confirmation}); err != nil {
			return err
		}
	case protocol.TentPickupFurniture:
		if len(p) != protocol.TentNativePickupBytes {
			return protocol.ErrMalformed
		}
		raw := r.U16()
		index := raw
		if index > 0 {
			index--
		}
		next, adds, err := s.Store.PickUpTentItem(ctx, ref, index, s.Assets.Items, *c.character)
		if err != nil {
			return c.send(serverwire.SystemLine("Pickup failed: " + err.Error()))
		}
		s.adoptSavedCharacter(c, next)
		confirmation := protocol.Builder{protocol.CommandTentFurniture, protocol.TentPickupFurniture}.U16(raw)
		s.broadcastWorld(c, confirmation)
		if err := s.sendAll(c, [][]byte{next.Bag.AdditionPacket(adds), confirmation}); err != nil {
			return err
		}
		return nil
	case protocol.TentPlaceFurniture:
		legacy := len(p) == protocol.TentPlaceRequestBytes && p[2] == 0
		if !legacy && len(p) != protocol.TentNativePlaceBytes && len(p) != protocol.TentNativePlaceFloorBytes {
			return protocol.ErrMalformed
		}
		if legacy {
			_ = r.U8()
		}
		slot := r.U8()
		x, y, third := r.U32(), r.U32(), r.U32()
		floor := uint32(0)
		if legacy {
			floor = third
		} else if r.Remaining() > 0 {
			floor = uint32(r.U8())
		}
		// The native client does not report the level; the interior map carries it.
		if c.tentFloor == 1 {
			floor = 1
		}
		if slot < 1 || slot > game.BagSize || x > protocol.TentCoordinateMax || y > protocol.TentCoordinateMax || floor > 1 || r.Err() != nil {
			return protocol.ErrMalformed
		}
		item := c.character.Bag[slot-1]
		def, known := s.Assets.Items[item.ID]
		if kind, ok := s.Assets.TentSurfaces[item.ID]; ok && known && !item.Empty() && !item.Locked {
			wallpaper := kind == assets.TentSurfaceWallpaper
			if !wallpaper && kind != assets.TentSurfaceFloor {
				return nil
			}
			next, _, err := s.Store.ApplyTentSurface(ctx, ref, slot, item, wallpaper, byte(floor), c.character.Clone())
			if err != nil {
				return c.send(serverwire.SystemLine("Decoration failed: " + err.Error()))
			}
			s.adoptSavedCharacter(c, next)
			sub := byte(protocol.TentFloorSurface)
			if wallpaper {
				sub = protocol.TentWallpaperSurface
			}
			packet := protocol.Builder{protocol.CommandTentFurniture, sub}.U16(item.ID)
			s.broadcastWorld(c, packet)
			return s.sendAll(c, [][]byte{{protocol.CommandInventory, protocol.InventoryRemove, slot, 1}, packet})
		}
		if item.Empty() || item.Locked || !known || (def.Type != tentFurnitureType && def.Type != tentWorkbenchType && def.Type != tentDecorationType) {
			return c.send(serverwire.SystemLine("Select an available furniture or decoration item."))
		}
		var next game.Character
		var err error
		if legacy {
			next, err = s.Store.PlaceTentItem(ctx, ref, slot, uint16(x), uint16(y), item)
		} else {
			next, err = s.Store.PlaceNativeTentItem(ctx, ref, slot, uint16(x), uint16(y), byte(floor), item, s.Assets.Items, *c.character)
		}
		if err != nil {
			return c.send(serverwire.SystemLine("Placement failed: " + err.Error()))
		}
		if err = game.PreserveItemLocks(*c.character, &next); err != nil {
			return err
		}
		s.adoptSavedCharacter(c, next)
		s.sendOrClose(c, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, 1})
		if legacy {
			s.sendOrClose(c, []byte{protocol.CommandTentFurniture, protocol.TentPlaceFurniture, protocol.TentFurnitureSucceeded})
			return nil
		}
		tent, err := s.Store.Tent(ctx, c.character.ID)
		if err != nil {
			return err
		}
		row := tent.Items[len(tent.Items)-1]
		confirmation := protocol.Builder{protocol.CommandTentFurniture, protocol.TentPlaceFurniture}.Bytes(serverwire.TentTreeRecord(uint16(len(tent.Items)), row.ItemID, uint32(row.X), uint32(row.Y), 1, row.Rotation, 0, nil, row.Floor))
		s.broadcastWorld(c, confirmation)
		return c.send(confirmation)

	case protocol.TentMoveFurniture:
		if len(p) != protocol.TentMoveRequestBytes && len(p) != protocol.TentMoveRotatedRequestBytes {
			return protocol.ErrMalformed
		}
		index, x, y, _ := r.U16(), r.U32(), r.U32(), r.U32()
		rotation := byte(0)
		if r.Remaining() > 0 {
			rotation = r.U8()
		}
		if x > protocol.TentCoordinateMax || y > protocol.TentCoordinateMax || rotation > tentRotationMax {
			return protocol.ErrMalformed
		}
		if err := s.Store.MoveNativeTentItem(ctx, ref, index, uint16(x), uint16(y), rotation, s.Assets.Items); err != nil {
			return c.send(serverwire.SystemLine(err.Error()))
		}
		tent, err := s.Store.Tent(ctx, c.tentOwner)
		if err != nil {
			return err
		}
		if index == 0 || int(index) > len(tent.Items) {
			return nil
		}
		row := tent.Items[index-1]
		record := protocol.Builder{protocol.CommandTentFurniture, protocol.TentMoveFurniture}.
			U16(index).U32(uint32(row.X)).U32(uint32(row.Y)).U32(1).U8(row.Rotation)
		s.broadcastWorld(c, record)
		return c.send(record)
	default:
		return ErrUnsupported
	}
	return s.refreshTent(ctx, c.tentOwner)
}
func (s *Server) tentChatCommand(ctx context.Context, c *Session, name string, args []string) (bool, error) {
	switch name {
	case "tentlock", "tentunlock":
		if len(args) != 0 || !gmIdle(c) {
			return true, c.send(serverwire.SystemLine("Finish active interactions before changing the tent lock."))
		}
		if _, err := s.ensurePlayerTent(ctx, c); err != nil {
			return true, err
		}
		err := s.Store.SetTentLocked(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, name == "tentlock")
		if err != nil {
			return true, err
		}
		return true, c.send(serverwire.SystemLine("Tent access updated."))
	case "tentpickup":
		if len(args) != 1 || !gmIdle(c) || c.tentOwner != c.character.ID {
			return true, c.send(serverwire.SystemLine("Usage: /tentpickup <zero-based furniture index> inside your own tent."))
		}
		index, err := strconv.ParseUint(args[0], 10, 16)
		if err != nil {
			return true, c.send(serverwire.SystemLine("Invalid furniture index."))
		}
		next, adds, err := s.Store.PickUpTentItem(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, uint16(index), s.Assets.Items, c.character.Clone())
		if err != nil {
			return true, c.send(serverwire.SystemLine(err.Error()))
		}
		if err = game.PreserveItemLocks(*c.character, &next); err != nil {
			return true, err
		}
		s.adoptSavedCharacter(c, next)
		s.sendOrClose(c, next.Bag.AdditionPacket(adds))
		return true, s.refreshTent(ctx, c.tentOwner)
	case "tentexit":
		if len(args) != 0 || !gmIdle(c) {
			return true, c.send(serverwire.SystemLine("Usage: /tentexit"))
		}
		return true, s.exitTent(ctx, c)
	case "tentclose":
		if len(args) != 0 {
			return true, c.send(serverwire.SystemLine("Usage: /tentclose"))
		}
		return true, s.closePlayerTent(ctx, c)
	}
	return false, nil
}

// Reconnects recover the durable overworld return point. Open signs and instance
// ownership are process state, so recovery never recreates an unavailable home.
func recoverTentCharacter(c *game.Character) bool {
	if c.TentReturn == nil {
		return false
	}
	dst := *c.TentReturn
	c.Map, c.X, c.Y = dst.Map, dst.X, dst.Y
	c.TentReturn = nil
	return true
}

// sendFloorSwitch moves the client to another tent level: scene bracket, level
// header, position, label refresh, that floor's furniture and surface.
func (s *Server) sendFloorSwitch(ctx context.Context, c *Session, tent store.Tent, floor byte) error {
	oldPeers := s.peers(c)
	// Arrive at the entrance of the target level instead of keeping the spot
	// the player used on the other floor.
	next := c.character.Clone()
	next.X, next.Y = s.Assets.Tents.SpawnX, s.Assets.Tents.SpawnY
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	s.adoptSavedCharacter(c, next)
	c.tentFloor = floor
	if len(oldPeers) > 0 {
		// A different map id makes the client drop the actor instead of
		// teleporting the sprite to the new spot on the same map.
		warp := protocol.Builder{protocol.CommandMapAcknowledgment}.U32(c.character.ID).U16(game.TentUpperMapID).
			U16(c.character.X).U16(c.character.Y).U16(0).U8(0).U16(1).U8(1).U8(1)
		for _, peer := range oldPeers {
			s.sendOrClose(peer, warp)
			s.sendOrClose(peer, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(c.character.ID).U32(0))
		}
	}
	surface, wallpaper := tent.Floor, tent.Wallpaper
	if floor == 1 {
		surface, wallpaper = tent.Floor2, tent.Wallpaper2
	}
	packets := [][]byte{
		c.character.PositionPacketTent(game.TentMapID, floor),
		{protocol.CommandCharacterState, protocol.CharacterStateRefresh},
		{protocol.CommandInventory, protocol.InventorySceneBegin},
		{protocol.CommandTent, protocol.TentInteriorLayout, max(byte(1), tent.Type), floor},
		serverwire.NativeTentTreeFloor(tent, int(floor)),
		protocol.Builder{protocol.CommandTentFurniture, protocol.TentFloorSurface}.U16(surface),
		protocol.Builder{protocol.CommandTentFurniture, protocol.TentWallpaperSurface}.U16(wallpaper),
		{protocol.CommandInventory, protocol.InventorySceneComplete},
		{protocol.CommandEvent, protocol.EventResume},
	}
	if err := s.sendAll(c, packets); err != nil {
		return err
	}
	// The scene rebuild drops early packets, so exchange occupants after it.
	c.tentOccupantsSync = true
	c.tentSyncAt = time.Now().Add(tentOccupantSyncDelay)
	return nil
}

// tentFloorClick handles a furniture activation. Stairs, ladders and elevators
// switch the rendered floor: the native client keeps the level itself and
// rebuilds its object table from the layout packet, so the server answers with
// the target surface and that floor's furniture.
func (s *Server) tentFloorClick(ctx context.Context, c *Session, index uint16) error {
	if c.tentOwner == 0 || index == 0 {
		return nil
	}
	// The native client repeats the activation while the player stands on the
	// stairs. Accept one switch per press: the next one needs the player to
	// step away and come back, otherwise the repeats toggle the level forever.
	now := time.Now()
	if now.Sub(c.tentFloorClickAt) < tentFloorClickCooldown {
		return nil
	}
	dx := int(c.character.X) - int(c.tentFloorClickX)
	dy := int(c.character.Y) - int(c.tentFloorClickY)
	if c.tentFloorClickX != 0 && dx*dx+dy*dy < tentFloorClickDistance*tentFloorClickDistance {
		return nil
	}
	tent, err := s.Store.Tent(ctx, c.tentOwner)
	if err != nil {
		return err
	}
	if int(index) > len(tent.Items) {
		return nil
	}
	row := tent.Items[index-1]
	if !s.Assets.Items[row.ItemID].TentFloorSwitch {
		return nil
	}
	floor := byte(1)
	if c.tentFloor == 1 {
		floor = 0
	}
	s.Log.Info("tent floor change", "character", c.character.ID, "floor", floor)
	x, y := c.character.X, c.character.Y
	if err := s.sendFloorSwitch(ctx, c, tent, floor); err != nil {
		return err
	}
	c.tentFloorClickAt = now
	c.tentFloorClickX, c.tentFloorClickY = x, y
	return nil
}

// floorUp switches the rendered tent to the second floor by sending only that
// floor's layout; floorDown restores the full ground-floor furniture.
func (s *Server) floorUp(ctx context.Context, c *Session) error {
	if c.tentOwner == 0 {
		return c.send(serverwire.HeadBanner("enter your tent first"))
	}
	tent, err := s.Store.Tent(ctx, c.tentOwner)
	if err != nil {
		return err
	}
	return s.sendFloorSwitch(ctx, c, tent, 1)
}

func (s *Server) floorDown(ctx context.Context, c *Session) error {
	if c.tentOwner == 0 {
		return c.send(serverwire.HeadBanner("enter your tent first"))
	}
	tent, err := s.Store.Tent(ctx, c.tentOwner)
	if err != nil {
		return err
	}
	return s.sendFloorSwitch(ctx, c, tent, 0)
}
