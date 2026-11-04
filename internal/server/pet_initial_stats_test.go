package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestRobinsonRecruitmentUsesInitialStatsAndRestoresReserveUnchanged(t *testing.T) {
	s, players, _ := petFixture(t)
	c := players[0]
	s.Assets.NPCs[12178] = assets.NPC{ID: 12178, Name: "Robinson", Level: 20, Type: 4, Element: game.Wind, Stats: [5]uint16{15, 20, 5, 8, 12}}
	s.Assets.PetInitialStats = map[uint32]game.PetInitialStats{12178: {Minimum: game.Attributes{Strength: 7, Constitution: 10, Intelligence: 2, Wisdom: 4, Agility: 6}, Maximum: game.Attributes{Strength: 7, Constitution: 11, Intelligence: 2, Wisdom: 5, Agility: 7}, TotalPoints: 30}}
	s.World = world.New(s.Assets)
	if ok, err := s.recruit(context.Background(), c, 12032); err != nil || !ok {
		t.Fatal(ok, err)
	}
	p := c.character.Pets[0]
	if p.Level != 1 || p.Potential != 0 || p.Base.Strength != 7 || p.Base.Intelligence != 2 || p.Base.Constitution < 10 || p.Base.Constitution > 11 || p.Base.Wisdom < 4 || p.Base.Wisdom > 5 || p.Base.Agility < 6 || p.Base.Agility > 7 {
		t.Fatal("wrong fresh Robinson", p)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Pets[0].Base != p.Base {
		t.Fatal("initial stats not saved", err)
	}
	next := c.character.Clone()
	next.Pets = nil
	p.Potential = 9
	p.Base.Strength = 17
	next.ReservePets = []game.Pet{p}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.recruit(context.Background(), c, 12032); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if c.character.Pets[0].Base != p.Base || c.character.Pets[0].Potential != 9 {
		t.Fatal("restored pet was rerolled")
	}
}
