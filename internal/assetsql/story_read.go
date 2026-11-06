package assetsql

import (
	"context"
	"gorm.io/gorm"
	"reflect"
)

// The deepest story table has four parent keys (cinematic, step and cue
// ordinals beneath the root). Generated array literals enforce this bound.
const storyParentDepth = 4

type storyParentKey [storyParentDepth]int64
type storyReadContextKey struct{}
type storyReadBatch struct {
	tx     *gorm.DB
	tables map[reflect.Type]any
}

// Each complete projection read gets its own lazy batch, even when another
// read occurred in this transaction before a write. Context cancellation and
// the caller's SQL transaction remain in force; no cache outlives the read.
func storyReadSession(tx *gorm.DB) *gorm.DB {
	batch := &storyReadBatch{tx: tx, tables: map[reflect.Type]any{}}
	return tx.WithContext(context.WithValue(tx.Statement.Context, storyReadContextKey{}, batch))
}

// Generated readers retain their presence flags, fixed-length checks and
// ordinal validation. Only their row retrieval changes: each required table
// is read once and split into ordered, parent-owned slice views in memory.
func storyReadRows[R any](query *gorm.DB, order string, parent storyParentKey, parentOf func(*R) storyParentKey) ([]R, error) {
	batch, _ := query.Statement.Context.Value(storyReadContextKey{}).(*storyReadBatch)
	if batch == nil {
		var rows []R
		err := query.Order(order).Find(&rows).Error
		return rows, err
	}
	typ := reflect.TypeFor[R]()
	if found, ok := batch.tables[typ]; ok {
		return found.(map[storyParentKey][]R)[parent], nil
	}
	var all []R
	if err := batch.tx.Order(order).Find(&all).Error; err != nil {
		return nil, err
	}
	groups := map[storyParentKey][]R{}
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
