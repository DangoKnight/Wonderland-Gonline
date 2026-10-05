package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

const (
	gmMuteDefaultMinutes = 10
	gmMuteMaximumMinutes = 365 * 24 * 60
	gmJailMap            = 10000
	gmJailX              = 600
	gmJailY              = 600
	gmReleaseMap         = 10001
	gmReleaseX           = 800
	gmReleaseY           = 750
)

func (s *Server) gmKick(c *Session, reason string) {
	if reason == "" {
		reason = "Disconnected by a game master."
	}
	_ = s.chatFeedback(c, reason)
	_ = c.conn.Close()
}

func (s *Server) gmModeration(ctx context.Context, c *Session, command string, args []string) error {
	release := command == "unmute" || command == "unjail"
	if len(args) < 1 || len(args) > 2 || (release && len(args) != 1) {
		usage := "Usage: /" + command + " <character>"
		if !release {
			usage += " [minutes]"
		}
		return s.chatFeedback(c, usage)
	}
	minutes := uint64(gmMuteDefaultMinutes)
	if !release && len(args) == 2 {
		n, err := strconv.ParseUint(args[1], 10, 32)
		if err != nil || n < 1 || n > gmMuteMaximumMinutes {
			return s.chatFeedback(c, "Duration must be 1–525600 minutes.")
		}
		minutes = n
	}
	target, err := s.gmTarget(c, args[0])
	if target == nil {
		return err
	}
	jail := command == "jail" || command == "unjail"
	if jail && !gmIdle(target) {
		return s.chatFeedback(c, "That character must finish active interactions first.")
	}
	next := target.character.Clone()
	next.MutedUntil = time.Time{}
	if !release {
		next.MutedUntil = time.Now().UTC().Add(time.Duration(minutes) * time.Minute)
	}
	dst := world.Destination{Map: gmJailMap, X: gmJailX, Y: gmJailY}
	if release {
		dst = world.Destination{Map: gmReleaseMap, X: gmReleaseX, Y: gmReleaseY}
	}
	if jail {
		if target.openTent != nil {
			if err = s.closePlayerTent(ctx, target); err != nil {
				return err
			}
		}
		next.TentReturn = nil
		if _, ok := s.World.Map(dst.Map); !ok {
			return s.chatFeedback(c, "The moderation destination is missing from the assets database.")
		}
		next.Map, next.X, next.Y = dst.Map, dst.X, dst.Y
		// Save both the mute and location together, retaining the old runtime map
		// until its peers have received the departure.
		if err = s.Store.UpdateCharacter(ctx, target.account.ID, next.ID, func(stored *game.Character) error { *stored = next; return nil }); err != nil {
			return err
		}
		baseline := next.Clone()
		target.autosaveBaseline = &baseline
		target.character.MutedUntil = next.MutedUntil
		if err = s.teleportPrelude(target); err == nil {
			err = s.teleportAfterSave(target, dst, 0)
		}
		if err != nil {
			s.depart(target, protocol.Builder{protocol.CommandMapAcknowledgment}.U32(next.ID).U16(dst.Map).U16(dst.X).U16(dst.Y).U16(0).U8(0))
			target.character.Map, target.character.X, target.character.Y = dst.Map, dst.X, dst.Y
			target.conn.Close()
			return s.chatFeedback(c, "Moderation saved; the target must reconnect.")
		}
	} else if err = s.commit(ctx, target, next); err != nil {
		return err
	}
	if err = s.chatFeedback(target, "A game master applied /"+command+"."); err != nil {
		target.conn.Close()
	}
	if target != c {
		return s.chatFeedback(c, fmt.Sprintf("%s: /%s saved.", target.character.Name, command))
	}
	return nil
}

func (s *Server) gmVisibility(_ context.Context, c *Session, command string, args []string) error {
	if len(args) != 0 {
		return s.chatFeedback(c, "Usage: /invis, /hide or /unhide")
	}
	hidden := !c.invisible
	if command == "hide" {
		hidden = true
	}
	if command == "unhide" {
		hidden = false
	}
	if hidden == c.invisible {
		return s.chatFeedback(c, "Visibility is already set.")
	}
	if hidden {
		s.broadcastWorld(c, protocol.Builder{protocol.CommandMapAcknowledgment}.U32(c.character.ID).U16(0).U16(0).U16(0).U16(0).U8(0))
		s.closeStall(c)
		c.gathering = nil
		c.invisible = true
	} else {
		packets, err := peerPackets(*c.character, false, false)
		if err != nil {
			return err
		}
		packets = append(packets, s.peerPetPackets(c.character)...)
		if c.emote != 0 {
			packets = append(packets, protocol.Builder{protocol.CommandPose, protocol.PoseBroadcast}.U32(c.character.ID).U8(c.emote))
		}
		c.invisible = false
		for _, packet := range packets {
			s.broadcastWorld(c, packet)
		}
	}
	return s.chatFeedback(c, "GM visibility: "+strings.ToUpper(strconv.FormatBool(!hidden)))
}
