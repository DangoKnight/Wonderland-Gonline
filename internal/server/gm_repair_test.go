package server

import (
	"bytes"
	"context"
	"reflect"
	"strconv"
	"testing"
	"wonderland-go/internal/game"
)

func gmRepairFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := repairFixture(t)
	for _, c := range players {
		s.sessions[c.info.ID] = c
		s.accounts[c.account.ID] = c.info.ID
	}
	return s, players, wires
}

func TestGMRepairPacketsAndPersistence(t *testing.T) {
	s, players, wires := gmRepairFixture(t)
	c := players[0]
	s.SetGMLevel(c.account.ID, 1)
	next := c.character.Clone()
	next.Equipment = game.Equipment{}
	next.Equipment[0] = game.Item{ID: 100, Count: 1, Damage: 99, Metadata: [26]byte{8}}
	next.Equipment[0].Metadata[game.ForgeMetadataOffset] = 3
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	before := next.Clone()
	say(t, s, c, "/repair")
	expected := before.Clone()
	expected.Bag[4].Damage, expected.Bag[8].Damage, expected.Equipment[0].Damage = 0, 0, 0
	if !reflect.DeepEqual(*c.character, expected) {
		t.Fatal("repair changed more than durability")
	}
	packets := wires[0].packets(t)
	if len(packets) != 5 || !bytes.Equal(packets[0], []byte{23, 9, 5, 2}) || !bytes.Equal(packets[1], []byte{23, 9, 9, 2}) {
		t.Fatal(packets)
	}
	// Reapply raw native additive inventory records to the pre-repair client state.
	client := before.Bag
	for _, p := range packets[:3] {
		switch p[1] {
		case 9:
			if err := client.Remove(p[2], p[3]); err != nil {
				t.Fatal(err)
			}
		case 5:
			for i := 2; i < len(p); i += 31 {
				r := p[i : i+31]
				if !client[r[0]-1].Empty() {
					t.Fatal("repaired stack overlaid old record")
				}
				item := game.Item{ID: uint16(r[1]) | uint16(r[2])<<8, Count: r[3], Damage: r[4]}
				copy(item.Metadata[:], r[5:])
				client[r[0]-1] = item
			}
		}
	}
	if client != expected.Bag {
		t.Fatal("native bag diverged")
	}
	gear := append([]byte{23, 11, 100, 0, 0}, make([]byte, 18)...)
	gear[17] = 3
	if !bytes.Equal(packets[3], gear) {
		t.Fatal("equipment durability/forge layout", packets[3])
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || !reflect.DeepEqual(saved[0], expected) {
		t.Fatal("repair not durable", err)
	}
	if wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("private repair leaked")
	}
	say(t, s, c, ":fixall")
	if packets := wires[0].packets(t); len(packets) != 1 || packets[0][0] != 2 {
		t.Fatal("healthy replay rewrote inventory", packets)
	}
}

func TestGMRepairTargetsAndAuthorization(t *testing.T) {
	s, players, wires := gmRepairFixture(t)
	actor, target := players[1], players[0]
	before := target.character.Clone()
	say(t, s, actor, "/repair "+target.character.Name)
	if !reflect.DeepEqual(*target.character, before) || wires[0].Len() != 0 {
		t.Fatal("non-GM repaired target")
	}
	s.SetGMLevel(actor.account.ID, 1)
	say(t, s, actor, "/repair missing")
	say(t, s, actor, "/repair one two")
	if !reflect.DeepEqual(*target.character, before) || wires[0].Len() != 0 {
		t.Fatal("bad target changed state")
	}
	wires[1].Reset()
	say(t, s, actor, "/fixall "+strconv.FormatUint(uint64(target.character.ID), 10))
	if target.character.Bag[4].Damage != 0 || target.character.Gold != before.Gold || wires[0].Len() == 0 || len(wires[1].packets(t)) != 1 {
		t.Fatal("target repair failed")
	}
	if wires[2].Len() != 0 {
		t.Fatal("target repair broadcast")
	}
	s.SetGMLevel(actor.account.ID, 0)
	target.character.Bag[4].Damage = 1
	say(t, s, actor, "/repair "+target.character.Name)
	if target.character.Bag[4].Damage != 1 {
		t.Fatal("revoked GM repaired")
	}
}

func TestGMRepairInteractionGates(t *testing.T) {
	for _, owner := range []int{0, 1} {
		for _, gate := range []string{"loading", "battle", "event", "storm", "beach", "trade", "minigame"} {
			t.Run(strconv.Itoa(owner)+gate, func(t *testing.T) {
				s, players, wires := gmRepairFixture(t)
				actor, target := players[1], players[0]
				s.SetGMLevel(actor.account.ID, 1)
				c := target
				if owner == 1 {
					c = actor
				}
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
				case "minigame":
					c.event = &eventSession{onMinigame: func(byte) error { return nil }}
				}
				before := target.character.Clone()
				err := s.worldCommand(context.Background(), actor, append([]byte{2, 2}, []byte("/repair "+target.character.Name)...))
				if owner == 1 && gate == "loading" {
					if err == nil {
						t.Fatal("loading actor accepted")
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(*target.character, before) || wires[0].Len() != 0 {
					t.Fatal("busy repair changed target")
				}
			})
		}
	}
}

func TestGMRepairFailedSave(t *testing.T) {
	s, players, wires := gmRepairFixture(t)
	c := players[0]
	s.SetGMLevel(c.account.ID, 1)
	before := c.character.Clone()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.worldCommand(ctx, c, append([]byte{2, 2}, []byte("/repair")...)); err == nil {
		t.Fatal("missing save failure")
	}
	if !reflect.DeepEqual(*c.character, before) || wires[0].Len() != 0 {
		t.Fatal("failed save mutated or published")
	}
}

func TestGMRepairFailedTargetDelivery(t *testing.T) {
	s, players, wires := gmRepairFixture(t)
	actor, target := players[1], players[0]
	s.SetGMLevel(actor.account.ID, 1)
	broken := &failedWorldConn{}
	target.conn = broken
	say(t, s, actor, "/repair "+target.character.Name)
	if !broken.closed || target.character.Bag[4].Damage != 0 {
		t.Fatal("committed repair lost or failed target not disconnected")
	}
	saved, err := s.Store.Characters(context.Background(), target.account.ID)
	if err != nil || saved[0].Bag != target.character.Bag {
		t.Fatal("repair not saved", err)
	}
	if len(wires[1].packets(t)) != 1 {
		t.Fatal("GM not told to reconnect target")
	}
}
