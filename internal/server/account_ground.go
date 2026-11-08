package server

import (
	"context"
	"wonderland-gonline/internal/game"
)

func (s *Server) accountGroundClaims(ctx context.Context, char game.Character) (map[byte]bool, error) {
	if scope := s.World.AccountGroundScope(char.Map); scope != 0 {
		return s.Store.AccountGroundClaims(ctx, char.ID, scope)
	}
	return nil, nil
}
