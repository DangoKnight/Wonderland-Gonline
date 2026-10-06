package server

import (
	"context"
	"fmt"
	"sort"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/store"
)

func (s *Server) grantStarterPack(ctx context.Context, ref store.CharacterRef, reservations ...game.Character) (game.Character, []game.Addition, error) {
	return s.Store.GrantStarterPack(ctx, ref, s.Assets.StarterItems, s.Assets.Items, reservations...)
}

type StarterPackDelivery struct {
	CharacterID uint32 `json:"character_id"`
	Name        string `json:"name"`
	Delivered   bool   `json:"delivered"`
	Error       string `json:"error,omitempty"`
}

const adminStarterPackBatchTimeout = 5 * time.Second

// AdminGiveStarterPacks targets loaded idle online characters. Each whole pack
// commits before delivery packets; failed or busy recipients are reported.
func (s *Server) AdminGiveStarterPacks(ctx context.Context) ([]StarterPackDelivery, error) {
	s.catalogMu.RLock()
	defer s.catalogMu.RUnlock()
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if len(s.Assets.StarterItems) == 0 {
		return nil, fmt.Errorf("starter pack is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, adminStarterPackBatchTimeout)
	defer cancel()
	players := s.gmOnline()
	sort.Slice(players, func(i, j int) bool { return players[i].character.ID < players[j].character.ID })
	result := make([]StarterPackDelivery, 0, len(players))
	for _, c := range players {
		row := StarterPackDelivery{CharacterID: c.character.ID, Name: c.character.Name}
		if !c.ready || !gmIdle(c) || c.stall != nil || c.trade != nil {
			row.Error = ErrAdminPlayerUnavailable.Error()
		} else {
			next, adds, err := s.grantStarterPack(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, *c.character)
			if err != nil {
				row.Error = err.Error()
			} else {
				s.adoptSavedCharacter(c, next)
				row.Delivered = true
				s.sendOrClose(c, next.Bag.AdditionPacket(adds))
			}
		}
		result = append(result, row)
	}
	s.Log.Info("administrator starter pack delivery", "recipients", result)
	return result, nil
}
