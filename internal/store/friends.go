package store

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"wonderland-gonline/internal/game"
)

const FriendLimit = 49

var ErrFriendLimit = errors.New("friend list is full")

func friendPair(a, b uint32) (uint32, uint32) {
	if a > b {
		return b, a
	}
	return a, b
}
func friendshipsFor(id uint32) clause.Expression {
	return clause.Or(clause.Eq{Column: "character1", Value: id}, clause.Eq{Column: "character2", Value: id})
}
func (s *Store) AddFriend(ctx context.Context, a, b uint32) error {
	if a == 0 || b == 0 || a == b {
		return errors.New("distinct characters are required")
	}
	a, b = friendPair(a, b)
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&friendshipRow{}).Where(map[string]any{"character1": a, "character2": b}).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			for _, id := range []uint32{a, b} {
				if err := tx.Model(&friendshipRow{}).Where(friendshipsFor(id)).Count(&count).Error; err != nil {
					return err
				}
				if count >= FriendLimit {
					return ErrFriendLimit
				}
			}
			return tx.Create(&friendshipRow{Character1: a, Character2: b}).Error
		}
		return nil
	})
}
func (s *Store) RemoveFriend(ctx context.Context, a, b uint32) (bool, error) {
	a, b = friendPair(a, b)
	result := s.orm.WithContext(ctx).Where(map[string]any{"character1": a, "character2": b}).Delete(&friendshipRow{})
	return result.RowsAffected > 0, result.Error
}
func (s *Store) Friends(ctx context.Context, id uint32) ([]game.Character, error) {
	result := []game.Character{}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var relations []friendshipRow
		if err := tx.Where(friendshipsFor(id)).Find(&relations).Error; err != nil {
			return err
		}
		if len(relations) == 0 {
			return nil
		}
		ids := make([]any, 0, len(relations))
		for _, r := range relations {
			other := r.Character1
			if other == id {
				other = r.Character2
			}
			ids = append(ids, other)
		}
		var rows []characterRow
		if err := tx.Omit("state").Where(clause.IN{Column: "id", Values: ids}).Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}}).Find(&rows).Error; err != nil {
			return err
		}
		var err error
		result, err = decodeCharacters(tx, rows)
		return err
	})
	return result, err
}
