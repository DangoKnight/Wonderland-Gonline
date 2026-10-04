package server

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

func manufactureStartPacket(job *store.ManufactureJob, now time.Time) []byte {
	seconds := max(int64(0), (job.DueAt-now.UnixMilli()+time.Second.Milliseconds()-1)/time.Second.Milliseconds())
	return protocol.Builder{protocol.CommandTentManufacture, protocol.TentManufactureStart, job.Bench}.U16(job.ItemID).U32(uint32(seconds)).U8(1)
}
func (s *Server) tentManufactureCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	ref := store.CharacterRef{Account: c.account.ID, ID: c.character.ID}
	now := time.Now()
	switch p[1] {
	case protocol.TentManufactureStart:
		if len(p) != protocol.TentManufactureStartBytes {
			return protocol.ErrMalformed
		}
		if !gmIdle(c) || c.tentOwner != c.character.ID {
			return s.chatFeedback(c, "Manufacture inside your own tent.")
		}
		id := protocol.NewReader(p[3:]).U16()
		formula, known := s.Assets.Manufacturing[id]
		if !known {
			return s.chatFeedback(c, "This manufacturing formula is unavailable.")
		}
		next, job, removes, err := s.Store.StartManufacturing(ctx, ref, p[2], formula, now, s.Assets.Items, *c.character)
		if err != nil {
			return s.chatFeedback(c, "Manufacturing could not start: "+err.Error())
		}
		s.adoptSavedCharacter(c, next)
		c.manufacturing = job
		c.manufacturingLoaded = true
		c.gathering = nil
		c.fishing = nil
		for _, remove := range removes {
			s.sendOrClose(c, []byte{protocol.CommandInventory, protocol.InventoryRemove, remove.Slot, remove.Count})
		}
		s.sendOrClose(c, manufactureStartPacket(job, now))
		return s.completeManufacture(ctx, c, now)
	case protocol.TentManufactureContinue, protocol.TentManufactureStop:
		if len(p) != 2 && len(p) != protocol.TentManufactureControlBytes {
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
		bench := current.Bench
		if len(p) == protocol.TentManufactureControlBytes {
			bench = p[2]
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
			s.sendOrClose(c, manufactureStartPacket(job, now))
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
	if completed.TentOutput {
		if err = s.refreshTent(ctx, c.character.ID); err != nil {
			return err
		}
	} else {
		s.sendOrClose(c, c.character.Bag.AdditionPacket(adds), []byte{protocol.CommandTentManufacture, protocol.TentManufactureBagComplete})
	}
	s.sendOrClose(c, []byte{protocol.CommandTentManufacture, protocol.TentManufactureContinue, completed.Bench}, []byte{protocol.CommandTentManufacture, protocol.TentManufactureStopped, completed.Bench})
	effect := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateRepairEffect}.U32(c.character.ID).U16(manufactureSparkleEffect)
	s.broadcastWorld(c, effect)
	s.sendOrClose(c, effect)
	return nil
}
func (s *Server) tickManufacturing(ctx context.Context, now time.Time) {
	for _, c := range s.world {
		if !c.ready || !gmIdle(c) {
			continue
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
					s.sendOrClose(c, manufactureStartPacket(job, now))
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
