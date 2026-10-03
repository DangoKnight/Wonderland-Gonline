package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"wonderland-go/internal/game"
)

func mallStoreFixture(t *testing.T) (*Store, Account, CharacterRef, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mall.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	a, err := s.Register(context.Background(), "shopper", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	c := game.Character{ID: a.CharacterID(1), Slot: 1, Name: "Shopper", Level: 1, HP: 10, MaxHP: 10}
	if err := s.CreateCharacterWithCode(context.Background(), a, c, "delete-me"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE accounts SET im=100,im_bonus=30 WHERE id=?", a.ID); err != nil {
		t.Fatal(err)
	}
	return s, a, CharacterRef{Account: a.ID, ID: c.ID}, path
}
func mallDelivery(c *game.Character) error { return c.Bag.Add(game.Item{ID: 32176}, 5, 50) }

func TestMallPurchaseCurrenciesAndRestart(t *testing.T) {
	ctx := context.Background()
	s, a, ref, path := mallStoreFixture(t)
	for _, bonus := range []bool{false, true} {
		c, balance, err := s.PurchaseMall(ctx, ref, bonus, 10, mallDelivery)
		if err != nil || c.Bag[0].Count != byte(5+5*map[bool]int{false: 0, true: 1}[bonus]) {
			t.Fatal(c.Bag, balance, err)
		}
	}
	s.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	balance, err := reopened.MallBalances(ctx, a.ID)
	if err != nil || balance.Points != 90 || balance.Bonus != 20 {
		t.Fatal(balance, err)
	}
	chars, err := reopened.Characters(ctx, a.ID)
	if err != nil || len(chars) != 1 || chars[0].Bag[0].Count != 10 {
		t.Fatal(chars, err)
	}
	account, err := reopened.Authenticate(ctx, a.Username, "password")
	if err != nil || account.IM != 90 || account.IMBonus != 20 {
		t.Fatal(account, err)
	}
}
func TestMallPurchaseRollbackAndOwnership(t *testing.T) {
	for _, failure := range []string{"funds", "bag", "identity", "account-write", "character-write", "owner", "banned"} {
		t.Run(failure, func(t *testing.T) {
			s, a, ref, _ := mallStoreFixture(t)
			ctx := context.Background()
			cost := int64(10)
			deliver := mallDelivery
			switch failure {
			case "funds":
				cost = 101
			case "bag":
				deliver = func(c *game.Character) error { c.Bag[0] = game.Item{ID: 99, Count: 1}; return game.ErrInventoryFull }
			case "identity":
				deliver = func(c *game.Character) error { c.Name = "Changed"; return nil }
			case "account-write":
				if _, err := s.db.Exec("CREATE TRIGGER fail_mall BEFORE UPDATE OF im ON accounts BEGIN SELECT RAISE(ABORT,'test'); END;"); err != nil {
					t.Fatal(err)
				}
			case "character-write":
				if _, err := s.db.Exec("CREATE TRIGGER fail_mall BEFORE UPDATE ON characters BEGIN SELECT RAISE(ABORT,'test'); END;"); err != nil {
					t.Fatal(err)
				}
			case "owner":
				ref.ID++
			case "banned":
				if err := s.SetBanned(ctx, a.ID, true); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := s.PurchaseMall(ctx, ref, false, cost, deliver); err == nil {
				t.Fatal("invalid purchase succeeded")
			}
			balance, err := s.MallBalances(ctx, a.ID)
			if err != nil || balance.Points != 100 || balance.Bonus != 30 {
				t.Fatal("currency changed", balance, err)
			}
			chars, err := s.Characters(ctx, a.ID)
			if err != nil || len(chars) != 1 || chars[0].Name != "Shopper" || chars[0].Bag[0].ID != 0 {
				t.Fatal("delivery escaped rollback", chars, err)
			}
		})
	}
}
func TestMallConcurrentPurchasesCannotOverspend(t *testing.T) {
	s, a, ref, _ := mallStoreFixture(t)
	ctx := context.Background()
	results := make(chan error, 3)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := s.PurchaseMall(ctx, ref, false, 40, mallDelivery)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrMallFunds) {
			t.Fatal(err)
		}
	}
	balance, err := s.MallBalances(ctx, a.ID)
	if success != 2 || err != nil || balance.Points != 20 {
		t.Fatal(success, balance, err)
	}
	chars, err := s.Characters(ctx, a.ID)
	if err != nil || chars[0].Bag[0].Count != 10 {
		t.Fatal(chars, err)
	}
}
func TestMallVersionFourMigration(t *testing.T) {
	s, a, _, path := mallStoreFixture(t)
	if _, err := s.db.Exec("ALTER TABLE accounts DROP COLUMN im_bonus; PRAGMA user_version=4;"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	migrated, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	got, err := migrated.Authenticate(context.Background(), a.Username, "password")
	if err != nil || got.IM != 100 || got.IMBonus != 0 {
		t.Fatal(got, err)
	}
	has, err := migrated.HasDeletionCode(context.Background(), a.ID)
	if err != nil || !has {
		t.Fatal("credential changed", err)
	}
	var version int
	if err := migrated.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != schemaVersion {
		t.Fatal(version, err)
	}
	if _, err := migrated.db.Exec("UPDATE accounts SET im_bonus=-1"); err == nil {
		t.Fatal("negative bonus points allowed")
	}
	if _, err := migrated.MallBalances(context.Background(), 999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
}
