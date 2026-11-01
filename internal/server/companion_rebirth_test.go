package server

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestCompanionAltarRebirthCommitAndReplay(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	c.character.Pets[1].Amity = 60
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	if err := s.openService(c, world.ServiceCompanionRebirth); err != nil {
		t.Fatal(err)
	}
	if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], []byte{44, 2}) {
		t.Fatal(got)
	}
	if err := s.dispatch(context.Background(), c, []byte{44, 1, 2}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) < 3 || !bytes.Equal(got[0], []byte{44, 1, 0}) {
		t.Fatal(got)
	}
	p := c.character.Pets[1]
	if !p.Reborn || p.Level != 1 || p.Exp != 0 || p.StatPoints != before.Pets[1].StatPoints || p.Amity != 60 || !reflect.DeepEqual(p.Skills, before.Pets[1].Skills) {
		t.Fatal(p)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || !reflect.DeepEqual(saved[0].Pets, c.character.Pets) {
		t.Fatal("durability", err)
	}
	if err := s.dispatch(context.Background(), c, []byte{44, 1, 2}); err != nil {
		t.Fatal(err)
	}
	if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], []byte{44, 1, 255}) {
		t.Fatal("replay", got)
	}
}

func TestCompanionAltarRebirthRejectionsAndRollback(t *testing.T) {
	s, c, wires := rebirthFixture(t)
	for _, tc := range []struct {
		mutate func(*game.Pet)
		code   byte
	}{
		{func(p *game.Pet) { p.Level = 99 }, 3},
		{func(p *game.Pet) { p.Reborn = true }, 6},
		{func(p *game.Pet) { p.Amity = 59 }, 7},
		{func(p *game.Pet) { p.Equipment[0] = game.Item{ID: 123, Count: 1} }, 8},
	} {
		original := c.character.Clone()
		tc.mutate(&c.character.Pets[1])
		before := c.character.Clone()
		c.view.CompanionRebirthOpen = true
		if err := s.dispatch(context.Background(), c, []byte{44, 1, 2}); err != nil {
			t.Fatal(err)
		}
		if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], []byte{44, 1, tc.code}) || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("rejection", got)
		}
		*c.character = original
	}
	before := c.character.Clone()
	s.Store.Close()
	if err := s.dispatch(context.Background(), c, []byte{44, 1, 2}); err == nil || wires[0].Len() != 0 || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("failed save published", err)
	}
}
