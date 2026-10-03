package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"wonderland-go/internal/battle"
)

const (
	gmShutdownDefaultSeconds = 10
	gmShutdownMaximumSeconds = 300
	gmShutdownNoticeInterval = 10
	gmShutdownFinalNotices   = 5
)

func (s *Server) gmCombat(_ context.Context, c *Session, command string, args []string) error {
	if command == "battle" || command == "fight" {
		if len(args) != 1 {
			return s.chatFeedback(c, "Usage: /battle <monster template ID>")
		}
		id, err := strconv.ParseUint(args[0], 10, 16)
		if err != nil {
			return s.chatFeedback(c, "Invalid monster template ID.")
		}
		npc, ok := s.Assets.NPCs[uint16(id)]
		if !ok || npc.HP == 0 {
			return s.chatFeedback(c, "Monster template is missing or has no HP.")
		}
		return s.startBattle(c, &battleRun{}, []battle.Enemy{{Template: uint32(npc.ID), Name: npc.Name, Level: int(npc.Level), HP: int(npc.HP), Element: npc.Element}})
	}
	if len(args) != 0 {
		return s.chatFeedback(c, "Usage: /winbattle")
	}
	run := c.battle
	if run == nil || run.b == nil || run.b.Finished {
		return s.chatFeedback(c, "There is no active battle to finish.")
	}
	if run.b.Processing {
		return s.chatFeedback(c, "Wait for the current battle animation to finish.")
	}
	for _, enemy := range run.b.Defenders {
		enemy.HP = 0
	}
	s.endBattle(run, battle.Victory)
	return nil
}

func (s *Server) gmOperations(_ context.Context, c *Session, command string, args []string) error {
	if command == "kickall" {
		reason := strings.Join(args, " ")
		if reason == "" {
			reason = "Server maintenance."
		}
		count := 0
		s.mu.Lock()
		var targets []*Session
		for _, target := range s.sessions {
			if target.info.CharacterID != 0 {
				targets = append(targets, target)
			}
		}
		s.mu.Unlock()
		for _, target := range targets {
			if target.gmLevel.Load() == 0 {
				s.gmKick(target, reason)
				count++
			}
		}
		return s.chatFeedback(c, fmt.Sprintf("Disconnected %d non-GM characters.", count))
	}
	if len(args) > 1 {
		return s.chatFeedback(c, "Usage: /shutdown [1-300 seconds]")
	}
	seconds := uint64(gmShutdownDefaultSeconds)
	if len(args) == 1 {
		n, err := strconv.ParseUint(args[0], 10, 16)
		if err != nil {
			return s.chatFeedback(c, "Invalid shutdown delay.")
		}
		seconds = min(max(n, 1), gmShutdownMaximumSeconds)
	}
	if !s.shutdownAt.IsZero() {
		return s.chatFeedback(c, "A shutdown is already scheduled.")
	}
	s.shutdownAt = time.Now().Add(time.Duration(seconds) * time.Second)
	s.notice(fmt.Sprintf("Server shutdown in %d seconds.", seconds))
	return s.chatFeedback(c, "Graceful shutdown scheduled.")
}

// Shutdown stops Run through its normal connection cleanup and autosave path.
func (s *Server) Shutdown() { s.shutdownOnce.Do(func() { close(s.shutdown) }) }

func (s *Server) runShutdown(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.shutdownTick(now)
		}
	}
}

func (s *Server) shutdownTick(now time.Time) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if s.shutdownAt.IsZero() {
		return
	}
	if !now.Before(s.shutdownAt) {
		s.shutdownAt = time.Time{}
		s.notice("Server shutting down.")
		s.Shutdown()
		return
	}
	remaining := int(s.shutdownAt.Sub(now).Seconds()) + 1
	if remaining <= gmShutdownFinalNotices || remaining%gmShutdownNoticeInterval == 0 {
		s.notice(fmt.Sprintf("Server shutdown in %d seconds.", remaining))
	}
}
