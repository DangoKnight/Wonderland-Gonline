package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestNativeRebirthJobSelectionAndMirror(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	s.Assets.RebornClasses = []assets.RebornClass{{Job: 1, Name: "Killer", CapeID: 25231, Enabled: true}, {Job: 3, Name: "Knight", CapeID: 25233, Enabled: true}}
	for _, id := range []uint16{15241, 15242} {
		s.Assets.Skills[id] = assets.Skill{ID: id, TableOrder: id}
	}
	s.Assets.Items[34018] = game.ItemDefinition{ID: 34018, Type: 10}
	s.Assets.Items[25233] = game.ItemDefinition{ID: 25233, Type: 16}
	c.character.SetLevel(120)
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, c, []byte{79, 1, 1})
	if c.character.Reborn {
		t.Fatal("unsolicited rebirth")
	}
	if err := s.openService(c, world.ServiceRebornJobSelection); err != nil {
		t.Fatal(err)
	}
	if !contains(wire.packets(t), []byte{79, 1}) {
		t.Fatal("form missing")
	}
	tradeDo(t, s, c, []byte{79, 1, 1})
	if !c.character.Reborn || c.character.Level != 21 || c.character.Job != 1 || c.character.RebornJob != 1 || !hasLearned(*c.character, 15241) {
		t.Fatal("rebirth state")
	}
	c.character.Bag[0] = game.Item{ID: 34018, Count: 2}
	c.character.Bag[1] = game.Item{ID: 25233, Count: 1}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, c, []byte{79, 2, 3, 1, 2})
	if c.character.Job != 3 || c.character.Bag[0].Count != 1 || c.character.Bag[1].ID != 25233 || !hasLearned(*c.character, 15242) {
		t.Fatal("mirror exchange")
	}
	tradeDo(t, s, c, []byte{79, 2, 3, 1, 2})
	if c.character.Bag[0].Count != 1 {
		t.Fatal("mirror replay consumed twice")
	}
}

func hasLearned(c game.Character, id uint16) bool {
	for _, sk := range c.Skills {
		if sk.ID == id {
			return true
		}
	}
	return false
}
