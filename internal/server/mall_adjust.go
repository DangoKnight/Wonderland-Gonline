package server

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"wonderland-go/internal/store"
)

// AdjustMallBalance is the administrator operation. Exclude account deletion and
// authentication publication, then order balance receipts against purchases.
func (s *Server) AdjustMallBalance(ctx context.Context, account uint32, bonus bool, delta int64) (store.MallBalances, error) {
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	return s.adjustMallBalance(ctx, account, bonus, delta, "administrator")
}

// Caller holds worldMu. A receipt failure closes that client; the committed
// adjustment still succeeds so an administrator is not invited to retry a grant.
func (s *Server) adjustMallBalance(ctx context.Context, account uint32, bonus bool, delta int64, actor string) (store.MallBalances, error) {
	balances, err := s.Store.AdjustMallBalance(ctx, account, bonus, delta, actor)
	if err != nil {
		return store.MallBalances{}, err
	}
	for _, c := range s.world {
		if c.account.ID != account || !c.ready {
			continue
		}
		c.account.IM, c.account.IMBonus = balances.Points, balances.Bonus
		for _, p := range mallBalancePackets(balances) {
			s.sendOrClose(c, p)
		}
	}
	return balances, nil
}

// gmMallPoints ports AC02's :im/:points_im/:mallpoints and GmManager.AddMallPoints.
// Missing explicit targets are refused, fixing the reference's self-grant fallback.
func (s *Server) gmMallPoints(ctx context.Context, c *Session, words []string) error {
	if len(words) < 2 {
		return nil
	}
	delta, err := strconv.ParseInt(words[1], 10, 32)
	if err != nil || delta == 0 {
		return nil
	}
	target := c
	if len(words) > 2 {
		target = s.findOnline(words[2])
		if target == nil {
			return nil
		}
	}
	_, err = s.adjustMallBalance(ctx, target.account.ID, false, delta, fmt.Sprintf("gm:%d/character:%d", c.account.ID, c.character.ID))
	if errors.Is(err, store.ErrMallAdjustment) {
		return nil
	}
	return err
}
