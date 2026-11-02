package server

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/world"
)

func TestNPCItemAcquisitionCapacityPreservesRewardsAndAllowsRetry(t *testing.T) {
	s, c, wire := eventFixture(t)
	ctx := context.Background()
	next := c.character.Clone()
	for i := range next.Bag {
		next.Bag[i] = game.Item{ID: 24001, Count: 1}
	}
	next.Bag[0] = game.Item{ID: 32176, Count: game.MaxItemStack - 1}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	wire.Reset()
	start := func() error {
		ev := s.Assets.Maps[10017].Events[0]
		es := &eventSession{mapID: 10017, click: 5, ev: &ev, branch: 1}
		c.event = es
		return s.advance(ctx, c, es)
	}
	if err := start(); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag != before.Bag || c.character.Gold != before.Gold || !reflect.DeepEqual(c.character.Quests, before.Quests) {
		t.Fatal("failed acquisition changed inventory, gold or quest state")
	}
	if !contains(wire.packets(t), serverwire.HeadBanner("Cannot receive reward. Check materials, bag space, gold, party slots and Pet Hotel.")) {
		t.Fatal("failed acquisition did not report insufficient capacity")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != before.Bag || chars[0].Gold != before.Gold || !reflect.DeepEqual(chars[0].Quests, before.Quests) {
		t.Fatal("failed acquisition persisted partial rewards", err)
	}
	next = c.character.Clone()
	next.Bag[game.BagSize-1] = game.Item{}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	if err := start(); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[0].Count != game.MaxItemStack || c.character.Bag[game.BagSize-1] != (game.Item{ID: 32176, Count: 1}) || c.character.Gold != before.Gold+100 || c.character.Quests[900].Step != 1 {
		t.Fatal("reward was not available after freeing sufficient capacity")
	}
	chars, err = s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != c.character.Bag || chars[0].Gold != c.character.Gold || chars[0].Quests[900].Step != 1 {
		t.Fatal("successful retry was not persisted", err)
	}
}

func TestBoxedItemsAtomicExchangeAndFailure(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	s.Assets.Items[26001] = game.ItemDefinition{ID: 26001, Type: 17}
	s.Assets.Items[11101] = game.ItemDefinition{ID: 11101, Type: 1}
	s.Assets.BoxedItems = map[uint16]assets.BoxedItem{26001: {ItemID: 11101, Quantity: 1}}
	c.character.Bag[0] = game.Item{ID: 26001, Count: 2}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if handled, err := s.openBoxedItem(context.Background(), c, 1); !handled || err != nil {
		t.Fatal(handled, err)
	}
	if c.character.Bag[0].Count != 1 || c.character.Bag[1].ID != 11101 {
		t.Fatal(c.character.Bag)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Bag != c.character.Bag {
		t.Fatal("exchange not durable", err)
	}
	for i := range c.character.Bag {
		if c.character.Bag[i].Empty() {
			c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
		}
	}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	// A stack of two boxes cannot release its occupied cell.
	c.character.Bag[0].Count = 2
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	if _, err := s.openBoxedItem(context.Background(), c, 1); err != nil || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("full bag consumed box", err)
	}
	c.character.Bag[2] = game.Item{}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	before = c.character.Clone()
	wire.Reset()
	s.Store.Close()
	if _, err := s.openBoxedItem(context.Background(), c, 1); err == nil || !reflect.DeepEqual(before, *c.character) || wire.Len() != 0 {
		t.Fatal("failed save published", err)
	}
}

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
	if c.character.Bag != before || !contains(wires[0].packets(t), serverwire.HeadBanner("Your inventory is full.")) {
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
	if !contains(wire.packets(t), serverwire.HeadBanner("Cannot receive reward. Check materials, bag space, gold, party slots and Pet Hotel.")) {
		t.Fatal("raft capacity rejection not reported")
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Bag != before.Bag || saved[0].Gold != before.Gold || saved[0].Quests[900] != before.Quests[900] {
		t.Fatal("raft failure persisted progress", err)
	}
}

func TestGroundItemsAndBagMoves(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	s.Assets.Maps[10017] = assets.Map{ID: 10017, Items: []assets.GroundItem{{ClickID: 5, ItemID: 32176, X: 1042, Y: 1075}}}
	s.World = world.New(s.Assets)
	alice, bobby := players[0], players[1]
	for _, c := range players[:2] {
		if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
	}
	for _, w := range wires {
		w.Reset()
	}
	command := func(c *Session, p ...byte) {
		t.Helper()
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
	}
	expect := func(i int, want ...[]byte) {
		t.Helper()
		got := wires[i].packets(t)
		if len(got) != len(want) {
			t.Fatalf("player %d: got %v", i, got)
		}
		for j := range want {
			if !bytes.Equal(got[j], want[j]) {
				t.Fatalf("player %d packet %d:\n got %v\nwant %v", i, j, got[j], want[j])
			}
		}
	}
	record := func(slot byte, id uint16, count byte) []byte {
		return protocol.Builder{23, 5, slot}.U16(id).U8(count).U8(0).Bytes(make([]byte, 26))
	}

	// Slot 1 holds a full stack of the starter item, so the pickup opens slot 2.
	command(alice, 23, 2, 5)
	expect(0, record(2, 32176, 1), []byte{23, 2, 5, 0, 1})
	expect(1, []byte{23, 2, 5, 0, 0})
	command(alice, 23, 2, 5)
	expect(0)
	chars, err := s.Store.Characters(ctx, alice.account.ID)
	if err != nil || chars[0].Bag[1] != (game.Item{ID: 32176, Count: 1}) {
		t.Fatal("pickup not persisted", err)
	}
	respawn := protocol.Builder{23, 4, 3, 5, 0}.U32(32176).U16(1042).U16(1075).U32(0)
	s.respawnGround(time.Now().Add(119 * time.Second))
	expect(0)
	s.respawnGround(time.Now().Add(121 * time.Second))
	expect(0, respawn)
	expect(1, respawn)

	// Each dropped unit takes its own slot, skipping the native node's slot 5.
	command(alice, 23, 3, 1, 2, 0)
	dropped := protocol.Builder{23, 4, 3, 1, 0}.U32(32176).U16(1042).U16(1075).U32(0).U8(3).U16(2).U32(32176).U16(1042).U16(1075).U32(0)
	expect(0, []byte{23, 9, 1, 2}, dropped)
	expect(1, dropped)
	command(bobby, 23, 2, 2)
	expect(1, record(2, 32176, 1), []byte{23, 2, 2, 0, 1})
	expect(0, []byte{23, 2, 2, 0, 0})

	alice.character.Bag[9] = game.Item{ID: 21001, Count: 1}
	command(alice, 23, 3, 10, 1, 0)
	expect(0, protocol.Builder{23, 212, 255, 10}.U16(21001).U8(1))
	command(alice, 23, 10, 2, 1, 20)
	expect(0, []byte{23, 10, 2, 1, 20})
	if alice.character.Bag[19].Count != 1 || !alice.character.Bag[1].Empty() {
		t.Fatal("move not applied")
	}

	for i := range bobby.character.Bag {
		bobby.character.Bag[i] = game.Item{ID: 24001, Count: 1}
	}
	command(bobby, 23, 2, 1)
	expect(1, serverwire.HeadBanner("Your inventory is full."))
	if _, ok := s.World.GroundAt(10017, 1); !ok {
		t.Fatal("failed pickup removed the item")
	}
	if err := s.worldCommand(ctx, alice, []byte{23, 3, 1}); err == nil {
		t.Fatal("truncated drop accepted")
	}
}

func TestWearAndRemoveEquipment(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	s.Assets.Items[21002] = game.ItemDefinition{ID: 21002, EquipSlot: 2, Status: [2]uint16{211, 0}, Values: [2]int32{110, 0}}
	c := players[0]
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	step := func(p []byte, reply []byte, banner string) {
		t.Helper()
		wires[0].Reset()
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		want := 17
		if banner != "" {
			want++
		}
		if len(got) != want || !bytes.Equal(got[0], reply) || got[1][0] != 8 {
			t.Fatalf("%v: %v", p, got)
		}
		if banner != "" && !bytes.Equal(got[17], serverwire.HeadBanner(banner)) {
			t.Fatalf("banner: %q", got[17])
		}
	}
	step([]byte{23, 12, 2, 2}, []byte{23, 16, 2, 2}, "")
	c.character.Bag[2] = game.Item{ID: 21002, Count: 1}
	step([]byte{23, 11, 3}, []byte{23, 17, 3, 3}, "Equipment: DEF +10")
	step([]byte{23, 11, 2}, []byte{23, 17, 2, 2}, "Equipment: DEF -10")
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Equipment[1].ID != 21001 || chars[0].Bag[1].ID != 21002 || !chars[0].Bag[2].Empty() {
		t.Fatal("equipment not persisted", err)
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, []byte{23, 12, 2, 1}); err != nil || wires[0].Len() != 0 {
		t.Fatal("removed onto an occupied bag slot", err)
	}
}

func TestStatAllocation(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	c := players[0]
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	next := c.character.Clone()
	next.StatPoints = 3
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	strength := c.character.Base.Strength
	if err := s.worldCommand(ctx, c, []byte{8, game.StatSTR, 2, 0}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) != 16 || !bytes.Equal(got[11], protocol.Builder{8, 1, 38, 1}.U32(1).U32(0)) {
		t.Fatal("stat block", got)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Base.Strength != strength+2 || chars[0].StatPoints != 1 {
		t.Fatal("allocation not persisted", err)
	}
	if err := s.worldCommand(ctx, c, []byte{8, game.StatSTR, 2, 0}); err != nil || wires[0].Len() != 0 {
		t.Fatal("allocated beyond available points", err)
	}
}

func TestUseAndDestroyItems(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	s.Assets.Items[30201] = game.ItemDefinition{ID: 30201, Type: 23, Status: [2]uint16{25, 0}, Values: [2]int32{150, 0}}
	c := players[0]
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	next := c.character.Clone()
	next.HP = 100
	next.Bag[5] = game.Item{ID: 30201, Count: 3}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, []byte{23, 15, 6, 2, 0, 0}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	// Two potions restore 100 HP, capped at this character's maximum of 181.
	if !bytes.Equal(got[0], []byte{23, 9, 6, 2}) || !bytes.Equal(got[len(got)-1], []byte{23, 15}) || c.character.HP != 181 || c.character.Bag[5].Count != 1 {
		t.Fatal("potion", got[0], c.character.HP)
	}
	// At full HP nothing is used.
	if err := s.worldCommand(ctx, c, []byte{23, 96, 6}); err != nil {
		t.Fatal(err)
	}
	if got = wires[0].packets(t); len(got) != 1 || c.character.Bag[5].Count != 1 {
		t.Fatal("used at full HP", got)
	}
	if err := s.worldCommand(ctx, c, []byte{23, 96, 1}); err != nil {
		t.Fatal(err)
	}
	if got = wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], serverwire.HeadBanner("Select a suitable target to use this item.")) {
		t.Fatal("non-recovery item", got)
	}
	if err := s.worldCommand(ctx, c, []byte{23, 124, 1, 10, 0}); err != nil {
		t.Fatal(err)
	}
	got = wires[0].packets(t)
	if len(got) != 2 || !bytes.Equal(got[0], protocol.Builder{23, 26}.U16(32176).U8(10)) || c.character.Bag[0].Count != 40 {
		t.Fatal("destroy", got)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag[0].Count != 40 || chars[0].HP != c.character.HP {
		t.Fatal("not persisted", err)
	}
}

func repairFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := compoundFixture(t)
	s.Assets.Items[48050] = game.ItemDefinition{ID: 48050}
	c := players[0]
	next := c.character.Clone()
	next.Gold = 700
	next.Bag = game.Inventory{}
	next.Bag[4] = game.Item{ID: 100, Count: 2, Damage: 73, Metadata: [26]byte{7, 8, 9}}
	next.Bag[8] = game.Item{ID: 48050, Count: 2, Damage: 6, Metadata: [26]byte{11}}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	return s, players, wires
}

func TestRepairWrenchPacketsPersistenceAndHealthyReplay(t *testing.T) {
	s, players, wires := repairFixture(t)
	c := players[0]
	ctx := context.Background()
	before := c.character.Clone()
	if err := s.dispatch(ctx, c, []byte{36, 7, 5}); err != nil {
		t.Fatal(err)
	}
	record := append([]byte{23, 5, 5, 100, 0, 2, 0, 7, 8, 9}, make([]byte, 23)...)
	effect := protocol.Builder{5, 5}.U32(c.character.ID).U16(0)
	message := []byte("[Blacksmith Repair] Successfully restored item #100 to maximum durability!")
	line := append([]byte{23, 57, 0, byte(len(message))}, message...)
	want := [][]byte{{23, 9, 9, 1}, {23, 9, 5, 2}, record, effect, line, {36, 7, 5, 1}}
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
	if len(peer) != 1 || !bytes.Equal(peer[0], effect) || wires[2].Len() != 0 {
		t.Fatal("map isolation", peer)
	}
	before.Bag[4].Damage = 0
	before.Bag[8].Count--
	if c.character.Bag != before.Bag || c.character.Gold != 700 {
		t.Fatal("repair changed unrelated state")
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Bag != before.Bag || saved[0].Gold != 700 {
		t.Fatal("repair not persisted", err)
	}
	if err := s.dispatch(ctx, c, []byte{36, 7, 5}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag != before.Bag || c.character.Gold != 700 || wires[1].Len() != 0 {
		t.Fatal("healthy replay charged payment")
	}
	got = wires[0].packets(t)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{36, 7, 5, 0}) {
		t.Fatal(got)
	}
}

func TestRepairGoldAndPaymentFailures(t *testing.T) {
	for _, gold := range []uint32{0, 499, 500, 700} {
		t.Run(fmt.Sprintf("gold_%d", gold), func(t *testing.T) {
			s, players, wires := repairFixture(t)
			c := players[0]
			ctx := context.Background()
			next := c.character.Clone()
			next.Bag[8] = game.Item{}
			next.Gold = gold
			if err := s.commit(ctx, c, next); err != nil {
				t.Fatal(err)
			}
			before := c.character.Clone()
			if err := s.dispatch(ctx, c, []byte{36, 1, 5}); err != nil {
				t.Fatal(err)
			}
			got := wires[0].packets(t)
			if gold < 500 {
				if c.character.Bag != before.Bag || c.character.Gold != gold || len(got) != 2 || !bytes.Equal(got[1], []byte{36, 1, 5, 0}) || wires[1].Len() != 0 {
					t.Fatal("unfunded repair", got)
				}
			} else {
				if c.character.Gold != gold-500 || c.character.Bag[4].Damage != 0 || len(got) != 6 || !bytes.Equal(got[0], protocol.Builder{26, 4}.U32(gold-500)) {
					t.Fatal("gold repair", got)
				}
				saved, err := s.Store.Characters(ctx, c.account.ID)
				if err != nil || saved[0].Gold != gold-500 || saved[0].Bag != c.character.Bag {
					t.Fatal("gold not saved", err)
				}
			}
		})
	}
}

func TestRepairMalformedUnknownAndFailedSave(t *testing.T) {
	s, players, wires := repairFixture(t)
	c := players[0]
	ctx := context.Background()
	before := c.character.Clone()
	for _, p := range [][]byte{{36}, {36, 1}, {36, 1, 5, 0}} {
		if err := s.dispatch(ctx, c, p); err == nil {
			t.Fatal("malformed accepted", p)
		}
		if c.character.Bag != before.Bag || c.character.Gold != before.Gold || wires[0].Len() != 0 {
			t.Fatal("malformed changed state")
		}
	}
	for _, slot := range []byte{0, 1, 51, 255} {
		if err := s.dispatch(ctx, c, []byte{36, 1, slot}); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		if !bytes.Equal(got[len(got)-1], []byte{36, 1, slot, 0}) {
			t.Fatal(got)
		}
	}
	delete(s.Assets.Items, 100)
	if err := s.dispatch(ctx, c, []byte{36, 1, 5}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{36, 1, 5, 0}) {
		t.Fatal(got)
	}
	s.Assets.Items[100] = game.ItemDefinition{ID: 100}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.dispatch(canceled, c, []byte{36, 1, 5}); err == nil {
		t.Fatal("save failure missing")
	}
	if c.character.Bag != before.Bag || c.character.Gold != before.Gold || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("failed save charged or sent success")
	}
}

func TestRepairGameplayGates(t *testing.T) {
	for _, gate := range []string{"loading", "battle", "trade", "event", "storm", "beach", "mounted"} {
		t.Run(gate, func(t *testing.T) {
			s, players, wires := repairFixture(t)
			c := players[0]
			switch gate {
			case "loading":
				c.ready = false
				c.warped = true
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
			}
			before := c.character.Clone()
			if err := s.dispatch(context.Background(), c, []byte{36, 1, 5}); err != nil {
				t.Fatal(err)
			}
			if c.character.Bag != before.Bag || c.character.Gold != before.Gold || wires[1].Len() != 0 {
				t.Fatal("gate mutated state")
			}
			got := wires[0].packets(t)
			if gate != "trade" && len(got) != 0 {
				t.Fatal("gate sent repair", got)
			}
		})
	}
}

// Apply the receipts as an additive native inventory decoder would, including
// payment from the repaired stack itself. A full bag needs no spare slot.
func TestRepairReceiptsPreserveFullBagAndSameSlotPayment(t *testing.T) {
	s, players, wires := repairFixture(t)
	c := players[0]
	ctx := context.Background()
	next := c.character.Clone()
	for i := range next.Bag {
		next.Bag[i] = game.Item{ID: 100, Count: 1, Damage: 12, Metadata: [26]byte{byte(i)}}
	}
	next.Bag[4] = game.Item{ID: 48050, Count: 2, Damage: 70, Metadata: [26]byte{77}}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	client := next.Bag
	if err := s.dispatch(ctx, c, []byte{36, 255, 5}); err != nil {
		t.Fatal(err)
	}
	packets := wires[0].packets(t)
	for _, p := range packets {
		if p[0] != 23 {
			continue
		}
		switch p[1] {
		case 9:
			if err := client.Remove(p[2], p[3]); err != nil {
				t.Fatal("client removal", err)
			}
		case 5:
			for i := 2; i < len(p); i += 31 {
				r := p[i : i+31]
				slot := r[0]
				item := game.Item{ID: uint16(r[1]) | uint16(r[2])<<8, Count: r[3], Damage: r[4]}
				copy(item.Metadata[:], r[5:])
				if !client[slot-1].Empty() {
					t.Fatal("repaired record overlays old stack")
				}
				client[slot-1] = item
			}
		}
	}
	if client != c.character.Bag || c.character.Gold != 700 || client[4].Count != 1 || client[4].Damage != 0 || client[4].Metadata[0] != 77 {
		t.Fatal("native inventory diverged")
	}
	if !bytes.Equal(packets[len(packets)-1], []byte{36, 255, 5, 1}) {
		t.Fatal("subcommand not echoed", packets)
	}
}

func TestVoucherRefusalsRetainInventory(t *testing.T) {
	a := &assets.Catalog{PetVouchers: map[uint16]uint16{30068: 14719}, Items: map[uint16]game.ItemDefinition{30068: {ID: 30068}}, NPCs: map[uint16]assets.NPC{14719: {ID: 14719}}}
	s := &Server{Assets: a, World: world.New(a)}
	for _, tc := range []struct {
		count  byte
		target uint16
		pets   []game.Pet
	}{
		{2, 0, nil}, {1, 1, nil}, {1, 0, []game.Pet{{ID: 14719, Slot: 1}}},
		{1, 0, []game.Pet{{ID: 1, Slot: 1}, {ID: 2, Slot: 2}, {ID: 3, Slot: 3}, {ID: 4, Slot: 4}}},
	} {
		wire := &captureConn{}
		char := &game.Character{Pets: tc.pets, Quests: map[uint32]game.Quest{}}
		char.Bag[0] = game.Item{ID: 30068, Count: 2}
		before := char.Clone()
		c := &Session{conn: wire, character: char, pets: newPetRoster()}
		handled, err := s.redeemVoucher(context.Background(), c, 1, tc.count, tc.target)
		if !handled || err != nil || !reflect.DeepEqual(before, *char) || len(wire.packets(t)) != 1 {
			t.Fatal("voucher refusal", tc, handled, err)
		}
	}
}

func TestVoucherRedemptionPersistence(t *testing.T) {
	s, players, wires := petFixture(t)
	c, wire := players[0], wires[0]
	s.Assets.PetVouchers = map[uint16]uint16{30068: 14156}
	s.Assets.Items[30068] = game.ItemDefinition{ID: 30068}
	next := c.character.Clone()
	next.Bag[8] = game.Item{ID: 30068, Count: 2}
	ctx := context.Background()
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.worldCommand(ctx, c, []byte{23, 96, 9}); err != nil {
		t.Fatal(err)
	}
	got := wire.packets(t)
	if !contains(got, []byte{23, 9, 9, 1}) || !contains(got, []byte{23, 15}) || c.pets.slot(14156) == 0 {
		t.Fatal("voucher packets", got)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || len(chars[0].Pets) != 1 || chars[0].Bag[8].Count != 1 {
		t.Fatal("voucher state", err)
	}
	if err := s.worldCommand(ctx, c, []byte{23, 96, 9}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[8].Count != 1 || len(c.character.Pets) != 1 {
		t.Fatal("duplicate consumed voucher")
	}
}
