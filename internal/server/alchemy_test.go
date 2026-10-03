package server

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

func TestAlchemyPacketsPersistenceAndReplay(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	// AC40 echoes subcommands; recipe lookup is symmetric and first-match wins.
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
	for _, name := range []string{"missing recipe", "unknown output", "unknown input"} {
		t.Run(name, func(t *testing.T) {
			recipes := s.Assets.AlchemyRecipes
			items := s.Assets.Items
			s.Assets.Items = map[uint16]game.ItemDefinition{}
			for id, definition := range items {
				s.Assets.Items[id] = definition
			}
			switch name {
			case "missing recipe":
				s.Assets.AlchemyRecipes = nil
			case "unknown output":
				delete(s.Assets.Items, 300)
			case "unknown input":
				delete(s.Assets.Items, 100)
			}
			if err := s.dispatch(context.Background(), c, []byte{40, 1, 5, 9}); err != nil {
				t.Fatal(err)
			}
			got := wires[0].packets(t)
			if c.character.Bag != before || len(got) != 1 || !bytes.Equal(got[0], []byte{40, 1, 0, 0, 0}) {
				t.Fatal("unavailable recipe consumed items", got)
			}
			s.Assets.Items = items
			s.Assets.AlchemyRecipes = recipes
		})
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
			} else if len(got) != 0 {
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
	next.Bag[4] = game.Item{ID: 37206, Count: 1}
	next.Bag[8] = game.Item{ID: 37207, Count: 1}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(context.Background(), c, []byte{40, 1, 9, 5}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) != 4 || !bytes.Equal(got[3], []byte{40, 1, 1, 63, 78}) || c.character.Bag[0].ID != 20031 {
		t.Fatal("SQL recipe synthesis failed", got)
	}
}
