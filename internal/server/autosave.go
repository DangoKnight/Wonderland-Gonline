package server

import (
	"context"
	"time"
	"wonderland-go/internal/store"
)

const (
	autosaveInterval = time.Second
	autosaveTimeout  = 5 * time.Second
)

// Caller holds worldMu, which also serializes gameplay commits and disconnects.
func (s *Server) autosaveSession(ctx context.Context, c *Session) error {
	if c.character == nil || c.autosaveBaseline == nil {
		return nil
	}
	next := c.character.Clone()
	_, err := s.Store.AutosaveCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: next.ID}, *c.autosaveBaseline, next)
	if err != nil {
		return err
	}
	c.autosaveBaseline = &next
	return nil
}

func (s *Server) autosaveCharacters(ctx context.Context) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, autosaveTimeout)
	defer cancel()
	s.tickGathering(ctx, time.Now())
	// Presence retains characters during warp loading; include those sessions.
	seen := make(map[*Session]bool)
	save := func(c *Session) {
		if c == nil || seen[c] || ctx.Err() != nil {
			return
		}
		seen[c] = true
		if err := s.autosaveSession(ctx, c); err != nil {
			s.Log.Error("character autosave failed", "session", c.info.ID, "character", c.character.ID, "error", err)
		}
	}
	for _, c := range s.world {
		save(c)
	}
	for _, c := range s.friendSessions {
		save(c)
	}
}

func (s *Server) runAutosave(ctx context.Context) {
	ticker := time.NewTicker(autosaveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.autosaveCharacters(ctx)
		}
	}
}
