package server

import (
	"context"
	"fmt"
	"strings"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/assetsql"
)

// AssetSnapshot returns a shallow, immutable snapshot for administration readers.
// Reload replaces collections rather than modifying published collection entries.
func (s *Server) AssetSnapshot() *assets.Catalog {
	s.catalogMu.RLock()
	defer s.catalogMu.RUnlock()
	snapshot := *s.Assets
	return &snapshot
}

func (s *Server) ChangeGMLevel(ctx context.Context, account uint32, level byte) error {
	s.privilegeMu.Lock()
	defer s.privilegeMu.Unlock()
	if err := s.Store.SetGMLevel(ctx, account, level); err != nil {
		return err
	}
	s.SetGMLevel(account, level)
	return nil
}

func (s *Server) gmReload(ctx context.Context, c *Session, _ string, args []string) error {
	if len(args) > 1 {
		return s.chatFeedback(c, "Usage: /reload [all|quests|mall|drops|gms]")
	}
	scope := "all"
	if len(args) == 1 {
		scope = strings.ToLower(args[0])
	}
	switch scope {
	case "quest":
		scope = "quests"
	case "itemmall", "items":
		scope = "mall"
	case "drop":
		scope = "drops"
	case "gm":
		scope = "gms"
	}
	switch scope {
	case "all", "quests", "mall", "drops", "gms":
	default:
		return s.chatFeedback(c, "Unknown reload category.")
	}
	var next *assets.Catalog
	var err error
	if scope != "gms" {
		next, err = assetsql.LoadDatabase(s.Config.AssetsDatabase)
		if err != nil {
			s.Log.Error("GM asset reload failed", "category", scope, "error", err)
			return s.chatFeedback(c, "Asset reload failed; the current catalog is unchanged.")
		}
	}
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	quests := scope == "all" || scope == "quests"
	if quests {
		s.mu.Lock()
		loading := false
		for _, player := range s.sessions {
			if player.info.CharacterID != 0 && s.friendSessions[player.info.CharacterID] != player {
				loading = true
				break
			}
		}
		s.mu.Unlock()
		if loading {
			return s.chatFeedback(c, "Quest reload requires all online characters to finish loading.")
		}
		// Authored events are held by active dialogues/battles. Keep those definitions
		// stable until all interactions have ended, including clients still loading.
		for _, player := range s.friendSessions {
			if !gmIdle(player) {
				return s.chatFeedback(c, "Quest reload requires all online characters to finish loading and active interactions.")
			}
		}
		for id := range s.Assets.Maps {
			if _, ok := next.Maps[id]; !ok {
				return s.chatFeedback(c, fmt.Sprintf("Reload is missing map %d; the current catalog is unchanged.", id))
			}
		}
	}
	var levels map[uint32]byte
	if scope == "all" || scope == "gms" {
		s.privilegeMu.Lock()
		defer s.privilegeMu.Unlock()
		levels, err = s.Store.GMLevels(ctx)
		if err != nil {
			return err
		}
	}
	// Everything above validates before publishing any selected category.
	if quests {
		maps := make(map[uint16]assets.Map, len(s.Assets.Maps))
		for id, old := range s.Assets.Maps {
			updated := next.Maps[id]
			old.Events, old.PreEvents = updated.Events, updated.PreEvents
			maps[id] = old
		}
		s.Assets.Maps = maps
		s.Assets.Marks, s.Assets.DisabledEvents, s.Assets.Talks = next.Marks, next.DisabledEvents, next.Talks
		s.Assets.QuestVisibility = next.QuestVisibility
		s.Assets.CombatTrials = next.CombatTrials
		s.World.ReloadEvents(maps)
	}
	if scope == "all" || scope == "mall" {
		s.Assets.Mall = next.Mall
	}
	if scope == "all" || scope == "drops" {
		s.Assets.Drops = next.Drops
	}
	if levels != nil {
		s.mu.Lock()
		for _, player := range s.sessions {
			if player.account.ID != 0 {
				player.gmLevel.Store(uint32(levels[player.account.ID]))
			}
		}
		s.mu.Unlock()
	}
	return s.chatFeedback(c, "Reloaded "+scope+" from the configured SQL databases.")
}
