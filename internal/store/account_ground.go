package store

import (
	"context"
	"database/sql"
	"gorm.io/gorm"
	"time"
	"wonderland-gonline/internal/game"
)

func migrateAccountGroundClaims(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS account_ground_claims (
account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
scope_id INTEGER NOT NULL CHECK(scope_id BETWEEN 1 AND 65535),
node_id INTEGER NOT NULL CHECK(node_id BETWEEN 1 AND 255),
claimed_at INTEGER NOT NULL,
PRIMARY KEY(account_id,scope_id,node_id));`)
	return err
}

// Character ownership is read from SQL, never inferred from numeric character IDs.
func (s *Store) AccountGroundClaims(ctx context.Context, characterID uint32, scopeID uint16) (map[byte]bool, error) {
	var slots []byte
	err := s.orm.WithContext(ctx).Raw(`SELECT node_id FROM account_ground_claims
WHERE scope_id=? AND account_id=(SELECT account_id FROM characters WHERE id=?)`, scopeID, characterID).Scan(&slots).Error
	claimed := make(map[byte]bool, len(slots))
	for _, slot := range slots {
		claimed[slot] = true
	}
	return claimed, err
}

// The permanent account claim and reward share a transaction. Failed grants do
// not consume the node; deleting/recreating a character does not reset its account.
func (s *Store) ClaimAccountGroundItem(ctx context.Context, ref CharacterRef, mapID, scopeID uint16, slot byte, item game.Item, limit byte, now time.Time, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, error) {
	var next game.Character
	if mapID == 0 || scopeID == 0 || slot == 0 {
		return next, ErrResourceUnavailable
	}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		result := tx.Exec(`INSERT INTO account_ground_claims(account_id,scope_id,node_id,claimed_at) VALUES(?,?,?,?)
ON CONFLICT(account_id,scope_id,node_id) DO NOTHING`, ref.Account, scopeID, slot, now.UnixMilli())
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrResourceUnavailable
		}
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if next.Map != mapID {
			return ErrResourceUnavailable
		}
		if err := preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		if _, err := next.Bag.Grant(item, int(item.Count), limit, items); err != nil {
			return err
		}
		if err := next.Validate(); err != nil {
			return err
		}
		return saveCharacter(tx, ref, next)
	})
	if err != nil {
		return game.Character{}, err
	}
	return next, nil
}
