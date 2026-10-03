package store

import (
	"bytes"
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MailContentMaxBytes matches the native delivery's one-byte string length.
const MailContentMaxBytes = 255

var ErrMailContent = errors.New("mail must contain 1–255 native text bytes without embedded control characters")

// TextMail keeps native encoded text losslessly; it contains no parcel attachments.
type TextMail struct {
	ID           uint64
	SenderID     uint32
	SenderName   string
	ReceiverID   uint32
	Kind         byte
	Content      []byte
	SentAtMillis int64
	Delivered    bool
}

func (TextMail) TableName() string { return "text_mail" }

func ValidateMailContent(content []byte) error {
	if len(content) == 0 || len(content) > MailContentMaxBytes || len(bytes.Trim(content, " \t\r\n")) == 0 {
		return ErrMailContent
	}
	for _, b := range content {
		if (b < 32 && b != '\n' && b != '\r' && b != '\t') || b == 127 {
			return ErrMailContent
		}
	}
	return nil
}

// SendTextMail verifies both identities and inserts the message atomically.
// SenderName is a snapshot; the caller cannot supply another sender's name.
func (s *Store) SendTextMail(ctx context.Context, sender CharacterRef, receiver uint32, kind byte, content []byte, at time.Time) (TextMail, error) {
	if err := ValidateMailContent(content); err != nil {
		return TextMail{}, err
	}
	var message TextMail
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var from, to characterRow
		if err := tx.Select("id", "name").Where(map[string]any{"id": sender.ID, "account_id": sender.Account}).Take(&from).Error; err != nil {
			return err
		}
		if err := tx.Select("id").Where(map[string]any{"id": receiver}).Take(&to).Error; err != nil {
			return err
		}
		message = TextMail{SenderID: from.ID, SenderName: from.Name, ReceiverID: to.ID, Kind: kind, Content: bytes.Clone(content), SentAtMillis: at.UTC().UnixMilli()}
		return tx.Create(&message).Error
	})
	return message, err
}

func (s *Store) PendingTextMail(ctx context.Context, receiver uint32, limit int) ([]TextMail, error) {
	if limit <= 0 {
		return nil, errors.New("positive mail batch size required")
	}
	var messages []TextMail
	err := s.orm.WithContext(ctx).Where(map[string]any{"receiver_id": receiver, "delivered": false}).Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}}).Limit(limit).Find(&messages).Error
	return messages, err
}

// MarkTextMailDelivered scopes updates to the recipient. Failed socket writes
// never call it, leaving the message available to the next login.
func (s *Store) MarkTextMailDelivered(ctx context.Context, receiver uint32, id uint64) error {
	result := s.orm.WithContext(ctx).Model(&TextMail{}).Where(map[string]any{"id": id, "receiver_id": receiver}).Update("delivered", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return persistenceError(gorm.ErrRecordNotFound)
	}
	return nil
}
