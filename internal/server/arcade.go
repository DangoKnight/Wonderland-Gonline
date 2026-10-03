package server

import (
	"context"
	"wonderland-go/internal/protocol"
)

// mallGameCommand ports AC75:4: forward the category byte with a zero seed,
// then refresh balances. The reference installs no quest result callbacks and
// charges no points. Existing EVE minigames retain exclusive event ownership.
func (s *Server) mallGameCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.MallGameRequestBytes {
		return protocol.ErrMalformed
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	balances, err := s.Store.MallBalances(ctx, c.account.ID)
	if err != nil {
		return err
	}
	c.account.IM, c.account.IMBonus = balances.Points, balances.Bonus
	start := protocol.Builder{protocol.CommandMinigame, protocol.MinigameStart, p[2]}.U8(protocol.MallGameSeed).U8(protocol.MallGameSeed).U8(protocol.MallGameSeed)
	return s.sendAll(c, append([][]byte{start}, mallBalancePackets(balances)...))
}
