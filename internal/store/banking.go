package store

import (
	"context"
	"gorm.io/gorm"
	"wonderland-gonline/internal/game"
)

type BankBalances struct {
	Gold     uint32
	BankGold uint32
}

// TransferBankGold checks ownership, funds and both limits in the SQL write.
// Starting with a conditional UPDATE serializes independent SQLite connections
// without a read-to-write upgrade race. Unrelated character rows stay untouched.
func (s *Store) TransferBankGold(ctx context.Context, ref CharacterRef, amount uint32, deposit bool) (BankBalances, bool, error) {
	var balances BankBalances
	var changed bool
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var result *gorm.DB
		if deposit {
			result = tx.Exec(`UPDATE character_state SET gold=gold-?, bank_gold=bank_gold+?
WHERE character_id=? AND ?>0 AND gold>=? AND bank_gold<=?-?
AND EXISTS(SELECT 1 FROM characters WHERE characters.id=character_state.character_id AND account_id=?)`, amount, amount, ref.ID, amount, amount, game.MaxBankGold, amount, ref.Account)
		} else {
			result = tx.Exec(`UPDATE character_state SET gold=gold+?, bank_gold=bank_gold-?
WHERE character_id=? AND ?>0 AND bank_gold>=? AND gold<=?-?
AND EXISTS(SELECT 1 FROM characters WHERE characters.id=character_state.character_id AND account_id=?)`, amount, amount, ref.ID, amount, amount, game.MaxGold, amount, ref.Account)
		}
		if result.Error != nil {
			return result.Error
		}
		changed = result.RowsAffected == 1
		return tx.Table("character_state AS state").Select("state.gold,state.bank_gold").Joins("JOIN characters AS owner ON owner.id=state.character_id").Where("state.character_id=? AND owner.account_id=?", ref.ID, ref.Account).Take(&balances).Error
	})
	if err != nil {
		return BankBalances{}, false, persistenceError(err)
	}
	return balances, changed, nil
}
