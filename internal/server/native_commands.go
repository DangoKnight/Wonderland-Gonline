package server

import (
	"context"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// nativeWord accepts the reference's optional UInt16 operand, defaulting only
// when it is entirely absent. A partial operand or trailing data is malformed.
func nativeWord(p []byte, fallback uint16) (uint16, error) {
	switch len(p) {
	case 2:
		return fallback, nil
	case 4:
		return uint16(p[2]) | uint16(p[3])<<8, nil
	default:
		return 0, protocol.ErrMalformed
	}
}

// nativeCommand ports small AC handlers. World dispatch owns worldMu and the
// normal gameplay gates; only read-only synchronization bypasses those gates.
func (s *Server) nativeCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	switch p[0] {
	case protocol.CommandAppearance:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		packet, err := c.character.AppearancePacket(true)
		if err != nil {
			return err
		}
		return c.send(packet)
	case protocol.CommandHeartbeatSync:
		if p[1] != protocol.HeartbeatSyncRequest {
			return ErrUnsupported
		}
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		return s.sendAll(c, [][]byte{
			{protocol.CommandHeartbeatSync, protocol.HeartbeatSyncRequest, 0},
			{protocol.CommandHeartbeatSync, protocol.HeartbeatSyncStatus, protocol.HeartbeatSyncStatusWireValue9, protocol.HeartbeatSyncStatusWireValue2},
		})
	case protocol.CommandMovie:
		if p[1] != protocol.MoviePlaybackAcknowledgment {
			return ErrUnsupported
		}
		id, err := nativeWord(p, protocol.NativeDefaultMovieID)
		if err != nil {
			return err
		}
		return c.send(protocol.Builder{protocol.CommandMovie, protocol.MoviePlaybackAcknowledgment}.U16(id).U8(protocol.NativeActive).U32(0))
	case protocol.CommandMallMatrixRequest:
		if p[1] != protocol.MallMatrixQuery {
			return ErrUnsupported
		}
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		return s.sendAll(c, [][]byte{
			protocol.Builder{protocol.CommandMallMatrix, protocol.MallMatrixRows, 0, protocol.MallMatrixQuery}.U16(protocol.MallMatrixRowWireID27).U8(1).U16(protocol.MallMatrixRowWireID29).U8(2).U16(protocol.MallMatrixRowWireID24).U8(0),
			protocol.Builder{protocol.CommandMallClaimState, protocol.MallClaimSnapshot}.Bytes(make([]byte, protocol.MallClaimReservedBytes)),
		})
	case protocol.CommandScene:
		id, err := nativeWord(p, protocol.NativeDefaultEntityID)
		if err != nil {
			return err
		}
		return c.send(protocol.Builder{protocol.CommandScene, p[1]}.U16(id).U8(protocol.NativeActive))
	case protocol.CommandGesture:
		id, err := nativeWord(p, protocol.NativeDefaultGestureID)
		if err != nil {
			return err
		}
		packet := protocol.Builder{protocol.CommandGesture, p[1]}.U32(c.character.ID).U16(id)
		s.broadcastWorld(c, packet)
		return c.send(packet)
	case protocol.CommandTitle:
		id, err := nativeWord(p, 0)
		if err != nil {
			return err
		}
		// Update only the authoritative SQL field; leave pending walking dirty.
		if err = s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error { stored.Title = id; return nil }); err != nil {
			return err
		}
		c.character.Title = id
		if c.autosaveBaseline != nil {
			c.autosaveBaseline.Title = id
		}
		if err = c.send(protocol.Builder{protocol.CommandTitle, p[1]}.U16(id)); err != nil {
			return err
		}
		packet := protocol.Builder{protocol.CommandTitle, protocol.TitleBroadcast}.U32(c.character.ID).U16(id)
		s.broadcastWorld(c, packet)
		return c.send(packet)
	case protocol.CommandRebornJob:
		if len(p) != 2 && len(p) != 3 {
			return protocol.ErrMalformed
		}
		job := byte(protocol.NativeDefaultRebornJob) // AC66's absent-operand compatibility default.
		if len(p) == 3 {
			job = p[2]
		}
		if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error { stored.RebornJob = job; return nil }); err != nil {
			return err
		}
		c.character.RebornJob = job
		if c.autosaveBaseline != nil {
			c.autosaveBaseline.RebornJob = job
		}
		return c.send([]byte{protocol.CommandRebornJob, p[1], job, protocol.NativeActive})
	case protocol.CommandBath:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		if c.event != nil {
			return nil
		}
		if err := s.heal(ctx, c, nil); err != nil {
			return err
		}
		return c.send([]byte{protocol.CommandBath, p[1], protocol.NativeActive})
	case protocol.CommandFishing:
		return s.fishingCommand(ctx, c, p)
	}
	return ErrUnsupported
}

// waypointCommand accepts the reference X/Y layout and an inferred map/X/Y
// adapter based on AC07's documented intent. Native captures are still needed. Coordinates go through AC6's terrain, interaction and vehicle checks,
// checkpointing and encounter processing instead of bypassing movement rules.
func (s *Server) waypointCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) == 2 {
		return c.send(protocol.Builder{protocol.CommandPosition, p[1]}.U16(c.character.X).U16(c.character.Y))
	}
	if len(p) != 6 && len(p) != 8 {
		return protocol.ErrMalformed
	}
	offset := 2
	if len(p) == 8 {
		mapID := uint16(p[2]) | uint16(p[3])<<8
		if mapID != c.character.Map {
			return c.send(c.character.PositionPacket())
		}
		offset += 2
	}
	r := protocol.NewReader(p[offset:])
	x, y := r.U16(), r.U16()
	if x != 0 || y != 0 {
		if err := s.movementCommand(ctx, c, protocol.Builder{protocol.CommandMovement, protocol.MovementMove, 0}.U16(x).U16(y)); err != nil {
			return err
		}
	}
	return c.send(protocol.Builder{protocol.CommandPosition, p[1]}.U16(c.character.X).U16(c.character.Y))
}

func (s *Server) nativeMallCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.NativeMallWindow:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		if err := s.sendMallBalances(ctx, c); err != nil {
			return err
		}
		packet := protocol.Builder{protocol.CommandNativeMall, protocol.NativeMallWindow}
		for slot := 1; slot <= protocol.NativeMallWindowSlots; slot++ {
			packet = packet.U8(byte(slot))
		}
		return c.send(packet)
	case protocol.NativeMallBalance:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		return s.sendMallBalances(ctx, c)
	case protocol.NativeMallBuy:
		if len(p) != 3 {
			return protocol.ErrMalformed
		}
		slot := int(p[2])
		entries := s.mallEntries(false)
		if slot == 0 || slot > len(entries) {
			return c.send(headBanner("Item Mall: this slot is unavailable. No points deducted."))
		}
		entry := entries[slot-1]
		// One purchase means one catalog bundle, not Count copies of that bundle.
		return s.purchaseMall(ctx, c, []mallCartRow{{item: entry.ID, category: mallCategory(entry), quantity: 1, order: mallOrder(entry)}}, false, false)
	default:
		return ErrUnsupported
	}
}
