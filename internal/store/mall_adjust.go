package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"math"
	"time"
	"wonderland-gonline/internal/game"
)

var ErrMallAdjustment = errors.New("invalid mall adjustment or balance limit exceeded")

// AdjustMallBalance applies a signed delta to saved points or bonus points. Native
// deductions floor at zero; positive overflow is refused instead of wrapping.
// The balance and audit entry commit together, including the applied (clamped)
// amount. Callers supply an actor label, never credentials or tokens.
func (s *Store) AdjustMallBalance(ctx context.Context, id uint32, bonus bool, delta int64, actor string) (MallBalances, error) {
	if delta == 0 || delta < math.MinInt32 || delta > game.MaxMallPoints || actor == "" {
		return MallBalances{}, ErrMallAdjustment
	}
	var balances MallBalances
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var row accountRow
		if err := tx.Where(map[string]any{"id": id}).Take(&row).Error; err != nil {
			return err
		}
		if row.IM < 0 || row.IM > game.MaxMallPoints || row.IMBonus < 0 || row.IMBonus > game.MaxMallPoints {
			return ErrMallAdjustment
		}
		current, column, currency := row.IM, "im", "points"
		if bonus {
			current, column, currency = row.IMBonus, "im_bonus", "bonus"
		}
		updated := max(int64(0), current+delta)
		if updated > game.MaxMallPoints {
			return ErrMallAdjustment
		}
		if err := tx.Model(&accountRow{}).Where(map[string]any{"id": id}).Update(column, updated).Error; err != nil {
			return err
		}
		subject, err := json.Marshal(struct {
			Account   uint32 `json:"account"`
			Actor     string `json:"actor"`
			Currency  string `json:"currency"`
			Requested int64  `json:"requested"`
			Applied   int64  `json:"applied"`
			Before    int64  `json:"before"`
			After     int64  `json:"after"`
		}{id, actor, currency, delta, updated - current, current, updated})
		if err != nil {
			return fmt.Errorf("mall audit: %w", err)
		}
		if err := tx.Create(&auditRow{At: time.Now().UTC().Format(time.RFC3339), Action: "mall_adjust", Subject: string(subject)}).Error; err != nil {
			return err
		}
		balances = MallBalances{Points: row.IM, Bonus: row.IMBonus}
		if bonus {
			balances.Bonus = updated
		} else {
			balances.Points = updated
		}
		return nil
	})
	if err != nil {
		return MallBalances{}, err
	}
	return balances, nil
}
