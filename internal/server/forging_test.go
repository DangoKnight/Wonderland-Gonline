package server

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func forgingFixture(t *testing.T) (*Server, *Session, *captureConn) {
	t.Helper()
	s, players, wires := compoundFixture(t)
	c := players[0]
	for _, id := range []uint16{100, 101, 102, 200} {
		s.Assets.Items[id] = game.ItemDefinition{ID: id, EquipSlot: 3, Status: [2]uint16{210, 214}, Values: [2]int32{110, 110}}
	}
	s.Assets.Items[30101] = game.ItemDefinition{ID: 30101, Type: 21}
	s.Assets.Forging = assets.Forging{Upgrades: map[uint16]assets.ForgeUpgrade{100: {Next: 101, Scrolls: 1}, 101: {Next: 102, Scrolls: 2}, 102: {}}, PointItems: map[uint16]bool{200: true}}
	next := c.character.Clone()
	next.Bag = game.Inventory{}
	next.Bag[4] = game.Item{ID: 100, Count: 1, Damage: 7, Metadata: [26]byte{9}}
	next.Bag[8] = game.Item{ID: 30101, Count: 1}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	return s, c, wires[0]
}
func TestStrongScrollForgePacketsAndPersistence(t *testing.T) {
	s, c, wire := forgingFixture(t)
	if err := s.dispatch(context.Background(), c, []byte{75, 3, 5}); err != nil {
		t.Fatal(err)
	}
	addition := append([]byte{23, 5, 5, 101, 0, 1, 7}, make([]byte, 26)...)
	want := [][]byte{{23, 9, 9, 1}, {23, 9, 5, 1}, addition, {75, 6, 6}}
	got := wire.packets(t)
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("packet %d: %v != %v", i, got[i], want[i])
		}
	}
	if c.character.Bag[4] != (game.Item{ID: 101, Count: 1, Damage: 7}) || !c.character.Bag[8].Empty() {
		t.Fatal("upgrade state")
	}
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || chars[0].Bag != c.character.Bag {
		t.Fatal("not persisted", err)
	}
	before := c.character.Bag
	if err := s.dispatch(context.Background(), c, []byte{75, 3, 5}); err != nil {
		t.Fatal(err)
	}
	got = wire.packets(t)
	if c.character.Bag != before || len(got) != 2 || !bytes.Equal(got[0], []byte{75, 6, 5}) {
		t.Fatal("replay consumed equipment", got)
	}
	// The next tier requires two scrolls, consumed in ascending slot order.
	next := c.character.Clone()
	next.Bag[8] = game.Item{ID: 30101, Count: 1}
	next.Bag[12] = game.Item{ID: 30101, Count: 3, Damage: 4, Metadata: [26]byte{6}}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(context.Background(), c, []byte{75, 3, 5}); err != nil {
		t.Fatal(err)
	}
	got = wire.packets(t)
	if len(got) != 5 || !bytes.Equal(got[0], []byte{23, 9, 9, 1}) || !bytes.Equal(got[1], []byte{23, 9, 13, 1}) || c.character.Bag[12].Count != 2 || c.character.Bag[12].Metadata[0] != 6 || c.character.Bag[4].ID != 102 {
		t.Fatal("split scroll costs", got)
	}
	before = c.character.Bag
	if err := s.dispatch(context.Background(), c, []byte{75, 3, 5}); err != nil {
		t.Fatal(err)
	}
	got = wire.packets(t)
	if c.character.Bag != before || got[0][2] != 8 {
		t.Fatal("terminal tier consumed scrolls")
	}
}
func pointForgeFixture(t *testing.T) (*Server, *Session, *captureConn) {
	t.Helper()
	s, c, wire := forgingFixture(t)
	next := c.character.Clone()
	next.Bag[4].ID = 200
	next.Bag[4].SetForge(2)
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.AdjustMallBalance(context.Background(), c.account.ID, false, 12, "test"); err != nil {
		t.Fatal(err)
	}
	return s, c, wire
}
func TestPointForgeWinLossAtomicBalances(t *testing.T) {
	s, c, wire := pointForgeFixture(t)
	before := c.character.Bag
	for _, success := range []bool{true, false} {
		if err := s.forgeItem(context.Background(), c, 5, func() (bool, error) { return success, nil }); err != nil {
			t.Fatal(err)
		}
		got := wire.packets(t)
		want := []byte{75, 6, 2}
		if success {
			want = []byte{75, 6, 1, 5, 3, 0}
		}
		if len(got) != 3 || !bytes.Equal(got[0], want) || got[1][1] != 3 || got[2][1] != 9 {
			t.Fatal("point forge receipt", got)
		}
	}
	balances, err := s.Store.MallBalances(context.Background(), c.account.ID)
	if err != nil || balances.Points != 6 || c.account.IM != 6 || c.character.Bag[4].Forge() != 3 || c.character.Bag[4].Damage != before[4].Damage || c.character.Bag[4].Metadata[0] != 9 {
		t.Fatal("point forging state", balances, err)
	}
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || chars[0].Bag != c.character.Bag {
		t.Fatal("point forge not persisted", err)
	}
}
func TestForgeFailuresDoNotSpend(t *testing.T) {
	s, c, wire := pointForgeFixture(t)
	before := c.character.Bag
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.forgeItem(canceled, c, 5, func() (bool, error) { return true, nil }); err == nil || c.character.Bag != before || wire.Len() != 0 {
		t.Fatal("failed save published success")
	}
	if err := s.forgeItem(context.Background(), c, 5, func() (bool, error) { return false, errors.New("entropy unavailable") }); err == nil || c.character.Bag != before || wire.Len() != 0 {
		t.Fatal("random failure spent points")
	}
	balances, err := s.Store.MallBalances(context.Background(), c.account.ID)
	if err != nil || balances.Points != 12 {
		t.Fatal(balances, err)
	}
	next := c.character.Clone()
	next.Bag[4].SetForge(200)
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.forgeItem(context.Background(), c, 5, func() (bool, error) { t.Fatal("max level rolled"); return true, nil }); err != nil {
		t.Fatal(err)
	}
	if got := wire.packets(t); len(got) != 2 || got[0][2] != 8 {
		t.Fatal(got)
	}
	next = c.character.Clone()
	next.Bag[4].SetForge(0)
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.AdjustMallBalance(context.Background(), c.account.ID, false, -11, "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.forgeItem(context.Background(), c, 5, func() (bool, error) { t.Fatal("insufficient balance rolled"); return true, nil }); err != nil {
		t.Fatal(err)
	}
	got := wire.packets(t)
	if len(got) != 4 || !bytes.Equal(got[0], []byte{75, 6, 4}) || c.character.Bag[4].Forge() != 0 {
		t.Fatal("insufficient points", got)
	}
}
func TestForgeInvalidDataAndOwnership(t *testing.T) {
	for _, gate := range []string{"unknown output", "wrong slot", "stack", "loading", "battle", "trade", "event", "storm", "beach", "mounted", "ineligible"} {
		t.Run(gate, func(t *testing.T) {
			s, c, wire := forgingFixture(t)
			switch gate {
			case "unknown output":
				delete(s.Assets.Items, 101)
			case "wrong slot":
				d := s.Assets.Items[101]
				d.EquipSlot = 4
				s.Assets.Items[101] = d
			case "stack":
				c.character.Bag[4].Count = 2
			case "loading":
				c.ready = false
			case "battle":
				c.battle = &battleRun{}
			case "trade":
				c.trade = &tradeSession{}
			case "event":
				c.event = &eventSession{}
			case "storm":
				c.storm = true
			case "beach":
				c.beach = &beachRun{}
			case "mounted":
				c.character.ActiveVehicle = 100
				c.character.VehicleSlot = 5
			case "ineligible":
				s.Assets.Forging = assets.Forging{}
			}
			before := c.character.Bag
			err := s.dispatch(context.Background(), c, []byte{75, 3, 5})
			if gate == "loading" && err == nil {
				t.Fatal("pre-ack mutation accepted")
			}
			if c.character.Bag != before {
				t.Fatal("invalid forge consumed items", err)
			}
			got := wire.packets(t)
			if gate == "battle" || gate == "loading" {
				if len(got) != 0 {
					t.Fatal(got)
				}
			} else if gate == "trade" {
				if len(got) != 1 || got[0][1] != 57 {
					t.Fatal(got)
				}
			} else if len(got) != 2 || got[0][2] != 8 {
				t.Fatal("selection not released", got)
			}
		})
	}
	s, c, wire := forgingFixture(t)
	for _, p := range [][]byte{{75, 3}, {75, 3, 5, 0}} {
		if err := s.dispatch(context.Background(), c, p); err == nil || wire.Len() != 0 {
			t.Fatal("malformed forge accepted")
		}
	}
	for _, slot := range []byte{0, 51, 1} {
		if err := s.dispatch(context.Background(), c, []byte{75, 3, slot}); err != nil {
			t.Fatal(err)
		}
		if got := wire.packets(t); len(got) != 2 || got[0][2] != 8 {
			t.Fatal(got)
		}
	}
}

func TestForgeFailedScrollSaveAndReceipt(t *testing.T) {
	s, c, wire := forgingFixture(t)
	before := c.character.Bag
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.dispatch(canceled, c, []byte{75, 3, 5}); err == nil || c.character.Bag != before || wire.Len() != 0 {
		t.Fatal("scroll save failed to roll back")
	}
	c.conn = &failedWorldConn{}
	if err := s.dispatch(context.Background(), c, []byte{75, 3, 5}); err == nil {
		t.Fatal("receipt error hidden")
	}
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || chars[0].Bag != c.character.Bag || chars[0].Bag[4].ID != 101 || !chars[0].Bag[8].Empty() {
		t.Fatal("committed upgrade lost after receipt failure", err)
	}
}

func TestPointForgeConcurrentAttemptsCannotOverspend(t *testing.T) {
	s, c, wire := pointForgeFixture(t)
	if _, err := s.Store.AdjustMallBalance(context.Background(), c.account.ID, false, -6, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.AdjustMallBalance(context.Background(), c.account.ID, true, 17, "test"); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	failures := make(chan error, 3)
	for range 3 {
		workers.Add(1)
		go func() { defer workers.Done(); failures <- s.dispatch(context.Background(), c, []byte{75, 3, 5}) }()
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	successes, attempts, rejected := 0, 0, 0
	for _, packet := range wire.packets(t) {
		if len(packet) < 3 || packet[0] != 75 || packet[1] != 6 {
			continue
		}
		switch packet[2] {
		case 1:
			successes++
			attempts++
		case 2:
			attempts++
		case 4:
			rejected++
		default:
			t.Fatal("unexpected result", packet)
		}
	}
	balances, err := s.Store.MallBalances(context.Background(), c.account.ID)
	if err != nil || balances.Points != 0 || balances.Bonus != 17 || attempts != 2 || rejected != 1 || c.character.Bag[4].Forge() != byte(2+successes) {
		t.Fatal("concurrent overspend", balances, attempts, rejected, successes, err)
	}
}

func TestForgingUsesSQLFamilies(t *testing.T) {
	if os.Getenv("WONDERLAND_TEST_ASSETS_DB") == "" {
		t.Skip("set WONDERLAND_TEST_ASSETS_DB")
	}
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, c, wire := forgingFixture(t)
	s.Assets = catalog
	next := c.character.Clone()
	next.Bag[4] = game.Item{ID: 11101, Count: 1, Damage: 9}
	next.Bag[8] = game.Item{ID: 30101, Count: 1}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(context.Background(), c, []byte{75, 3, 5}); err != nil {
		t.Fatal(err)
	}
	got := wire.packets(t)
	if len(got) != 4 || !bytes.Equal(got[3], []byte{75, 6, 6}) || c.character.Bag[4].ID != 11103 || c.character.Bag[4].Damage != 9 {
		t.Fatal("SQL upgrade family missing", got)
	}
}
