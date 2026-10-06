package server

import (
	"context"
	"errors"
	"math"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

// EditAdminCharacterFields only changes the selected tab's owned fields. It
// preserves unrelated SQL state and pending session walking and publishes after commit.
func (s *Server) EditAdminCharacterFields(ctx context.Context, id uint32, version string, requested game.Character, scope string) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	if s.adminCharacterLoading(id) {
		return ErrAdminPlayerUnavailable
	}
	c := s.friendSessions[id]
	if c != nil && (!gmIdle(c) || c.stall != nil || c.openTent != nil || c.tentOwner != 0) {
		return ErrAdminPlayerUnavailable
	}
	row, err := s.Store.AdminCharacter(ctx, id)
	if err != nil {
		return err
	}
	if row.Version != version || version == "" {
		return store.ErrAdminConflict
	}
	next := row.State.Clone()
	switch scope {
	case "inventory":
		next.Bag, next.Equipment, next.Storage = requested.Bag, requested.Equipment, requested.Storage
		next.RecalculateVitals(s.Assets.Items)
	case "stats":
		next.Base, next.Skills, next.Level, next.EXP, next.StatPoints = requested.Base, requested.Skills, requested.Level, requested.EXP, requested.StatPoints
		next.RecalculateVitals(s.Assets.Items)
	case "player_settings":
		next.Settings = requested.Clone().Settings
	case "quests":
		next.Quests = requested.Clone().Quests
		for id, q := range next.Quests {
			if id == 0 || q.ID != id || q.State > game.Failed || q.Step < 0 || q.Step > assets.MaxQuestSteps || q.Kills < 0 || q.Kills > math.MaxInt32 {
				return errors.New("invalid quest progress")
			}
		}
	default:
		return errors.New("unsupported live character tab")
	}
	if err = s.validateAdminState(next); err != nil {
		return err
	}
	var packets [][]byte
	switch scope {
	case "inventory":
		// Native AC23:5 overlays slots; remove old contents before publishing the replacement.
		if c != nil {
			for i, item := range c.character.Bag {
				if !item.Empty() {
					packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, byte(i + 1), item.Count})
				}
			}
		}
		packets = append(packets, next.Bag.Packet(protocol.CommandInventory, protocol.InventoryItems), next.EquipmentPacket())
		packets = append(packets, next.StatPackets(s.Assets.Items)...)
	case "stats":
		snapshot := skillSnapshotWithRemovals(row.State, next)
		base, err := s.nativeStatsSnapshot(snapshot).BaseStatsPacket(func(id uint16) (uint16, bool) { skill, ok := s.Assets.Skills[id]; return skill.TableOrder, ok })
		if err != nil {
			return err
		}
		packets = append(packets, base)
		packets = append(packets, next.StatPackets(s.Assets.Items)...)
	case "player_settings":
		packets = append(packets, next.Preferences().Packet())
	}
	if err = s.Store.ReplaceAdminCharacter(ctx, id, version, next); err != nil {
		return err
	}
	if c == nil {
		return nil
	}
	s.adoptSavedCharacter(c, next)
	if scope == "quests" {
		packets = append(packets, s.World.Journal(c.character, c.view)...)
		packets = append(packets, storyConstellations(c.character))
		packets = append(packets, s.World.Sync(c.character, c.view, false)...)
	}
	packets = append(packets, []byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh})
	if err := s.sendAll(c, packets); err != nil {
		c.conn.Close()
	}
	if scope == "inventory" {
		s.broadcastWorld(c, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateEquipmentSnapshot}.U32(id).Bytes(next.WornEquipment()))
	}
	if c.party != nil {
		s.partyUpdate(c.party)
	}
	return nil
}
