package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestMallAdjustmentAuditBoundsAndRestart(t *testing.T) {
	s, a, _, path := mallStoreFixture(t)
	ctx := context.Background()
	balance, err := s.AdjustMallBalance(ctx, a.ID, false, 25, "administrator")
	if err != nil || balance != (MallBalances{Points: 125, Bonus: 30}) {
		t.Fatal(balance, err)
	}
	balance, err = s.AdjustMallBalance(ctx, a.ID, true, -100, "gm:1/character:10001")
	if err != nil || balance != (MallBalances{Points: 125, Bonus: 0}) {
		t.Fatal(balance, err)
	}
	var subject string
	if err := s.db.QueryRow("SELECT subject FROM audit WHERE action='mall_adjust' ORDER BY id DESC LIMIT 1").Scan(&subject); err != nil {
		t.Fatal(err)
	}
	var audit struct {
		Account                           uint32
		Actor, Currency                   string
		Requested, Applied, Before, After int64
	}
	if err := json.Unmarshal([]byte(subject), &audit); err != nil || audit.Account != a.ID || audit.Actor != "gm:1/character:10001" || audit.Currency != "bonus" || audit.Requested != -100 || audit.Applied != -30 || audit.Before != 30 || audit.After != 0 {
		t.Fatal(subject, err)
	}
	for _, delta := range []int64{0, math.MinInt64, math.MaxInt64, math.MinInt32 - 1, game.MaxMallPoints} {
		if _, err := s.AdjustMallBalance(ctx, a.ID, false, delta, "administrator"); !errors.Is(err, ErrMallAdjustment) {
			t.Fatal(delta, err)
		}
	}
	if _, err := s.AdjustMallBalance(ctx, a.ID, false, 1, ""); !errors.Is(err, ErrMallAdjustment) {
		t.Fatal(err)
	}
	if _, err := s.AdjustMallBalance(ctx, 999999, false, 1, "administrator"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if current, err := s.MallBalances(ctx, a.ID); err != nil || current != balance {
		t.Fatal(current, err)
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM audit WHERE action='mall_adjust'").Scan(&count); err != nil || count != 2 {
		t.Fatal(count, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if current, err := reopened.MallBalances(ctx, a.ID); err != nil || current != balance {
		t.Fatal(current, err)
	}
}

func TestMallAdjustmentAuditFailureRollsBack(t *testing.T) {
	s, a, _, _ := mallStoreFixture(t)
	ctx := context.Background()
	if _, err := s.db.Exec("CREATE TRIGGER fail_audit BEFORE INSERT ON audit BEGIN SELECT RAISE(ABORT,'audit unavailable'); END"); err != nil {
		t.Fatal(err)
	}
	for _, bonus := range []bool{false, true} {
		if _, err := s.AdjustMallBalance(ctx, a.ID, bonus, 20, "administrator"); err == nil {
			t.Fatal("audit failure ignored")
		}
	}
	if balance, err := s.MallBalances(ctx, a.ID); err != nil || balance != (MallBalances{Points: 100, Bonus: 30}) {
		t.Fatal(balance, err)
	}
}

func TestMallAdjustmentConcurrentPurchases(t *testing.T) {
	s, a, ref, _ := mallStoreFixture(t)
	ctx := context.Background()
	const operations = 20
	var wg sync.WaitGroup
	errs := make(chan error, operations*2)
	for i := 0; i < operations; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := s.AdjustMallBalance(ctx, a.ID, false, 1, "administrator")
			if err != nil {
				errs <- err
			}
		}()
		go func() {
			defer wg.Done()
			_, _, err := s.PurchaseMall(ctx, ref, false, 1, func(c *game.Character) error { return c.Bag.Add(game.Item{ID: 32176}, 1, 50) })
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if balance, err := s.MallBalances(ctx, a.ID); err != nil || balance.Points != 100 || balance.Bonus != 30 {
		t.Fatal(balance, err)
	}
	chars, err := s.Characters(ctx, a.ID)
	if err != nil || chars[0].Bag[0].Count != operations {
		t.Fatal(chars, err)
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM audit WHERE action='mall_adjust'").Scan(&count); err != nil || count != operations {
		t.Fatal(count, err)
	}
}

func TestMallAdjustmentSignedRangeAndCorruptBalances(t *testing.T) {
	s, a, _, _ := mallStoreFixture(t)
	ctx := context.Background()
	if balance, err := s.AdjustMallBalance(ctx, a.ID, false, game.MaxMallPoints-100, "administrator"); err != nil || balance.Points != game.MaxMallPoints {
		t.Fatal(balance, err)
	}
	if _, err := s.AdjustMallBalance(ctx, a.ID, false, 1, "administrator"); !errors.Is(err, ErrMallAdjustment) {
		t.Fatal("overflow accepted", err)
	}
	if balance, err := s.AdjustMallBalance(ctx, a.ID, false, math.MinInt32, "administrator"); err != nil || balance.Points != 0 {
		t.Fatal(balance, err)
	}
	if _, err := s.db.Exec("UPDATE accounts SET im=? WHERE id=?", int64(game.MaxMallPoints)+1, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdjustMallBalance(ctx, a.ID, true, 1, "administrator"); !errors.Is(err, ErrMallAdjustment) {
		t.Fatal("corrupt saved balance accepted", err)
	}
}
