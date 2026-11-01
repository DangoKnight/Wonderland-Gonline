package server

import (
	"context"
	"fmt"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

func (s *Server) rebirthJobCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 3 {
		return protocol.ErrMalformed
	}
	job := p[2]
	if job < game.JobKiller || job > game.JobSeer {
		return protocol.ErrMalformed
	}
	var cape uint16
	for _, class := range s.Assets.RebornClasses {
		if class.Enabled && class.Job == job {
			cape = class.CapeID
			break
		}
	}
	if cape == 0 {
		return nil
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	switch p[1] {
	case protocol.RebirthJobConfirm:
		if len(p) != 3 {
			return protocol.ErrMalformed
		}
		if c.view == nil || !c.view.RebornFormOpen {
			return nil
		}
		return s.performReborn(ctx, c, job, false)
	case protocol.RebirthJobChange:
		if len(p) != 5 {
			return protocol.ErrMalformed
		}
		return s.changeRebornJob(ctx, c, job, cape, p[3], p[4])
	}
	return ErrUnsupported
}

func (s *Server) changeRebornJob(ctx context.Context, c *Session, job byte, cape uint16, mirrorSlot, capeSlot byte) error {
	if mirrorSlot < 1 || mirrorSlot > game.BagSize || capeSlot < 1 || capeSlot > game.BagSize || mirrorSlot == capeSlot {
		return nil
	}
	var packets [][]byte
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		mirror, coat := next.Bag[mirrorSlot-1], next.Bag[capeSlot-1]
		if !next.Reborn || next.Job == job || mirror.ID != game.TransferMirrorItemID || mirror.Empty() || mirror.Locked || coat.ID != cape || coat.Empty() || coat.Locked {
			return fmt.Errorf("job change requires a Transfer Mirror and the selected job's cape")
		}
		if err := next.Bag.Remove(mirrorSlot, 1); err != nil {
			return err
		}
		next.Job, next.RebornJob = job, job
		packets = s.unlockQualifiedSkills(next, true)
		next.Refill(s.Assets.Items)
		return nil
	}, *c.character)
	if err != nil {
		return s.chatFeedback(c, "Job change failed: "+err.Error())
	}
	s.adoptSavedCharacter(c, next)
	packets = append([][]byte{{protocol.CommandInventory, protocol.InventoryRemove, mirrorSlot, 1}, next.ExpPacket()}, packets...)
	packets = append(packets, next.StatPackets(s.Assets.Items)...)
	self, err := next.AppearancePacket(false)
	if err != nil {
		return err
	}
	peer, err := next.AppearancePacket(true)
	if err != nil {
		return err
	}
	s.broadcastWorld(c, peer)
	packets = append(packets, self)
	return s.sendAll(c, packets)
}
