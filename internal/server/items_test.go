package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

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
	expect(1, headBanner("Your inventory is full."))
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
		want := 16
		if banner != "" {
			want++
		}
		if len(got) != want || !bytes.Equal(got[0], reply) || got[1][0] != 8 {
			t.Fatalf("%v: %v", p, got)
		}
		if banner != "" && !bytes.Equal(got[16], headBanner(banner)) {
			t.Fatalf("banner: %q", got[16])
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
	if len(got) != 15 || !bytes.Equal(got[10], protocol.Builder{8, 1, 38, 1}.U32(1).U32(0)) {
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
	if got = wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], headBanner("Select a suitable target to use this item.")) {
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
