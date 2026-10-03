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

func rebirthFixture(t *testing.T) (*Server, *Session, []*captureConn) {
	t.Helper()
	s, c, wires := mountFixture(t)
	next := c.character.Clone()
	pet := &next.Pets[1] // Persistent slot 3 is client slot 2; Robinson alias 12178.
	pet.Level, pet.Amity, pet.Exp, pet.StatPoints = 100, 100, 123, 7
	pet.Base = game.Attributes{Strength: 40, Constitution: 60, Intelligence: 30, Wisdom: 50, Agility: 70}
	pet.Skills = []game.PetSkill{{ID: 11001, Grade: 5, Exp: 73}}
	pet.Battle = true
	pet.Normalize(s.Assets.Items, false)
	next.ActivePet = 12178
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	return s, c, wires
}

func TestPetRebirthPacketsPersistenceAndReconnect(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	before := c.character.Clone()
	if err := s.dispatch(context.Background(), c, []byte{69, 1, 2}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) < 3 || !bytes.Equal(got[0], []byte{69, 1, 2, 1}) || !bytes.Equal(got[1][:2], []byte{15, 8}) || !contains(got, protocol.Builder{8, 2, 4, 2, 0, 35, 1}.U32(1).U32(0)) || !contains(got, protocol.Builder{8, 2, 4, 2, 0, 38, 1}.U32(57).U32(0)) || !contains(got, protocol.Builder{8, 2, 4, 2, 0, 36, 1}.U32(6).U32(0)) {
		t.Fatal("rebirth wire refresh", got)
	}
	if !bytes.Equal(got[1][len(got[1])-11:], []byte{0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatal("reborn flag missing from roster", got[1])
	}
	pet := c.character.Pets[1]
	if !pet.Reborn || pet.Level != 1 || pet.Exp != 0 || pet.StatPoints != 57 || pet.HP != pet.MaxHP || pet.SP != pet.MaxSP || pet.Slot != 3 || c.character.ActivePet != 12178 || !pet.Battle || !reflect.DeepEqual(c.character.Pets[0], before.Pets[0]) {
		t.Fatal("wrong pet/state", c.character.Pets)
	}
	if wires[1].Len() != 0 {
		t.Fatal("unnecessary peer packet")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(saved) != 1 || !reflect.DeepEqual(saved[0].Pets, c.character.Pets) {
		t.Fatal("rebirth not saved", err)
	}
	roster := newPetRoster()
	restored := saved[0].Clone()
	for _, p := range restored.Pets {
		roster.register(p.ID)
	}
	if !bytes.Equal(s.petListPacket(&restored, roster), got[1]) {
		t.Fatal("reconnect rebirth list differs")
	}
	after := c.character.Clone()
	if err := s.dispatch(context.Background(), c, []byte{69, 1, 2}); err != nil {
		t.Fatal(err)
	}
	reply := wires[0].packets(t)
	if len(reply) == 0 || !bytes.Equal(reply[0], []byte{69, 1, 2, 0}) || !reflect.DeepEqual(after, *c.character) {
		t.Fatal("repeat ascension granted bonus", reply)
	}
}

func TestPetRebirthRejectsInvalidRequestsAndSaveFailure(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	for _, p := range [][]byte{{69}, {69, 1}, {69, 1, 2, 0}, {69, 2, 2}} {
		before := c.character.Clone()
		if err := s.dispatch(context.Background(), c, p); err == nil || !reflect.DeepEqual(before, *c.character) || wires[0].Len() != 0 {
			t.Fatal("bad request mutated state", p, err)
		}
	}
	for _, slot := range []byte{0, 4, 255} {
		if err := s.dispatch(context.Background(), c, []byte{69, 1, slot}); err != nil {
			t.Fatal(err)
		}
		if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], []byte{69, 1, slot, 0}) {
			t.Fatal("unknown client slot", got)
		}
	}
	for _, change := range []func(*game.Pet){
		func(p *game.Pet) { p.Level = 99 }, func(p *game.Pet) { p.Amity = 99 },
		func(p *game.Pet) { p.Reborn = true }, func(p *game.Pet) { p.StatPoints = math.MaxUint16 },
	} {
		original := c.character.Clone()
		change(&c.character.Pets[1])
		before := c.character.Clone()
		if err := s.dispatch(context.Background(), c, []byte{69, 1, 2}); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		if len(got) == 0 || !bytes.Equal(got[0], []byte{69, 1, 2, 0}) || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("ineligible pet ascended", got)
		}
		*c.character = original
	}
	before := c.character.Clone()
	s.Store.Close()
	if err := s.dispatch(context.Background(), c, []byte{69, 1, 2}); err == nil || !reflect.DeepEqual(before, *c.character) || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("failed save published rebirth", err)
	}
}

func TestPetRebirthRespectsWorldInteractionGates(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	before := c.character.Clone()
	c.ready = false
	if err := s.dispatch(context.Background(), c, []byte{69, 1, 2}); !errors.Is(err, protocol.ErrMalformed) {
		t.Fatal("loading rebirth accepted", err)
	}
	c.warped = true
	if err := s.dispatch(context.Background(), c, []byte{69, 1, 2}); err != nil {
		t.Fatal(err)
	}
	c.ready = true
	for _, set := range []func(){
		func() { c.battle = &battleRun{} }, func() { c.event = &eventSession{} },
		func() { c.storm = true }, func() { c.beach = &beachRun{} }, func() { c.trade = &tradeSession{} },
	} {
		set()
		if err := s.dispatch(context.Background(), c, []byte{69, 1, 2}); err != nil || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("locked interaction rebirth", err)
		}
		for _, packet := range wires[0].packets(t) {
			if len(packet) > 0 && packet[0] == 69 {
				t.Fatal("locked interaction emitted rebirth result", packet)
			}
		}
		c.battle, c.event, c.trade = nil, nil, nil
		c.storm = false
		c.beach = nil
	}
}
