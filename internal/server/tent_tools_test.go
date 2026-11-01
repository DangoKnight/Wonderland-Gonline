package server

import (
	"bytes"
	"context"
	"testing"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

func TestTentAccessoryTreeGolden(t *testing.T) {
	tent := store.Tent{OwnerID: 0x01020304, Items: []store.TentItem{{ItemID: 38049, X: 43, Y: 42, Rotation: 2, Tools: []store.TentTool{{ItemID: 38058}}}}}
	got := nativeTentTree(tent)
	want := make([]byte, 124)
	copy(want, []byte{62, 4, 4, 3, 2, 1, 1, 0, 0xa1, 0x94, 43, 0, 0, 0, 42, 0, 0, 0, 1})
	want[22] = 2 // Root rotation.
	want[27] = 2 // Root's first child index.
	copy(want[65:], []byte{2, 0, 0xaa, 0x94})
	want[82] = 1  // Child's parent index.
	want[115] = 1 // Child bypasses the ground collision grid.
	if !bytes.Equal(got, want) {
		t.Fatal("native 57-byte item tree", got)
	}
}

func TestTentToolAttachmentAndAtomicHostRefund(t *testing.T) {
	s, players, wires := tentFixture(t)
	c := players[0]
	s.Assets.TentToolHosts = []uint16{38027}
	s.Assets.Items[38058] = game.ItemDefinition{ID: 38058, Type: 29}
	c.character.Bag[2] = game.Item{ID: 38058, Count: 2, Damage: 4, Metadata: [26]byte{9}}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, c, []byte{23, 96, 1})
	enterTestTent(t, s, c, c.character.ID)
	for _, w := range wires {
		w.Reset()
	}
	tradeDo(t, s, c, []byte{62, 4, 3, 1, 0})
	home, err := s.Store.Tent(context.Background(), c.character.ID)
	if err != nil || len(home.Items[0].Tools) != 1 || home.Items[0].Tools[0].Damage != 4 || home.Items[0].Tools[0].Metadata[0] != 9 || c.character.Bag[2].Count != 1 {
		t.Fatal("attachment", err)
	}
	if !contains(wires[0].packets(t), []byte{62, 5, 2, 0, 0xaa, 0x94, 1, 0}) {
		t.Fatal("attachment acknowledgement")
	}
	tradeDo(t, s, c, []byte{62, 4, 3, 1, 0})
	if c.character.Bag[2].Count != 1 {
		t.Fatal("duplicate tool consumed")
	}
	for i, it := range c.character.Bag {
		if it.Empty() {
			c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
		}
	}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	before := c.character.Bag
	tradeDo(t, s, c, []byte{62, 2, 1, 0})
	home, err = s.Store.Tent(context.Background(), c.character.ID)
	if err != nil || len(home.Items) != 1 || len(home.Items[0].Tools) != 1 || c.character.Bag != before {
		t.Fatal("partial refund", err)
	}
	c.character.Bag[1], c.character.Bag[3] = game.Item{}, game.Item{}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, c, []byte{62, 2, 1, 0})
	home, err = s.Store.Tent(context.Background(), c.character.ID)
	if err != nil || len(home.Items) != 0 || c.character.Bag[1].ID != 38027 || c.character.Bag[3].ID != 38058 || c.character.Bag[3].Metadata[0] != 9 {
		t.Fatal("host/tool refund", err)
	}
}

func TestTentNativePlacementLayoutAndCollision(t *testing.T) {
	s, players, wires := tentFixture(t)
	c := players[0]
	s.Assets.Items[38049] = game.ItemDefinition{ID: 38049, Type: 29, CellWidth: 3, CellHeight: 2}
	c.character.Bag[2] = game.Item{ID: 38049, Count: 1}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, c, []byte{23, 96, 1})
	enterTestTent(t, s, c, c.character.ID)
	place := func(x, y uint32) []byte { return protocol.Builder{62, 1, 3}.U32(x).U32(y).U32(1) }
	tradeDo(t, s, c, place(43, 42))
	if c.character.Bag[2].ID != 38049 {
		t.Fatal("overlap consumed furniture")
	}
	wires[0].Reset()
	tradeDo(t, s, c, place(29, 38))
	home, err := s.Store.Tent(context.Background(), c.character.ID)
	if err != nil || len(home.Items) != 2 || home.Items[1].X != 29 || home.Items[1].Y != 38 || !c.character.Bag[2].Empty() {
		t.Fatal("native placement", err)
	}
	found := false
	for _, p := range wires[0].packets(t) {
		if len(p) == 61 && p[0] == 62 && p[1] == 1 && p[2] == 2 {
			found = true
		}
	}
	if !found {
		t.Fatal("native placement record missing")
	}
}
