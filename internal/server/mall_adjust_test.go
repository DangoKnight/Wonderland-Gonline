package server

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/store"
)

func expectMallAdjustment(t *testing.T, wire *captureConn, points, bonus byte) {
	t.Helper()
	packets := wire.packets(t)
	// Independent native AC75:3 and AC75:9 layout: balance, spent, item, count.
	want := [][]byte{{75, 3, points, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, {75, 9, bonus, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}
	if len(packets) != 2 {
		t.Fatal(packets)
	}
	for i := range want {
		if !bytes.Equal(packets[i], want[i]) {
			t.Fatal(packets[i], want[i])
		}
	}
}

func TestGMMallPointCommandsAndTargeting(t *testing.T) {
	s, players, wires := chatFixture(t)
	gm, target := players[0], players[1]
	ctx := context.Background()
	say(t, s, gm, ":im 10 Bobby")
	if balance, err := s.Store.MallBalances(ctx, target.account.ID); err != nil || balance.Points != 0 {
		t.Fatal("non-GM granted currency", balance, err)
	}
	s.SetGMLevel(gm.account.ID, 1)
	for _, command := range []string{":im 10 Bobby", "/points_im 10 Bobby", "/mallpoints 10 Bobby"} {
		say(t, s, gm, command)
	}
	got := wires[1].packets(t)
	if len(got) != 6 {
		t.Fatal(got)
	}
	if balance, err := s.Store.MallBalances(ctx, target.account.ID); err != nil || balance.Points != 30 || target.account.IM != 30 {
		t.Fatal(balance, err)
	}
	for _, command := range []string{":im 10 nobody", ":im bad", ":im 2147483648", ":im 0", ":im"} {
		say(t, s, gm, command)
	}
	if wires[0].Len() != 0 || wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("invalid target granted to self or leaked receipts")
	}
	say(t, s, gm, ":im 15")
	expectMallAdjustment(t, wires[0], 15, 0)
	say(t, s, gm, ":mallpoints -100 Bobby")
	expectMallAdjustment(t, wires[1], 0, 0)
	if target.account.IM != 0 {
		t.Fatal("deduction did not floor at zero")
	}
}

func TestAdminMallAdjustmentLiveLoadingAndFailure(t *testing.T) {
	s, players, wires := chatFixture(t)
	c := players[0]
	ctx := context.Background()
	if _, err := s.AdjustMallBalance(ctx, c.account.ID, true, 17); err != nil {
		t.Fatal(err)
	}
	expectMallAdjustment(t, wires[0], 0, 17)
	if wires[1].Len() != 0 || wires[2].Len() != 0 || c.account.IMBonus != 17 {
		t.Fatal("balance leaked or cache stale")
	}
	// Defer an adjustment while this connection is loading its next map.
	s.worldMu.Lock()
	c.ready = false
	delete(s.world, c.info.ID)
	s.worldMu.Unlock()
	if _, err := s.AdjustMallBalance(ctx, c.account.ID, false, 23); err != nil {
		t.Fatal(err)
	}
	if wires[0].Len() != 0 || c.account.IM != 0 {
		t.Fatal("interleaved loading snapshot")
	}
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	packets := wires[0].packets(t)
	if len(packets) < 2 || !bytes.Equal(packets[0], []byte{75, 3, 23, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) || !bytes.Equal(packets[1], []byte{75, 9, 17, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) || c.account.IM != 23 {
		t.Fatal("loading grant not synchronized", packets)
	}
	for _, w := range wires {
		w.Reset()
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.AdjustMallBalance(canceled, c.account.ID, false, 1); err == nil || c.account.IM != 23 || wires[0].Len() != 0 {
		t.Fatal("failed grant published", err)
	}
	failed := &failedWorldConn{}
	c.conn = failed
	balance, err := s.AdjustMallBalance(ctx, c.account.ID, false, 1)
	if err != nil || balance.Points != 24 || !failed.closed {
		t.Fatal("committed grant reported failure after receipt error", balance, err)
	}
	if current, err := s.Store.MallBalances(ctx, c.account.ID); err != nil || current != balance {
		t.Fatal(current, err)
	}
}

func TestAdminMallAdjustmentOrdersConcurrentRequests(t *testing.T) {
	s, players, wires := chatFixture(t)
	c := players[0]
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.AdjustMallBalance(context.Background(), c.account.ID, false, 1)
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
	packets := wires[0].packets(t)
	if len(packets) != 20 || c.account.IM != 10 {
		t.Fatal(packets)
	}
	for i := 0; i < 10; i++ {
		if packets[i*2][2] != byte(i+1) {
			t.Fatal("balance receipts reordered", packets)
		}
	}
}

func TestMallAdjustmentPurchaseUsesSavedGrant(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	// Ensure publication so administrator receipts share the purchase order.
	s.world[c.info.ID] = c
	if _, err := s.AdjustMallBalance(ctx, c.account.ID, false, -100); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	if err := s.worldCommand(ctx, c, mallCart(1, mallCartRow{item: 32176, category: 4, quantity: 1, order: 12})); err != nil {
		t.Fatal(err)
	}
	if !c.character.Bag[0].Empty() || c.account.IM != 0 {
		t.Fatal("purchase used stale pre-adjustment funds")
	}
	wire.Reset()
	if _, err := s.AdjustMallBalance(ctx, c.account.ID, false, 10); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	if err := s.worldCommand(ctx, c, mallCart(1, mallCartRow{item: 32176, category: 4, quantity: 1, order: 12})); err != nil || c.character.Bag[0] != (game.Item{ID: 32176, Count: 5}) || c.account.IM != 0 {
		t.Fatal("grant not spendable", err)
	}
	if balance, err := s.Store.MallBalances(ctx, c.account.ID); err != nil || balance != (store.MallBalances{Points: 0, Bonus: 30}) {
		t.Fatal(balance, err)
	}
}
