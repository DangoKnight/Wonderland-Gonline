package store

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"wonderland-gonline/internal/game"
)

var ErrMallFunds = errors.New("insufficient mall points")
var ErrMallPurchase = errors.New("invalid mall purchase")

type MallBalances struct {
	Points int64 `json:"points"`
	Bonus  int64 `json:"bonus"`
}

func (s *Store) MallBalances(ctx context.Context, account uint32) (MallBalances, error) {
	var row accountRow
	err := s.orm.WithContext(ctx).Select("im", "im_bonus").Where(map[string]any{"id": account}).Take(&row).Error
	return MallBalances{Points: row.IM, Bonus: row.IMBonus}, persistenceError(err)
}

// PurchaseMall debits the authoritative account balance and delivers inventory in
// one transaction. The callback receives saved state, never a stale session copy.
func (s *Store) PurchaseMall(ctx context.Context, ref CharacterRef, bonus bool, cost int64, deliver func(*game.Character) error) (game.Character, MallBalances, error) {
	if cost < 0 || cost > game.MaxMallPoints || deliver == nil {
		return game.Character{}, MallBalances{}, ErrMallPurchase
	}
	var result game.Character
	var balances MallBalances
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var account accountRow
		if err := tx.Where(map[string]any{"id": ref.Account}).Take(&account).Error; err != nil {
			return err
		}
		if account.Banned {
			return ErrCredentials
		}
		balance, column := account.IM, "im"
		if bonus {
			balance, column = account.IMBonus, "im_bonus"
		}
		if balance < cost {
			return ErrMallFunds
		}
		if balance > game.MaxMallPoints {
			return ErrMallPurchase
		}
		character, err := loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		id, slot, name := character.ID, character.Slot, character.Name
		if err := deliver(&character); err != nil {
			return err
		}
		if character.ID != id || character.Slot != slot || character.Name != name {
			return ErrMallPurchase
		}
		if err := character.Validate(); err != nil {
			return err
		}
		if err := tx.Model(&accountRow{}).Where(map[string]any{"id": ref.Account}).Update(column, balance-cost).Error; err != nil {
			return err
		}
		if err := saveCharacter(tx, ref, character); err != nil {
			return err
		}
		balances = MallBalances{Points: account.IM, Bonus: account.IMBonus}
		if bonus {
			balances.Bonus -= cost
		} else {
			balances.Points -= cost
		}
		result = character
		return nil
	})
	if err != nil {
		return game.Character{}, MallBalances{}, err
	}
	return result, balances, nil
}
