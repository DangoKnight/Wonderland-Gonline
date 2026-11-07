package assetsql

import (
	"context"
	"reflect"

	"gorm.io/gorm"
)

type catalogReadContextKey struct{}

type catalogReadBatch struct {
	tx     *gorm.DB
	tables map[reflect.Type]any
}

// Each root projection owns a fresh lazy batch. It never survives a read or
// crosses SQL transactions, so a later read also observes intervening writes.
// Queries stay on the caller's transaction and retain its cancellation context.
func catalogReadSession(tx *gorm.DB) *gorm.DB {
	batch := &catalogReadBatch{tx: tx, tables: make(map[reflect.Type]any)}
	return tx.WithContext(context.WithValue(tx.Statement.Context, catalogReadContextKey{}, batch))
}

// Generated keys include every ancestor with its original type, including
// string map keys. Ordering by all keys makes each parent's rows contiguous.
// Presence, ordinal and fixed-length checks remain in the generated readers.
func catalogReadRows[R any, K comparable](query *gorm.DB, order string, parent K, parentOf func(*R) K) ([]R, error) {
	if err := query.Statement.Context.Err(); err != nil {
		return nil, err
	}
	batch, _ := query.Statement.Context.Value(catalogReadContextKey{}).(*catalogReadBatch)
	if batch == nil {
		var rows []R
		err := query.Order(order).Find(&rows).Error
		return rows, err
	}
	typ := reflect.TypeFor[R]()
	if found, ok := batch.tables[typ]; ok {
		return found.(map[K][]R)[parent], nil
	}
	var all []R
	if err := batch.tx.Order(order).Find(&all).Error; err != nil {
		return nil, err
	}
	groups := make(map[K][]R)
	for start := 0; start < len(all); {
		key := parentOf(&all[start])
		end := start + 1
		for end < len(all) && parentOf(&all[end]) == key {
			end++
		}
		groups[key] = all[start:end]
		start = end
	}
	batch.tables[typ] = groups
	return groups[parent], nil
}
