package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

const (
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
	return a != nil && b != nil && a.character != nil && b.character != nil && a.character.Map == b.character.Map && a.tentOwner == b.tentOwner
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
func (s *Server) tentSnapshot(ctx context.Context, c *Session) error {
	tent, err := s.Store.Tent(ctx, c.tentOwner)
	if err != nil {
		return err
	}
	packets := [][]byte{
		{protocol.CommandTent, protocol.TentInteriorLayout, max(byte(1), tent.Type), 0},
		protocol.Builder{protocol.CommandTentFurniture, protocol.TentFloorSurface}.U16(tent.Floor),
		protocol.Builder{protocol.CommandTentFurniture, protocol.TentWallpaperSurface}.U16(tent.Wallpaper),
	}
	if tent.Enlarged {
		packets = append(packets, []byte{protocol.CommandTent, protocol.TentInteriorLayout, max(byte(1), tent.Type), 1})
	}
	if c.ready {
		tree := serverwire.NativeTentTree(tent)
		if len(tree) > protocol.MaxPayload {
			return errors.New("tent furniture exceeds the native snapshot limit")
		}
		packets = append(packets, tree)
	}
	packets = append(packets, []byte{protocol.CommandTent, protocol.TentInteriorStatus, tentLockedByte(tent.Locked), 0}, []byte{protocol.CommandInventory, protocol.InventoryTentPlayers})
	occupants := append(s.peers(c), c)
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
	if c.openTent != nil && owner != c.character.ID {
		if err = s.closePlayerTent(ctx, c); err != nil {
			return err
		}
	}
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
	return s.tentSnapshot(ctx, c)
}
func (s *Server) exitTent(ctx context.Context, c *Session) error {
	if c.tentOwner == 0 || c.character.TentReturn == nil {
		return nil
	}
	dst := *c.character.TentReturn
	return s.teleport(ctx, c, world.Destination{Map: dst.Map, X: dst.X, Y: dst.Y}, 0)
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
		if len(p) != protocol.TentHeaderRequestBytes {
			return protocol.ErrMalformed
		}
		return s.closePlayerTent(ctx, c)
	case protocol.TentExit:
		if !gmIdle(c) {
			return nil
		}
		if len(p) != protocol.TentHeaderRequestBytes {
			return protocol.ErrMalformed
		}
		return s.exitTent(ctx, c)
	default:
		return ErrUnsupported
	}
}
func (s *Server) tentFurnitureCommand(ctx context.Context, c *Session, p []byte) error {
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
		if err := s.sendAll(c, [][]byte{next.Bag.AdditionPacket(adds), protocol.Builder{protocol.CommandTentFurniture, protocol.TentPickupFurniture}.U16(raw)}); err != nil {
			return err
		}
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
		} else {
			tent, err := s.Store.Tent(ctx, c.character.ID)
			if err != nil {
				return err
			}
			row := tent.Items[len(tent.Items)-1]
			confirmation := protocol.Builder{protocol.CommandTentFurniture, protocol.TentPlaceFurniture}.Bytes(serverwire.TentTreeRecord(uint16(len(tent.Items)), row.ItemID, uint32(row.X), uint32(row.Y), 1, row.Rotation, 0, nil, row.Floor))
			s.sendOrClose(c, confirmation)
		}

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

func tentLockedByte(locked bool) byte {
	if locked {
		return 1
	}
	return 0
}
