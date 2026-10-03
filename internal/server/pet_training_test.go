package server

import (
	"bytes"
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

func TestPetTrainingAllocationPacketsAndPersistence(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	ctx := context.Background()
	next := c.character.Clone()
	next.Pets[1].HP, next.Pets[1].SP = 17, 11
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	statIDs := []byte{28, 29, 27, 33, 30}
	baseValues := []uint32{40, 60, 30, 50, 70}
	for selector := byte(1); selector <= 5; selector++ {
		if err := s.dispatch(ctx, c, []byte{68, 2, 2, selector}); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		if len(got) < 2 || !bytes.Equal(got[len(got)-1], []byte{68, 2, 2, selector, 1}) || !contains(got, protocol.Builder{8, 2, 4, 2, 0, statIDs[selector-1], 1}.U32(baseValues[selector-1]+1).U32(0)) || !contains(got, protocol.Builder{8, 2, 4, 2, 0, 38, 1}.U32(uint32(7-selector)).U32(0)) {
			t.Fatal("training wire reply", selector, got)
		}
	}
	pet := c.character.Pets[1]
	if pet.Base != (game.Attributes{Strength: 41, Constitution: 61, Intelligence: 31, Wisdom: 51, Agility: 71}) || pet.StatPoints != 2 || pet.HP != 17 || pet.SP != 11 || pet.MaxHP <= before.Pets[1].MaxHP || pet.MaxSP <= before.Pets[1].MaxSP || pet.Slot != 3 || pet.Level != 100 || pet.Exp != 123 || pet.Amity != 100 || pet.Reborn || !reflect.DeepEqual(pet.Skills, before.Pets[1].Skills) || !reflect.DeepEqual(c.character.Pets[0], before.Pets[0]) {
		t.Fatal("wrong pet training state", pet)
	}
	if wires[1].Len() != 0 {
		t.Fatal("training sent peer packets")
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || len(saved) != 1 || !reflect.DeepEqual(saved[0].Pets, c.character.Pets) {
		t.Fatal("training not durable", err)
	}
	roster := newPetRoster()
	for _, p := range saved[0].Pets {
		roster.register(p.ID)
	}
	if !bytes.Equal(s.petListPacket(&saved[0], roster), s.petListPacket(c.character, c.pets)) {
		t.Fatal("reconnected training differs")
	}
	// Existing AC8 allocation spends the same budget; AC68 cannot grant extra points.
	for range 2 {
		if err := s.dispatch(ctx, c, []byte{8, 2, 2, 28, 1}); err != nil {
			t.Fatal(err)
		}
		wires[0].Reset()
	}
	exhausted := c.character.Clone()
	if err := s.dispatch(ctx, c, []byte{68, 2, 2, 1}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{68, 2, 2, 1, 0}) || !reflect.DeepEqual(exhausted, *c.character) {
		t.Fatal("spent exhausted budget", got)
	}
	// The newly ported rebirth bonus feeds that same point budget.
	if err := s.dispatch(ctx, c, []byte{69, 1, 2}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.dispatch(ctx, c, []byte{68, 2, 2, 3}); err != nil {
		t.Fatal(err)
	}
	got = wires[0].packets(t)
	if !c.character.Pets[1].Reborn || c.character.Pets[1].StatPoints != 49 || c.character.Pets[1].Base.Intelligence != 32 || !contains(got, protocol.Builder{8, 2, 4, 2, 0, 38, 1}.U32(49).U32(0)) {
		t.Fatal("rebirth points unavailable for training", got)
	}
	saved, err = s.Store.Characters(ctx, c.account.ID)
	if err != nil || !reflect.DeepEqual(saved[0].Pets, c.character.Pets) {
		t.Fatal("trained rebirth not durable", err)
	}
}

func TestPetTrainingRejectionsAndSaveFailure(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	before := c.character.Clone()
	for _, p := range [][]byte{{68}, {68, 2}, {68, 2, 2}, {68, 2, 2, 1, 0}, {68, 3, 2, 1}} {
		if err := s.dispatch(context.Background(), c, p); err == nil || wires[0].Len() != 0 || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("malformed training mutated state", p, err)
		}
	}
	for _, p := range [][]byte{{68, 2, 0, 1}, {68, 2, 4, 1}, {68, 2, 255, 1}, {68, 2, 2, 0}, {68, 2, 2, 6}, {68, 2, 2, 255}} {
		if err := s.dispatch(context.Background(), c, p); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		if len(got) != 1 || !bytes.Equal(got[0], append(append([]byte(nil), p...), 0)) || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("invalid selector or slot", p, got)
		}
	}
	if err := s.dispatch(context.Background(), c, []byte{68, 1, 2, 0}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) != 2 || !bytes.Equal(got[0], []byte{68, 1, 2, 0, 0}) || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("unfunded potential training accepted", got)
	}
	c.character.Pets[1].Base.Strength = math.MaxUint16
	overflow := c.character.Clone()
	if err := s.dispatch(context.Background(), c, []byte{68, 2, 2, 1}); err != nil {
		t.Fatal(err)
	}
	got = wires[0].packets(t)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{68, 2, 2, 1, 0}) || !reflect.DeepEqual(overflow, *c.character) {
		t.Fatal("training wrapped attribute", got)
	}
	*c.character = before
	s.Store.Close()
	if err := s.dispatch(context.Background(), c, []byte{68, 2, 2, 1}); err == nil || !reflect.DeepEqual(before, *c.character) || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("failed save published training", err)
	}
}

func TestPetTrainingRespectsWorldInteractionGates(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	before := c.character.Clone()
	c.ready = false
	if err := s.dispatch(context.Background(), c, []byte{68, 2, 2, 1}); !errors.Is(err, protocol.ErrMalformed) {
		t.Fatal("training while loading", err)
	}
	c.warped = true
	if err := s.dispatch(context.Background(), c, []byte{68, 2, 2, 1}); err != nil {
		t.Fatal(err)
	}
	c.ready = true
	for _, set := range []func(){
		func() { c.battle = &battleRun{} }, func() { c.event = &eventSession{} },
		func() { c.storm = true }, func() { c.beach = &beachRun{} }, func() { c.trade = &tradeSession{} },
	} {
		set()
		for _, command := range [][]byte{{68, 2, 2, 1}, {68, 1, 2, 0}} {
			if err := s.dispatch(context.Background(), c, command); err != nil || !reflect.DeepEqual(before, *c.character) {
				t.Fatal("locked training mutated state", err)
			}
			for _, packet := range wires[0].packets(t) {
				if len(packet) > 0 && packet[0] == 68 {
					t.Fatal("locked training emitted result", packet)
				}
			}
		}
		c.battle, c.event, c.trade = nil, nil, nil
		c.storm = false
		c.beach = nil
	}
}
