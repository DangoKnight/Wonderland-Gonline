package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"gorm.io/gorm"
	"time"
)

const messageBoardPageLimit = 10
const MessageBoardPostsPerAuthor = 3
const messageBoardMaxPosts = messageBoardPageLimit * 255 // Native one-byte page number.
const messageBoardTitleLimit = 30
const messageBoardContentLimit = 151
const MessageBoardLifetime = 24 * time.Hour

func activeMessageBoardPosts(tx *gorm.DB, at time.Time) *gorm.DB {
	return tx.Where("posted_at_millis > ?", at.Add(-MessageBoardLifetime).UnixMilli())
}

var ErrMessageBoardAuthorLimit = errors.New("message board author limit reached")
var ErrMessageBoardFull = errors.New("message board is full")
var ErrMessageBoardText = errors.New("invalid message board text")

// MessageBoardPost is durable community content. Text fields contain the native
// Big5-compatible wire bytes, as mail does; presentation does not own the data.
type MessageBoardPost struct {
	ID             uint32 `gorm:"primaryKey;autoIncrement:false"`
	AuthorID       uint32
	AuthorName     []byte
	Title          []byte
	Content        []byte
	PostedAtMillis int64
}

func (MessageBoardPost) TableName() string { return "message_board_posts" }

// MessageBoardPage reads count and entries from one SQL snapshot. Offset and
// limit are bounded by the caller's protocol page size.
func (s *Store) MessageBoardPage(ctx context.Context, offset, limit int) (posts []MessageBoardPost, count int64, err error) {
	if offset < 0 || limit < 1 || limit > messageBoardPageLimit {
		return nil, 0, errors.New("invalid message board page")
	}
	tx := s.orm.WithContext(ctx).Begin(&sql.TxOptions{ReadOnly: true})
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer tx.Rollback()
	live := activeMessageBoardPosts(tx, time.Now())
	if err = live.Model(&MessageBoardPost{}).Count(&count).Error; err != nil {
		return
	}
	if err = live.Order("posted_at_millis DESC, id DESC").Offset(offset).Limit(limit).Find(&posts).Error; err != nil {
		return
	}
	err = tx.Commit().Error
	return
}

func (s *Store) MessageBoardEntry(ctx context.Context, id uint32) (post MessageBoardPost, err error) {
	err = activeMessageBoardPosts(s.orm.WithContext(ctx), time.Now()).Where("id = ?", id).Take(&post).Error
	return post, persistenceError(err)
}

func migrateMessageBoard(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS message_board_posts (
 id INTEGER PRIMARY KEY CHECK(id BETWEEN 1 AND 4294967295),
 author_id INTEGER NOT NULL CHECK(author_id BETWEEN 0 AND 4294967295),
 author_name BLOB NOT NULL CHECK(length(author_name) BETWEEN 1 AND 14),
 title BLOB NOT NULL CHECK(length(title) BETWEEN 1 AND 30),
 content BLOB NOT NULL CHECK(length(content) BETWEEN 1 AND 2048),
 posted_at_millis INTEGER NOT NULL);
 CREATE INDEX IF NOT EXISTS message_board_posts_date ON message_board_posts(posted_at_millis DESC,id DESC);
 CREATE TABLE IF NOT EXISTS message_board_sequence(id INTEGER PRIMARY KEY CHECK(id=1),value INTEGER NOT NULL CHECK(value BETWEEN 0 AND 4294967295));
 INSERT OR IGNORE INTO message_board_sequence(id,value) SELECT 1,COALESCE(MAX(id),0) FROM message_board_posts;`)
	return err
}

func validBoardText(text []byte, limit int, multiline bool) bool {
	if len(text) == 0 || len(text) > limit || len(bytes.TrimSpace(text)) == 0 {
		return false
	}
	for _, b := range text {
		if b == 127 || b < 32 && !(multiline && (b == '\n' || b == '\r' || b == '\t')) {
			return false
		}
	}
	return true
}

// CreateMessageBoardPost authenticates the author and reserves their quota in
// the same transaction as insertion. Retries of the identical post are harmless.
func (s *Store) CreateMessageBoardPost(ctx context.Context, ref CharacterRef, title, content []byte, at time.Time) (post MessageBoardPost, err error) {
	if !validBoardText(title, messageBoardTitleLimit, false) || !validBoardText(content, messageBoardContentLimit, true) {
		return post, ErrMessageBoardText
	}
	err = s.transaction(ctx, func(tx *gorm.DB) error {
		var author characterRow
		if e := tx.Select("id", "name").Where("id = ? AND account_id = ?", ref.ID, ref.Account).Take(&author).Error; e != nil {
			return e
		}
		if len(author.Name) == 0 || len(author.Name) > 14 {
			return ErrMessageBoardText
		}
		e := activeMessageBoardPosts(tx, at).Where("author_id = ? AND title = ? AND content = ?", author.ID, title, content).Take(&post).Error
		if e == nil {
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var count int64
		if e = activeMessageBoardPosts(tx, at).Model(&MessageBoardPost{}).Where("author_id = ?", author.ID).Count(&count).Error; e != nil {
			return e
		}
		if count >= MessageBoardPostsPerAuthor {
			return ErrMessageBoardAuthorLimit
		}
		if e = activeMessageBoardPosts(tx, at).Model(&MessageBoardPost{}).Count(&count).Error; e != nil {
			return e
		}
		if count >= messageBoardMaxPosts {
			return ErrMessageBoardFull
		}
		// Never reuse a deleted ID: stale delete/read requests must not refer
		// to a different notice created later. Also respect administrator IDs.
		var last uint64
		if e = tx.Raw("SELECT MAX(value,COALESCE((SELECT MAX(id) FROM message_board_posts),0)) FROM message_board_sequence WHERE id=1").Scan(&last).Error; e != nil {
			return e
		}
		if last >= uint64(^uint32(0)) {
			return ErrMessageBoardFull
		}
		if e = tx.Exec("UPDATE message_board_sequence SET value=? WHERE id=1", last+1).Error; e != nil {
			return e
		}
		post = MessageBoardPost{AuthorID: author.ID, AuthorName: []byte(author.Name), Title: bytes.Clone(title), Content: bytes.Clone(content), PostedAtMillis: at.UTC().UnixMilli()}
		post.ID = uint32(last + 1)
		return tx.Create(&post).Error
	})
	return
}

func (s *Store) DeleteMessageBoardPost(ctx context.Context, ref CharacterRef, id uint32) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var author characterRow
		if e := tx.Select("id").Where("id = ? AND account_id = ?", ref.ID, ref.Account).Take(&author).Error; e != nil {
			return e
		}
		result := tx.Where("id = ? AND author_id = ?", id, author.ID).Delete(&MessageBoardPost{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}
