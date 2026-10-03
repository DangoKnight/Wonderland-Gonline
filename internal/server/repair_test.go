package server

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

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
	effect := protocol.Builder{5, 5}.U32(c.character.ID).U16(60015)
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
