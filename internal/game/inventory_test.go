package game

import (
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

func TestGrantRollbackAndMetadata(t *testing.T) {
	var b Inventory
	for i := range b {
		b[i] = Item{ID: 1, Count: 50}
	}
	b[0].Count = 49
	before := b
	if e := b.Add(Item{ID: 1}, 2, 50); !errors.Is(e, ErrInventoryFull) || b != before {
		t.Fatal("partial grant", e)
	}
	if e := b.Add(Item{ID: 1}, 1, 50); e != nil {
		t.Fatal(e)
	}
	p := b.Packet(23, 5)
	if len(p) != 2+50*31 || binary.LittleEndian.Uint16(p[3:5]) != 1 {
		t.Fatal("record layout")
	}
}
func TestTransferRollback(t *testing.T) {
	var a, b Inventory
	a[0] = Item{ID: 2, Count: 5}
	for i := range b {
		b[i] = Item{ID: 1, Count: 50}
	}
	before := a
	if e := Transfer(&a, &b, 1, 5, 50); e == nil || a != before {
		t.Fatal("lost source items")
	}
	b[0] = Item{}
	if e := Transfer(&a, &b, 1, 5, 50); e != nil || !a[0].Empty() || b[0].Count != 5 {
		t.Fatal(e)
	}
}
func TestQuestRewardCannotReplay(t *testing.T) {
	c := Character{Quests: map[uint32]Quest{1: {ID: 1, State: InProgress, Step: 1}}}
	reward := []Item{{ID: 3, Count: 1}}
	limit := func(uint16) byte { return 50 }
	if e := c.GrantQuestReward(1, reward, limit, time.Now()); e != nil {
		t.Fatal(e)
	}
	if e := c.GrantQuestReward(1, reward, limit, time.Now()); e == nil || c.Bag[0].Count != 1 {
		t.Fatal("reward replay")
	}
}

func TestGrantReportsAdditions(t *testing.T) {
	var b Inventory
	b[2] = Item{ID: 7, Count: 48}
	adds, e := b.Grant(Item{ID: 7}, 5, 50)
	if e != nil || len(adds) != 2 || adds[0] != (Addition{3, 2}) || adds[1] != (Addition{1, 3}) {
		t.Fatal(adds, e)
	}
	p := b.AdditionPacket(adds)
	if len(p) != 2+2*31 || p[2] != 3 || p[5] != 2 || p[33] != 1 || p[36] != 3 {
		t.Fatalf("additive AC23:5: %v", p)
	}
}

func TestMoveMovesWhatFits(t *testing.T) {
	var b Inventory
	b[0] = Item{ID: 7, Count: 10}
	b[1] = Item{ID: 7, Count: 45}
	b[2] = Item{ID: 8, Count: 1}
	if moved, e := b.Move(1, 2, 10, 50); e != nil || moved != 5 || b[0].Count != 5 || b[1].Count != 50 {
		t.Fatal(moved, e, b[:2])
	}
	if moved, e := b.Move(1, 4, 9, 50); e != nil || moved != 5 || !b[0].Empty() || b[3].Count != 5 {
		t.Fatal("whole stack move", moved, e)
	}
	before := b
	if _, e := b.Move(4, 3, 1, 50); e == nil || b != before {
		t.Fatal("moved onto a different item")
	}
	if _, e := b.Move(2, 5, 1, 1); e != nil || b[4].Count != 1 || b[1].Count != 49 {
		t.Fatal("non-stackable move", e)
	}
}

func TestApplyQuestItems(t *testing.T) {
	limit := func(id uint16) (byte, bool) { return map[uint16]byte{1: 50, 2: 1, 3: 50}[id], id >= 1 && id <= 3 }
	var b Inventory
	for i := range b {
		b[i] = Item{ID: 2, Count: 1}
	}
	b[4] = Item{ID: 1, Count: 3}
	before := b
	// The bag is full: the reward fits only because the hand-in frees slot 5 first.
	r, e := b.ApplyQuestItems([]ItemChange{{1, -3}, {3, 2}}, limit)
	if e != nil || b[4] != (Item{ID: 3, Count: 2}) || len(r.Removed) != 1 || r.Removed[0] != (Addition{5, 3}) || len(r.Added) != 1 || r.Added[0] != (Addition{5, 2}) {
		t.Fatal(r, e, b[4])
	}
	b = before
	if _, e := b.ApplyQuestItems([]ItemChange{{3, 2}, {1, -3}}, limit); e == nil || b != before {
		t.Fatal("reward before hand-in should not fit")
	}
	if _, e := b.ApplyQuestItems([]ItemChange{{1, -4}}, limit); e == nil || b != before {
		t.Fatal("removed more than held")
	}
	if _, e := b.ApplyQuestItems([]ItemChange{{9, 1}}, limit); e == nil {
		t.Fatal("unknown item")
	}
	r, e = b.ApplyQuestItems([]ItemChange{{1, -1}}, limit)
	if e != nil || len(r.Removed) != 1 || r.Removed[0] != (Addition{5, 1}) || b[4].Count != 2 {
		t.Fatal("partial hand-in", r, e)
	}
}
