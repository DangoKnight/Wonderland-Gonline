package server

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

func feedingFixture(t *testing.T) (*Server, *Session, []*captureConn) {
	t.Helper()
	s, c, wires := rebirthFixture(t)
	s.Assets.Items[50000] = game.ItemDefinition{ID: 50000, Type: 23, Status: [2]uint16{64, 64}, Values: [2]int32{107, 103}}
	next := c.character.Clone()
	next.Pets[1].Amity = 95
	next.Bag[8] = game.Item{ID: 50000, Count: 5, Damage: 7, Metadata: [26]byte{18: 2}}
	next.Bag[16] = game.Item{ID: 50000, Count: 3}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	return s, c, wires
}

func TestPetFeedingPacketsConsumptionAndPersistence(t *testing.T) {
	s, c, wires := feedingFixture(t)
	before := c.character.Clone()
	// Native request is a WORD item ID, not either the bag or party slot.
	if err := s.dispatch(context.Background(), c, []byte{67, 1, 0x50, 0xc3}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) != 5 || !bytes.Equal(got[0], []byte{23, 9, 9, 1}) || !bytes.Equal(got[len(got)-1], []byte{67, 1, 0x50, 0xc3, 1}) || !contains(got, protocol.Builder{8, 2, 4, 2, 0, 64, 1}.U32(100).U32(0)) || !contains(got, []byte{23, 15}) {
		t.Fatal("feeding wire packets", got)
	}
	if c.character.Pets[1].Amity != 100 || c.character.Bag[8].Count != 4 || c.character.Bag[8].Metadata != before.Bag[8].Metadata || c.character.Bag[8].Damage != 7 || c.character.Bag[16] != before.Bag[16] || !reflect.DeepEqual(c.character.Pets[0], before.Pets[0]) {
		t.Fatal("wrong food/pet consumed", c.character.Pets, c.character.Bag)
	}
	if wires[1].Len() != 0 {
		t.Fatal("feeding emitted peer packets")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || !reflect.DeepEqual(saved[0].Pets, c.character.Pets) || saved[0].Bag != c.character.Bag {
		t.Fatal("feeding not durable", err)
	}
	// Feeding at the cap fails without wasting another food unit.
	after := c.character.Clone()
	if err := s.dispatch(context.Background(), c, []byte{67, 1, 0x50, 0xc3}); err != nil {
		t.Fatal(err)
	}
	got = wires[0].packets(t)
	if len(got) != 2 || !bytes.Equal(got[1], []byte{67, 1, 0x50, 0xc3, 0}) || !reflect.DeepEqual(after, *c.character) {
		t.Fatal("cap consumed food", got)
	}
}

func TestPetFeedingInvalidTargetsFoodAndSaveFailure(t *testing.T) {
	s, c, wires := feedingFixture(t)
	before := c.character.Clone()
	for _, p := range [][]byte{{67}, {67, 1}, {67, 1, 0x50}, {67, 1, 0x50, 0xc3, 0}, {67, 2, 0x50, 0xc3}} {
		if err := s.dispatch(context.Background(), c, p); err == nil || wires[0].Len() != 0 || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("malformed feeding changed state", p, err)
		}
	}
	for _, change := range []func(){
		func() { c.character.Pets[1].Battle = false },
		func() { c.pets.slots = map[uint32]byte{} },
		func() { c.character.Bag[8], c.character.Bag[16] = game.Item{}, game.Item{} },
		func() { s.Assets.Items[50000] = game.ItemDefinition{ID: 50000} },
	} {
		change()
		snapshot := c.character.Clone()
		if err := s.dispatch(context.Background(), c, []byte{67, 1, 0x50, 0xc3}); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		if len(got) != 2 || !bytes.Equal(got[1], []byte{67, 1, 0x50, 0xc3, 0}) || !reflect.DeepEqual(snapshot, *c.character) {
			t.Fatal("invalid feeding succeeded", got)
		}
		*c.character = before.Clone()
		c.pets = newPetRoster()
		for _, p := range c.character.Pets {
			c.pets.register(p.ID)
		}
		s.Assets.Items[50000] = game.ItemDefinition{ID: 50000, Type: 23, Status: [2]uint16{64, 64}, Values: [2]int32{107, 103}}
	}
	for _, food := range []uint16{0, 49999} {
		if err := s.dispatch(context.Background(), c, protocol.Builder{67, 1}.U16(food)); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		if len(got) != 2 || !bytes.Equal(got[1], protocol.Builder{67, 1}.U16(food).U8(0)) || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("missing food mutated state", got)
		}
	}
	s.Store.Close()
	if err := s.dispatch(context.Background(), c, []byte{67, 1, 0x50, 0xc3}); err == nil || !reflect.DeepEqual(before, *c.character) || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("failed save consumed/published food", err)
	}
}

func TestPetFeedingWorldGatesAndSharedInputBounds(t *testing.T) {
	s, c, wires := feedingFixture(t)
	before := c.character.Clone()
	// Shared feeding is safe even when an internal caller supplies bad inputs.
	for _, request := range [][3]byte{{0, 1, 2}, {51, 1, 2}, {9, 0, 2}, {9, 1, 0}, {9, 1, 255}} {
		if ok, err := s.feedPet(context.Background(), c, request[0], request[1], request[2]); ok || err != nil || !reflect.DeepEqual(before, *c.character) || wires[0].Len() != 0 {
			t.Fatal("bad shared feed inputs", request, ok, err)
		}
	}
	c.ready = false
	if err := s.dispatch(context.Background(), c, []byte{67, 1, 0x50, 0xc3}); !errors.Is(err, protocol.ErrMalformed) {
		t.Fatal("feeding during loading", err)
	}
	c.warped = true
	if err := s.dispatch(context.Background(), c, []byte{67, 1, 0x50, 0xc3}); err != nil {
		t.Fatal(err)
	}
	c.ready = true
	for _, set := range []func(){
		func() { c.battle = &battleRun{} }, func() { c.event = &eventSession{} },
		func() { c.storm = true }, func() { c.beach = &beachRun{} }, func() { c.trade = &tradeSession{} },
	} {
		set()
		if err := s.dispatch(context.Background(), c, []byte{67, 1, 0x50, 0xc3}); err != nil || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("locked feeding consumed item", err)
		}
		for _, packet := range wires[0].packets(t) {
			if len(packet) > 0 && packet[0] == 67 {
				t.Fatal("locked feeding emitted receipt", packet)
			}
		}
		c.battle, c.event, c.trade = nil, nil, nil
		c.storm = false
		c.beach = nil
	}
}
