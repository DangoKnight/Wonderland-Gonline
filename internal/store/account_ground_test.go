package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"wonderland-gonline/internal/game"
)

func TestAccountGroundClaimsAtomicPermanentAndAccountScoped(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	const mapID, alternateMapID, scopeID = 10035, 10036, 10003
	items := map[uint16]game.ItemDefinition{40012: {ID: 40012, Type: 10}}
	for _, ref := range refs {
		if err := db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error {
			c.Map = mapID
			for i := range c.Bag {
				c.Bag[i] = game.Item{ID: 40012, Count: 50}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	claim := func(ref CharacterRef, mapID uint16) error {
		_, err := db.ClaimAccountGroundItem(ctx, ref, mapID, scopeID, 2, game.Item{ID: 40012, Count: 1}, 50, time.Now(), items)
		return err
	}
	if err := claim(refs[0], mapID); !errors.Is(err, game.ErrInventoryFull) {
		t.Fatal("full bag", err)
	}
	if slots, err := db.AccountGroundClaims(ctx, refs[0].ID, scopeID); err != nil || len(slots) != 0 {
		t.Fatal("failed pickup consumed node", slots, err)
	}
	for _, ref := range refs {
		if err := db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error { c.Bag = game.Inventory{}; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- claim(refs[0], mapID) }()
	}
	wg.Wait()
	close(errs)
	wins := 0
	for err := range errs {
		if err == nil {
			wins++
		} else if !errors.Is(err, ErrResourceUnavailable) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatal("concurrent replay", wins)
	}
	if err := claim(refs[1], mapID); err != nil {
		t.Fatal("another account blocked", err)
	}
	account := Account{ID: refs[0].Account}
	alt := game.Character{ID: account.CharacterID(2), Slot: 2, Name: "Alternate", Level: 1, HP: 1, MaxHP: 1, Map: alternateMapID}
	if err := db.CreateCharacter(ctx, account, alt); err != nil {
		t.Fatal(err)
	}
	if err := claim(CharacterRef{Account: account.ID, ID: alt.ID}, alternateMapID); !errors.Is(err, ErrResourceUnavailable) {
		t.Fatal("alternate character/map bypass", err)
	}
	var path string
	if err := db.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&path); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if slots, err := reopened.AccountGroundClaims(ctx, alt.ID, scopeID); err != nil || !slots[2] {
		t.Fatal("restart lost account claim", slots, err)
	}
}
