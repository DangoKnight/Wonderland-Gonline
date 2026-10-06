package server

import (
	"context"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
)

// mallGameCommand ports AC75:4: forward the category byte with a zero seed,
// then refresh balances. The reference installs no quest result callbacks and
// charges no points. Existing EVE minigames retain exclusive event ownership.
func (s *Server) mallGameCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.MallGameRequestBytes {
		return protocol.ErrMalformed
	}
	if c.arcade != nil || c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	balances, err := s.Store.MallBalances(ctx, c.account.ID)
	if err != nil {
		return err
	}
	c.account.IM, c.account.IMBonus = balances.Points, balances.Bonus
	if assets.ArcadeKindSupported(p[2]) {
		c.arcade = &arcadeSession{kind: p[2], mapID: c.character.Map}
	}
	start := protocol.Builder{protocol.CommandMinigame, protocol.MinigameStart, p[2]}.U8(protocol.MallGameSeed).U8(protocol.MallGameSeed).U8(protocol.MallGameSeed)
	return s.sendAll(c, append([][]byte{start}, mallBalancePackets(balances)...))
}
