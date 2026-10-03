package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"wonderland-go/internal/game"
)

func TestFriendsPersistentSymmetricAndDeletionCleanup(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db")
	db, refs := pairFixture(t)
	// A separate reopened DB also proves friendship schema/state persistence.
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { reopened.Close() }()
	ids := []uint32{}
	for i, ref := range refs {
		a, err := reopened.Register(ctx, []string{"alice", "bobby"}[i], "password", "")
		if err != nil {
			t.Fatal(err)
		}
		source, err := db.Characters(ctx, ref.Account)
		if err != nil {
			t.Fatal(err)
		}
		c := source[0]
		c.ID = a.CharacterID(1)
		c.Name = []string{"Alice", "Bobby"}[i]
		if err := reopened.CreateCharacter(ctx, a, c); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}
	if err := reopened.AddFriend(ctx, ids[1], ids[0]); err != nil {
		t.Fatal(err)
	}
	if err := reopened.AddFriend(ctx, ids[0], ids[1]); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		f, err := reopened.Friends(ctx, id)
		if err != nil || len(f) != 1 || f[0].ID != ids[1-i] {
			t.Fatal("asymmetric friendship", f, err)
		}
	}
	reopened.Close()
	reopened, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]uint32{{ids[0], ids[0]}, {0, ids[0]}, {ids[0], 999}} {
		if err := reopened.AddFriend(ctx, pair[0], pair[1]); err == nil {
			t.Fatal("invalid relationship added")
		}
	}
	if err := reopened.DeleteCharacter(ctx, Account{ID: 2}, 1, ""); err != nil {
		t.Fatal(err)
	}
	if f, err := reopened.Friends(ctx, ids[0]); err != nil || len(f) != 0 {
		t.Fatal("deleted friend remained", f, err)
	}
}

func TestFriendsConcurrentAddAndFailedDelete(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	a, b := refs[0].ID, refs[1].ID
	var wg sync.WaitGroup
	failures := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); failures <- db.AddFriend(ctx, a, b) }()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	f, err := db.Friends(ctx, a)
	if err != nil || len(f) != 1 {
		t.Fatal("duplicate friends", f, err)
	}
	if _, err := db.db.Exec("CREATE TRIGGER reject_delete BEFORE DELETE ON friendships BEGIN SELECT RAISE(ABORT,'reject'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RemoveFriend(ctx, a, b); err == nil {
		t.Fatal("failed remove ignored")
	}
	if f, err := db.Friends(ctx, b); err != nil || len(f) != 1 {
		t.Fatal("failed remove lost relationship", f, err)
	}
}

func TestFriendLimitAppliesToBothSides(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	// Synthetic rows isolate list capacity without spending time hashing credentials.
	for i := 0; i < FriendLimit; i++ {
		id := uint32(200000 + i)
		c := game.Character{ID: id, Slot: 1, Name: fmt.Sprintf("Friend%d", i), Level: 1}
		raw, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		// Extra accounts avoid violating the two-slot uniqueness rule.
		result, err := db.db.Exec("INSERT INTO accounts(username,password_hash,created_at) VALUES(?, 'unused', '')", id)
		if err != nil {
			t.Fatal(err)
		}
		account, _ := result.LastInsertId()
		if _, err := db.db.Exec("INSERT INTO characters(id,account_id,slot,name,state) VALUES(?,?,1,?,?)", id, account, c.Name, raw); err != nil {
			t.Fatal(err)
		}
		if err := db.AddFriend(ctx, refs[0].ID, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AddFriend(ctx, refs[1].ID, refs[0].ID); !errors.Is(err, ErrFriendLimit) {
		t.Fatal("recipient cap bypassed", err)
	}
	var count int
	db.db.QueryRow("SELECT count(*) FROM friendships WHERE character1=? OR character2=?", refs[1].ID, refs[1].ID).Scan(&count)
	if count != 0 {
		t.Fatal("full recipient partially added")
	}
}
