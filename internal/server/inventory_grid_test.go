package server

import (
	"bytes"
	"context"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

func TestMultiSlotGroundPickupPreservesSourceAndSQL(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	s.Assets.Items[48016] = game.ItemDefinition{ID: 48016, Name: "Robinson Raft", CellWidth: 4, CellHeight: 3}
	s.Assets.Maps[c.character.Map] = assets.Map{ID: c.character.Map, Items: []assets.GroundItem{{ClickID: 5, ItemID: 48016, X: uint32(c.character.X), Y: uint32(c.character.Y)}}}
	s.World = world.New(s.Assets)
	next := c.character.Clone()
	next.Bag = game.Inventory{}
	for i := 2; i < game.BagSize; i += game.BagColumns {
		next.Bag[i] = game.Item{ID: 24001, Count: 1}
	}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before := c.character.Bag
	wires[0].Reset()
	if err := s.pickup(ctx, c, 5); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag != before || !contains(wires[0].packets(t), headBanner("Your inventory is full.")) {
		t.Fatal("fragmented pickup accepted")
	}
	if _, ok := s.World.GroundAt(c.character.Map, 5); !ok {
		t.Fatal("failed acquisition removed raft")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != before {
		t.Fatal("failed acquisition persisted partial state", err)
	}
	next = c.character.Clone()
	next.Bag = game.Inventory{}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.pickup(ctx, c, 5); err != nil {
		t.Fatal(err)
	}
	packets := wires[0].packets(t)
	want := protocol.Builder{23, 5, 1}.U16(48016).U8(1).U8(0).Bytes(make([]byte, 26))
	if len(packets) != 2 || !bytes.Equal(packets[0], want) {
		t.Fatal("raft anchor reply", packets)
	}
	if _, ok := s.World.GroundAt(c.character.Map, 5); ok {
		t.Fatal("successful pickup retained source")
	}
	if err := s.pickup(ctx, c, 5); err != nil || wires[0].Len() != 0 {
		t.Fatal("pickup replay", err)
	}
	cells, err := c.character.Bag.Occupancy(s.Assets.Items)
	if err != nil || cells[13] != 1 {
		t.Fatal("raft footprint", err, cells)
	}
	chars, err = s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != c.character.Bag {
		t.Fatal("raft persistence", err)
	}
}

func TestMultiSlotStorageTransferRejectsCoveredAndFragmentedCells(t *testing.T) {
	s, _, _ := worldFixture(t)
	s.Assets.Items[48016] = game.ItemDefinition{ID: 48016, CellWidth: 4, CellHeight: 3}
	var storage, bag game.Inventory
	storage[0] = game.Item{ID: 48016, Count: 1}
	for i := 2; i < game.BagSize; i += game.BagColumns {
		bag[i] = game.Item{ID: 24001, Count: 1}
	}
	beforeStorage, beforeBag := storage, bag
	if moved, _ := s.transfer(&storage, &bag, 1, 0); moved != 0 || storage != beforeStorage || bag != beforeBag {
		t.Fatal("fragmented withdrawal changed inventories")
	}
	bag = game.Inventory{}
	if moved, _ := s.transfer(&storage, &bag, 1, 0); moved != 1 || !storage[0].Empty() || bag[0].ID != 48016 {
		t.Fatal("raft withdrawal")
	}
	storage[0] = game.Item{ID: 24001, Count: 1}
	beforeStorage, beforeBag = storage, bag
	if moved, _ := s.transfer(&storage, &bag, 1, 2); moved != 0 || storage != beforeStorage || bag != beforeBag {
		t.Fatal("withdrawal onto covered cell")
	}
	storage[0].Locked = true
	if moved, _ := s.transfer(&storage, &bag, 1, 5); moved != 0 || !bag[4].Empty() {
		t.Fatal("locked source duplicated")
	}
}

func TestMultiSlotNPCRewardRejectsGoldAndProgressWithoutSpace(t *testing.T) {
	s, c, wire := eventFixture(t)
	ctx := context.Background()
	s.Assets.Items[48016] = game.ItemDefinition{ID: 48016, CellWidth: 4, CellHeight: 3}
	next := c.character.Clone()
	next.Bag = game.Inventory{}
	for i := 2; i < game.BagSize; i += game.BagColumns {
		next.Bag[i] = game.Item{ID: 24001, Count: 1}
	}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	ev := s.Assets.Maps[c.character.Map].Events[0]
	ev.Branches[1].Operations[1] = evOp(2, 1, 1, 1, 48016, 1)
	c.event = &eventSession{mapID: c.character.Map, click: 5, ev: &ev, branch: 1}
	if err := s.advance(ctx, c, c.event); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag != before.Bag || c.character.Gold != before.Gold || c.character.Quests[900] != before.Quests[900] {
		t.Fatal("raft acquisition failed after partial rewards")
	}
	if !contains(wire.packets(t), headBanner("Cannot receive reward. Check materials, bag space, gold, party slots and Pet Hotel.")) {
		t.Fatal("raft capacity rejection not reported")
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Bag != before.Bag || saved[0].Gold != before.Gold || saved[0].Quests[900] != before.Quests[900] {
		t.Fatal("raft failure persisted progress", err)
	}
}
