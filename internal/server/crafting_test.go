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
	"wonderland-gonline/internal/store"
)

func TestAlchemyPacketsPersistenceAndReplay(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	// AC40 echoes subcommands while the same rank/base engine supplies results.
	s.Assets.AlchemyRecipes = append(s.Assets.AlchemyRecipes, assets.AlchemyRecipe{Input1: 200, Input2: 100, Output: 200})
	if err := s.dispatch(context.Background(), c, []byte{40, 7, 9, 5}); err != nil {
		t.Fatal(err)
	}
	addition := append([]byte{23, 5, 1, 44, 1, 1, 0}, make([]byte, 26)...)
	want := [][]byte{{23, 9, 9, 1}, {23, 9, 5, 1}, addition, {40, 7, 1, 44, 1}}
	got := wires[0].packets(t)
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("packet %d: %v != %v", i, got[i], want[i])
		}
	}
	if wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("private inventory receipts leaked to peers")
	}
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || chars[0].Bag != c.character.Bag || chars[0].Bag[0] != (game.Item{ID: 300, Count: 1}) || !chars[0].Bag[4].Empty() || !chars[0].Bag[8].Empty() {
		t.Fatal("alchemy not persisted", err)
	}
	before := c.character.Bag
	if err := s.dispatch(context.Background(), c, []byte{40, 7, 9, 5}); err != nil {
		t.Fatal(err)
	}
	got = wires[0].packets(t)
	if c.character.Bag != before || len(got) != 1 || !bytes.Equal(got[0], []byte{40, 7, 0, 0, 0}) {
		t.Fatal("replay consumed state", got)
	}
}

func TestAlchemyMalformedAndUnavailableInputs(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	before := c.character.Bag
	for _, request := range [][]byte{{40}, {40, 1}, {40, 1, 5}, {40, 1, 5, 9, 0}} {
		if err := s.dispatch(context.Background(), c, request); err == nil {
			t.Fatal("malformed request accepted", request)
		}
		if c.character.Bag != before || wires[0].Len() != 0 {
			t.Fatal("malformed request changed state")
		}
	}
	for _, request := range [][]byte{{40, 1, 0, 9}, {40, 1, 51, 9}, {40, 1, 5, 5}, {40, 1, 1, 9}} {
		if err := s.dispatch(context.Background(), c, request); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		if c.character.Bag != before || len(got) != 1 || !bytes.Equal(got[0], []byte{40, 1, 0, 0, 0}) {
			t.Fatal("invalid ingredients accepted", request, got)
		}
	}
	// Unknown definitions cannot be consumed; obsolete recipe rows are ignored.
	delete(s.Assets.Items, 100)
	s.alchemyIndex = nil
	if err := s.dispatch(context.Background(), c, []byte{40, 1, 5, 9}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if c.character.Bag != before || len(got) != 1 || !bytes.Equal(got[0], []byte{40, 1, 0, 0, 0}) {
		t.Fatal("unknown material consumed", got)
	}
}

func TestAlchemyFullBagStacksAndMetadata(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	next := c.character.Clone()
	for i := range next.Bag {
		next.Bag[i] = game.Item{ID: 100, Count: 2, Damage: 7, Metadata: [26]byte{9}}
	}
	next.Bag[8].ID = 200
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before := c.character.Bag
	if err := s.dispatch(ctx, c, []byte{40, 1, 5, 9}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag != before || !bytes.Equal(wires[0].packets(t)[0], []byte{40, 1, 0, 0, 0}) {
		t.Fatal("full bag lost ingredients")
	}
	// A fresh output can merge into a compatible stack even with no free slots.
	next = c.character.Clone()
	next.Bag[1] = game.Item{ID: 300, Count: 2}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before = c.character.Bag
	if err := s.dispatch(ctx, c, []byte{40, 1, 5, 9}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) != 4 || got[2][2] != 2 || c.character.Bag[1].Count != 3 || c.character.Bag[4].Count != 1 || c.character.Bag[4].Damage != 7 || c.character.Bag[4].Metadata != before[4].Metadata {
		t.Fatal("stack or ingredient metadata corrupted", got)
	}
	// A forged/damaged result stack must not receive fresh synthesized items.
	next = c.character.Clone()
	next.Bag[1] = game.Item{ID: 300, Count: 2, Damage: 7, Metadata: [26]byte{9}}
	next.Bag[4].Count = 2
	next.Bag[8].Count = 2
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before = c.character.Bag
	if err := s.dispatch(ctx, c, []byte{40, 1, 5, 9}); err != nil {
		t.Fatal(err)
	}
	got = wires[0].packets(t)
	if c.character.Bag != before || len(got) != 1 || got[0][2] != 0 {
		t.Fatal("incompatible result stack merged")
	}
	// Consuming the last ingredient frees space before the result grant.
	next = c.character.Clone()
	next.Bag[8].Count = 1
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx, c, []byte{40, 1, 5, 9}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[8] != (game.Item{ID: 300, Count: 1}) {
		t.Fatal("freed ingredient slot unused")
	}
}

func TestAlchemyFailedSaveAndReceipt(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	before := c.character.Bag
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.dispatch(canceled, c, []byte{40, 1, 5, 9}); err == nil {
		t.Fatal("failed save accepted")
	}
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || c.character.Bag != before || chars[0].Bag != before || wires[0].Len() != 0 {
		t.Fatal("failed save published or persisted success", err)
	}
	c.conn = &failedWorldConn{}
	if err := s.dispatch(context.Background(), c, []byte{40, 1, 5, 9}); err == nil {
		t.Fatal("receipt error hidden")
	}
	chars, err = s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || chars[0].Bag != c.character.Bag || chars[0].Bag[0].ID != 300 {
		t.Fatal("successful commit lost after receipt failure", err)
	}
}

func TestAlchemyOwnershipGates(t *testing.T) {
	for _, gate := range []string{"loading", "battle", "event", "storm", "beach", "trade", "mounted", "minigame"} {
		t.Run(gate, func(t *testing.T) {
			s, players, wires := compoundFixture(t)
			c := players[0]
			before := c.character.Bag
			switch gate {
			case "loading":
				c.ready = false
			case "battle":
				c.battle = &battleRun{}
			case "event":
				c.event = &eventSession{}
			case "storm":
				c.storm = true
			case "beach":
				c.beach = &beachRun{}
			case "trade":
				c.trade = &tradeSession{}
			case "mounted":
				c.character.ActiveVehicle = 100
				c.character.VehicleSlot = 5
			case "minigame":
				c.event = &eventSession{onMinigame: func(byte) error { return nil }}
			}
			err := s.dispatch(context.Background(), c, []byte{40, 1, 5, 9})
			if gate == "loading" && err == nil {
				t.Fatal("loading mutation accepted")
			}
			if c.character.Bag != before || wires[1].Len() != 0 || wires[2].Len() != 0 {
				t.Fatal("reserved state changed", err)
			}
			got := wires[0].packets(t)
			if gate == "trade" {
				if len(got) != 1 || got[0][0] != 23 || got[0][1] != 57 {
					t.Fatal("trade warning missing", got)
				}
			} else if gate != "mounted" && len(got) != 0 {
				t.Fatal("gated alchemy emitted receipt", got)
			}
		})
	}
}

func TestAlchemyConcurrentReplay(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	var workers sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		workers.Add(1)
		go func() { defer workers.Done(); errors <- s.dispatch(context.Background(), c, []byte{40, 1, 5, 9}) }()
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	got := wires[0].packets(t)
	if len(got) != 5 || !bytes.Equal(got[3], []byte{40, 1, 1, 44, 1}) || !bytes.Equal(got[4], []byte{40, 1, 0, 0, 0}) || c.character.Bag[0] != (game.Item{ID: 300, Count: 1}) {
		t.Fatal("concurrent replay duplicated output", got)
	}
}

func TestAlchemyWithSQLAssets(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, players, wires := compoundFixture(t)
	s.Assets = catalog
	c := players[0]
	next := c.character.Clone()
	next.Bag = game.Inventory{}
	next.Bag[4] = game.Item{ID: 32011, Count: 1}
	next.Bag[8] = game.Item{ID: 32012, Count: 1}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(context.Background(), c, []byte{40, 1, 9, 5}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) != 4 || got[3][0] != 40 || got[3][2] != 1 {
		t.Fatal("SQL rank/base synthesis", got)
	}
	output := catalog.NativeItems[c.character.Bag[0].ID].AlchemyItem()
	input := min(catalog.NativeItems[32011].AlchemyItem().Rank, catalog.NativeItems[32012].AlchemyItem().Rank)
	if output.Rank > input+4 || output.Bases[0] != catalog.NativeItems[32011].AlchemyItem().Bases[0] {
		t.Fatal("SQL rank/base constraints", output)
	}
}

func compoundFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := worldFixture(t)
	for _, id := range []uint16{100, 200, 300} {
		s.Assets.Items[id] = game.ItemDefinition{ID: id, Type: 23}
	}
	s.Assets.AlchemyRecipes = []assets.AlchemyRecipe{{Input1: 100, Input2: 200, Output: 100}} // Conflicting obsolete table is ignored.
	s.Assets.NativeItems = map[uint16]assets.NativeItem{}
	for _, id := range []uint16{100, 200, 300} {
		it := assets.NativeItem{Definition: s.Assets.Items[id]}
		it.Record[45], it.Record[136], it.Record[406], it.Record[407] = 1, 1, 1, 1
		s.Assets.NativeItems[id] = it
	}
	s.alchemyRandom = func(n int) (int, error) { return n - 1, nil }
	for _, c := range players {
		if err := s.worldCommand(context.Background(), c, []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
	}
	next := players[0].character.Clone()
	next.Bag = game.Inventory{}
	next.Bag[4] = game.Item{ID: 100, Count: 1}
	next.Bag[8] = game.Item{ID: 200, Count: 1}
	if err := s.commit(context.Background(), players[0], next); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	return s, players, wires
}

func TestCompoundNativePacketsPersistenceAndReplay(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	// Exercise dispatch, not only the item handler.
	if err := s.dispatch(ctx, c, []byte{23, 14, 2, 9, 5}); err != nil {
		t.Fatal(err)
	}
	result := append([]byte{23, 8, 5, 44, 1, 1}, make([]byte, 28)...)
	want := [][]byte{{23, 9, 9, 1}, {23, 9, 5, 1}, result, {23, 13, 44, 1, 1, 5}}
	got := wires[0].packets(t)
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("packet %d: %v != %v", i, got[i], want[i])
		}
	}
	peer := wires[1].packets(t)
	if len(peer) != 0 || wires[2].Len() != 0 {
		t.Fatal("synthesis must not broadcast native fishing-stop", peer)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != c.character.Bag || chars[0].Bag[4] != (game.Item{ID: 300, Count: 1}) || !chars[0].Bag[8].Empty() {
		t.Fatal("not persisted", err)
	}
	before := c.character.Bag
	if err := s.dispatch(ctx, c, []byte{23, 14, 2, 9, 5}); err != nil || c.character.Bag != before || wires[1].Len() != 0 {
		t.Fatal("empty-slot replay changed state", err)
	}
}

func TestCompoundRefusesMalformedAndUnavailableInputs(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	before := c.character.Bag
	for _, p := range [][]byte{{23, 14}, {23, 14, 2, 5}, {23, 14, 2, 5, 9, 1}, {23, 14, 3, 5, 9}, {23, 14, 2, 5, 5}, {23, 14, 2, 0, 9}, {23, 14, 2, 51, 9}, {23, 14, 2, 1, 9}} {
		err := s.worldCommand(context.Background(), c, p)
		if len(p) != 5 && err == nil {
			t.Fatalf("truncated or extra packet accepted: %v", p)
		}
		if c.character.Bag != before || wires[1].Len() != 0 {
			t.Fatal("invalid input changed state", p)
		}
	}
	delete(s.Assets.Items, 100)
	s.alchemyIndex = nil
	if err := s.worldCommand(context.Background(), c, []byte{23, 14, 2, 5, 9}); err != nil || c.character.Bag != before || wires[1].Len() != 0 {
		t.Fatal("unknown input", err)
	}
}

func TestCompoundFullBagAndFailedSave(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	next := c.character.Clone()
	for i := range next.Bag {
		next.Bag[i] = game.Item{ID: 100, Count: 2, Damage: 5, Metadata: [26]byte{8}}
	}
	next.Bag[8].ID = 200
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before := c.character.Bag
	if err := s.worldCommand(ctx, c, []byte{23, 14, 2, 5, 9}); err != nil || before != c.character.Bag {
		t.Fatal("full bag consumed ingredients", err)
	}
	wires[0].Reset()
	// A removal can free the other ingredient slot even in a full bag.
	next = c.character.Clone()
	next.Bag[8].Count = 1
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before = c.character.Bag
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.worldCommand(canceled, c, []byte{23, 14, 2, 5, 9}); err == nil || c.character.Bag != before || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("save failure published success", err)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != before {
		t.Fatal("failed save mutated persistence", err)
	}
	if err := s.worldCommand(ctx, c, []byte{23, 14, 2, 5, 9}); err != nil || c.character.Bag[8] != (game.Item{ID: 300, Count: 1}) || c.character.Bag[4].Count != 1 || c.character.Bag[4].Metadata != before[4].Metadata {
		t.Fatal("leftovers", err)
	}
}

func TestCompoundOwnershipGates(t *testing.T) {
	for _, gate := range []string{"loading", "battle", "event", "storm", "beach", "trade", "mounted"} {
		t.Run(gate, func(t *testing.T) {
			s, players, wires := compoundFixture(t)
			c := players[0]
			before := c.character.Bag
			switch gate {
			case "loading":
				c.ready = false
			case "battle":
				c.battle = &battleRun{}
			case "event":
				c.event = &eventSession{}
			case "storm":
				c.storm = true
			case "beach":
				c.beach = &beachRun{}
			case "trade":
				c.trade = &tradeSession{}
			case "mounted":
				c.character.ActiveVehicle = 100
				c.character.VehicleSlot = 5
			}
			err := s.worldCommand(context.Background(), c, []byte{23, 14, 2, 5, 9})
			if gate == "loading" && err == nil {
				t.Fatal("pre-ack accepted")
			}
			if c.character.Bag != before || wires[1].Len() != 0 || wires[2].Len() != 0 {
				t.Fatal("reserved items changed", err)
			}
			packets := wires[0].packets(t)
			if gate == "trade" {
				if len(packets) != 1 || packets[0][1] != 57 {
					t.Fatal("trade warning", packets)
				}
			} else if gate != "mounted" && len(packets) != 0 {
				t.Fatal("gated success", packets)
			}
		})
	}
}

func TestCompoundingMultiInputBooksAndSQLSkills(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	for _, id := range []uint16{game.AlchemyBookOne, game.AlchemyBookFour} {
		s.Assets.Items[id] = game.ItemDefinition{ID: id}
		it := assets.NativeItem{Definition: s.Assets.Items[id]}
		it.Record[45], it.Record[136], it.Record[406], it.Record[407] = byte(game.AlchemyBookBonus(id)), 2, 1, 1
		s.Assets.NativeItems[id] = it
	}
	next := c.character.Clone()
	next.Bag[10] = game.Item{ID: 100, Count: 1}
	next.Bag[18] = game.Item{ID: game.AlchemyBookOne, Count: 1}
	next.Bag[19] = game.Item{ID: game.AlchemyBookFour, Count: 1}
	next.Skills = []game.LearnedSkill{{ID: game.AlchemySuperiorSkill, Grade: 1, EXP: 13}}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 101, 5, 5, 9, 11, 19, 20}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[4].ID != 300 || !c.character.Bag[8].Empty() || !c.character.Bag[10].Empty() || !c.character.Bag[18].Empty() || !c.character.Bag[19].Empty() {
		t.Fatal("multi-input/book debit")
	}
	if c.character.Skills[0].Grade != 2 || c.character.Skills[0].EXP != 0 {
		t.Fatal("alchemy progression", c.character.Skills)
	}
	got := wires[0].packets(t)
	if len(got) < 8 || got[0][2] != 5 || got[4][2] != 20 || got[5][1] != 8 || got[6][1] != 13 {
		t.Fatal("five-input receipts", got)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != c.character.Bag || chars[0].Skills[0] != c.character.Skills[0] {
		t.Fatal("SQL result/progression", err)
	}
}
func TestCompoundingFreshSQLInventoryAndOwnership(t *testing.T) {
	s, players, _ := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	before := c.character.Bag
	ref := store.CharacterRef{Account: c.account.ID, ID: c.character.ID}
	changed, err := s.Store.MutateOwnedCharacter(ctx, ref, func(next *game.Character) error { return next.Bag.Remove(5, 1) })
	if err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 14, 2, 5, 9}); err != nil {
		t.Fatal(err)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != changed.Bag || c.character.Bag != before {
		t.Fatal("stale SQL resources used", err)
	}
}
func TestCompoundingConcurrentReplay(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.dispatch(context.Background(), c, []byte{23, 14, 2, 5, 9}) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	successes := 0
	for _, p := range wires[0].packets(t) {
		if len(p) > 1 && p[0] == 23 && p[1] == 13 {
			successes++
		}
	}
	if successes != 1 || c.character.Bag[4].ID != 300 {
		t.Fatal("duplicate delivery", successes)
	}
}
func TestCompoundingCatastropheAndRandomFailureAtomicity(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	id := uint16(game.AlchemyCommonStone)
	s.Assets.Items[id] = game.ItemDefinition{ID: id}
	it := assets.NativeItem{Definition: s.Assets.Items[id]}
	it.Record[45], it.Record[136], it.Record[406], it.Record[407] = 1, 34, 1, 1
	s.Assets.NativeItems[id] = it
	s.alchemyRandom = func(int) (int, error) { return 0, nil }
	if err := s.dispatch(ctx, c, []byte{23, 14, 2, 5, 9}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[4].ID != id || !c.character.Bag[8].Empty() {
		t.Fatal("catastrophe not delivered/consumed")
	}
	// Failed entropy must leave both SQL and session inventory intact.
	next := c.character.Clone()
	next.Bag = game.Inventory{}
	next.Bag[4] = game.Item{ID: 100, Count: 1}
	next.Bag[8] = game.Item{ID: 200, Count: 1}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	s.alchemyRandom = func(n int) (int, error) { return n, nil }
	before := c.character.Bag
	wires[0].Reset()
	if err := s.dispatch(ctx, c, []byte{23, 14, 2, 5, 9}); err == nil {
		t.Fatal("invalid random value accepted")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != before || c.character.Bag != before || wires[0].Len() != 0 {
		t.Fatal("random failure spent resources", err)
	}
}

func TestCompoundingUsesFreshSQLSkillTier(t *testing.T) {
	s, players, _ := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	ref := store.CharacterRef{Account: c.account.ID, ID: c.character.ID}
	if _, err := s.Store.MutateOwnedCharacter(ctx, ref, func(next *game.Character) error {
		next.Skills = []game.LearnedSkill{{ID: game.AlchemySuperiorSkill, Grade: 4}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, id := c.character.AlchemySkill(); id == game.AlchemySuperiorSkill {
		t.Fatal("fixture was not stale")
	}
	s.worldMu.Lock()
	execution, err := s.executeCompounding(ctx, c, []byte{5, 9}, false)
	s.worldMu.Unlock()
	if err != nil || execution.Outcome.Tier != game.AlchemySuperior || execution.Outcome.Level != 4 || c.character.Skills[0].EXP != 1 {
		t.Fatal("SQL skill tier ignored", execution.Outcome, err)
	}
}
func TestCompoundingNoCandidateConsumesForCatastrophe(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	for id, it := range s.Assets.NativeItems {
		it.Record[45] = 10
		s.Assets.NativeItems[id] = it
	}
	id := uint16(game.AlchemyCommonStone)
	s.Assets.Items[id] = game.ItemDefinition{ID: id}
	it := assets.NativeItem{Definition: s.Assets.Items[id]}
	it.Record[45], it.Record[136], it.Record[406], it.Record[407] = 1, 34, 1, 1
	s.Assets.NativeItems[id] = it
	count := 0
	s.alchemyRandom = func(n int) (int, error) {
		count++
		if count == 1 {
			return n - 1, nil
		}
		return 0, nil
	}
	if err := s.dispatch(ctx, c, []byte{23, 14, 2, 5, 9}); err != nil {
		t.Fatal(err)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != c.character.Bag || c.character.Bag[4].ID != id || !c.character.Bag[8].Empty() {
		t.Fatal("unavailable rank failed to consume inputs and persist junk", err)
	}
	success := false
	for _, p := range wires[0].packets(t) {
		if len(p) > 1 && p[0] == 23 && p[1] == 13 {
			success = true
		}
	}
	if !success {
		t.Fatal("catastrophic fallback did not reply with result")
	}
}

// Raw command bytes come from TRe_CompoundForm's Delphi string literals:
// 17 0e (Primary), 17 57 (Junior), 17 65 (Superior).
func TestCompoundingNativeTierSelectionAndLevelUp(t *testing.T) {
	for _, tc := range []struct {
		name  string
		code  byte
		skill uint16
	}{
		{"primary", 14, game.AlchemyPrimarySkill},
		{"junior", 87, game.AlchemyJuniorSkill},
		{"superior", 101, game.AlchemySuperiorSkill},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, players, wires := compoundFixture(t)
			c := players[0]
			ctx := context.Background()
			next := c.character.Clone()
			next.Skills = []game.LearnedSkill{{ID: game.AlchemyPrimarySkill, Grade: 1, EXP: 13}, {ID: game.AlchemyJuniorSkill, Grade: 1, EXP: 13}, {ID: game.AlchemySuperiorSkill, Grade: 1, EXP: 13}}
			if err := s.commit(ctx, c, next); err != nil {
				t.Fatal(err)
			}
			wires[0].Reset()
			if err := s.dispatch(ctx, c, []byte{23, tc.code, 2, 5, 9}); err != nil {
				t.Fatal(err)
			}
			for _, sk := range c.character.Skills {
				if sk.ID == tc.skill {
					if sk.Grade != 2 || sk.EXP != 0 {
						t.Fatal("selected skill did not advance", sk)
					}
				} else if sk.Grade != 1 || sk.EXP != 13 {
					t.Fatal("unselected skill advanced", sk)
				}
			}
			chars, err := s.Store.Characters(ctx, c.account.ID)
			if err != nil {
				t.Fatal(err)
			}
			for i, sk := range chars[0].Skills {
				if sk != c.character.Skills[i] {
					t.Fatal("progression not persisted")
				}
			}
			found := false
			for _, p := range wires[0].packets(t) {
				if len(p) == 12 && p[0] == 8 && p[1] == 1 && p[2] == 110 && p[4] == 2 && p[8] == byte(tc.skill) && p[9] == byte(tc.skill>>8) {
					found = true
				}
			}
			if !found {
				t.Fatal("missing native grade reply")
			}
		})
	}
}

func TestCompoundingUnlearnedNativeTierRejectsWithoutDebit(t *testing.T) {
	for _, code := range []byte{87, 101} {
		s, players, _ := compoundFixture(t)
		c := players[0]
		ctx := context.Background()
		next := c.character.Clone()
		next.Skills = nil
		if err := s.commit(ctx, c, next); err != nil {
			t.Fatal(err)
		}
		before := c.character.Bag
		if err := s.dispatch(ctx, c, []byte{23, code, 2, 5, 9}); err != nil {
			t.Fatal(err)
		}
		chars, err := s.Store.Characters(ctx, c.account.ID)
		if err != nil || chars[0].Bag != before || c.character.Bag != before {
			t.Fatal("unlearned tier consumed resources", err)
		}
	}
}

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

func TestNativeHotbarAssignmentDoesNotSynthesize(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	before := c.character.Clone()
	// Independent native golden bytes: AC40:1, skill 10001, page 1/F1.
	// Item binding uses ID 100; zero ID removes a binding locally.
	for _, p := range [][]byte{
		{40, 1, 2, 0x11, 0x27, 1, 1}, {40, 1, 2, 0x75, 0xea, 3, 8},
		{40, 1, 1, 100, 0, 2, 5}, {40, 1, 2, 0, 0, 1, 1},
	} {
		if err := s.dispatch(context.Background(), c, p); err != nil {
			t.Fatalf("native binding %x: %v", p, err)
		}
		if c.character.Bag != before.Bag || c.character.Gold != before.Gold {
			t.Fatal("binding modified resources")
		}
		for _, w := range wires {
			if w.Len() != 0 {
				t.Fatal("binding emitted an incompatible alchemy reply")
			}
		}
	}
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || chars[0].Bag != before.Bag || chars[0].Gold != before.Gold {
		t.Fatal("binding modified durable resources", err)
	}
	// Hotbar preferences are also safe during map loading and trade gates.
	c.ready = false
	if err := s.dispatch(context.Background(), c, []byte{40, 1, 2, 0x11, 0x27, 1, 1}); err != nil {
		t.Fatal(err)
	}
}
func TestNativeHotbarMalformed(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	before := c.character.Bag
	for _, p := range [][]byte{
		{40, 1, 2, 0x11, 0x27, 1}, {40, 1, 2, 0x11, 0x27, 1, 1, 0},
		{40, 1, 3, 0x11, 0x27, 1, 1}, {40, 1, 2, 0x11, 0x27, 0, 1},
		{40, 1, 2, 0x11, 0x27, 4, 1}, {40, 1, 2, 0x11, 0x27, 1, 0},
		{40, 1, 2, 0x11, 0x27, 1, 9}, {40, 2, 2, 0x11, 0x27, 1, 1},
	} {
		if err := s.dispatch(context.Background(), c, p); err == nil {
			t.Fatalf("malformed binding accepted: %x", p)
		}
		if c.character.Bag != before || wires[0].Len() != 0 {
			t.Fatal("malformed binding mutated state")
		}
	}
}
