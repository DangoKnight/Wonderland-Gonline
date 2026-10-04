package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestTextMailPersistsAndScopesDelivery(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	body := []byte{0xa4, 0xa4, 0xa4, 0xe5, '\n', 'H', 'i'}
	sent := time.Date(2026, 10, 2, 10, 20, 30, 123000000, time.UTC)
	first, err := db.SendTextMail(ctx, refs[0], refs[1].ID, 3, body, sent)
	if err != nil || first.SenderName != "Alice" || first.SentAtMillis != sent.UnixMilli() {
		t.Fatal(first, err)
	}
	body[0] = 0
	second, err := db.SendTextMail(ctx, refs[0], refs[1].ID, 0, []byte("second"), sent)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.MarkTextMailDelivered(ctx, refs[0].ID, first.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("another recipient marked mail", err)
	}
	var sequence int
	var name, path string
	if err := db.db.QueryRow("PRAGMA database_list").Scan(&sequence, &name, &path); err != nil {
		t.Fatal(err)
	}
	db.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	pending, err := reopened.PendingTextMail(ctx, refs[1].ID, 1)
	if err != nil || len(pending) != 1 || pending[0].ID != first.ID || !bytes.Equal(pending[0].Content, []byte{0xa4, 0xa4, 0xa4, 0xe5, '\n', 'H', 'i'}) {
		t.Fatal(pending, err)
	}
	if err := reopened.MarkTextMailDelivered(ctx, refs[1].ID, first.ID); err != nil {
		t.Fatal(err)
	}
	pending, err = reopened.PendingTextMail(ctx, refs[1].ID, 10)
	if err != nil || len(pending) != 1 || pending[0].ID != second.ID {
		t.Fatal(pending, err)
	}
	if err := reopened.DeleteAccount(ctx, refs[0].Account); err != nil {
		t.Fatal(err)
	}
	pending, err = reopened.PendingTextMail(ctx, refs[1].ID, 10)
	if err != nil || len(pending) != 0 {
		t.Fatal("deleted sender left mail tied to reusable ID", pending, err)
	}
}

func TestTextMailRejectsInvalidIdentityContentAndCommit(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	wrong := refs[0]
	wrong.Account = refs[1].Account
	for _, sender := range []CharacterRef{wrong, {ID: 0}} {
		if _, err := db.SendTextMail(ctx, sender, refs[1].ID, 0, []byte("hello"), time.Now()); !errors.Is(err, sql.ErrNoRows) {
			t.Fatal(err)
		}
	}
	if _, err := db.SendTextMail(ctx, refs[0], 999999, 0, []byte("hello"), time.Now()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	for _, body := range [][]byte{nil, []byte(" \n"), bytes.Repeat([]byte{'a'}, 256), []byte("a\x00b"), []byte("a\x01b")} {
		if _, err := db.SendTextMail(ctx, refs[0], refs[1].ID, 0, body, time.Now()); !errors.Is(err, ErrMailContent) {
			t.Fatal(err)
		}
	}
	if _, err := db.db.Exec("CREATE TRIGGER reject_mail BEFORE INSERT ON text_mail BEGIN SELECT RAISE(ABORT,'reject'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SendTextMail(ctx, refs[0], refs[1].ID, 0, []byte("hello"), time.Now()); err == nil {
		t.Fatal("failed commit accepted")
	}
	pending, err := db.PendingTextMail(ctx, refs[1].ID, 10)
	if err != nil || len(pending) != 0 {
		t.Fatal(pending, err)
	}
}

func TestTextMailVersionFiveUpgradePreservesPlayerState(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	if err := db.AddFriend(ctx, refs[0].ID, refs[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec("UPDATE accounts SET im_bonus=123; DROP TABLE text_mail; PRAGMA user_version=5;"); err != nil {
		t.Fatal(err)
	}
	var sequence int
	var name, path string
	if err := db.db.QueryRow("PRAGMA database_list").Scan(&sequence, &name, &path); err != nil {
		t.Fatal(err)
	}
	db.Close()
	migrated, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	a, err := migrated.Authenticate(ctx, "Alice", "password")
	if err != nil || a.IMBonus != 123 {
		t.Fatal(a, err)
	}
	friends, err := migrated.Friends(ctx, refs[0].ID)
	if err != nil || len(friends) != 1 {
		t.Fatal(friends, err)
	}
	var version int
	if err := migrated.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != schemaVersion {
		t.Fatal(version, err)
	}
	if _, err := migrated.SendTextMail(ctx, refs[0], refs[1].ID, 0, []byte("after upgrade"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := migrated.DeleteAccount(ctx, refs[1].Account); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := migrated.orm.Model(&TextMail{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("recipient cleanup", count, err)
	}
}

func TestTextMailWhitespaceValidationPreservesNativeBytes(t *testing.T) {
	// Native text is not UTF-8. A byte pair that UTF-8 interprets as whitespace
	// must not be discarded or rejected by Unicode whitespace classification.
	if err := ValidateMailContent([]byte{0xc2, 0xa0}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMailContent([]byte(" \t\r\n")); !errors.Is(err, ErrMailContent) {
		t.Fatal(err)
	}
}
