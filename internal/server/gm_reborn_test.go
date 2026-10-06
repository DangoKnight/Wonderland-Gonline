package server

import (
	"context"
	"strconv"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestGMRebornOwnershipCapeResetAndReplay(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	s.Assets.Items[25231] = game.ItemDefinition{ID: 25231, Type: 16, EquipSlot: 6}
	s.Assets.RebornClasses = []assets.RebornClass{{Job: game.JobKiller, Name: "Killer", CapeID: 25231, Enabled: true}}
	c.character.SetLevel(100)
	c.character.Refill(s.Assets.Items)
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	// The owning chat dispatcher checks GM authority before invoking the helper.
	c.gmLevel.Store(1)
	if err := s.gmReborn(ctx, c, "reborn", []string{"Killer"}); err != nil {
		t.Fatal(err)
	}
	if !c.character.Reborn || c.character.Job != 1 || c.character.RebornJob != 0 || c.character.Level != 1 || c.character.EXP != 0 || c.character.Bag[0].ID != 25231 {
		t.Fatal(c.character)
	}
	if !contains(wire.packets(t), []byte{5, 5, 17, 39, 0, 0, 146, 234}) {
		t.Fatal("reborn aura missing")
	}
	before := c.character.Clone()
	if err := s.gmReborn(ctx, c, "reborn", []string{"Killer"}); err != nil || c.character.Bag != before.Bag || c.character.Job != before.Job {
		t.Fatal("reborn replay", err)
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Job != 1 || saved[0].EXP != 0 {
		t.Fatal(saved, err)
	}
}

func TestGMRebornRejectsLowLevelAndFullBag(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(strconv.FormatBool(full), func(t *testing.T) {
			s, c, _, _ := mallFixture(t)
			ctx := context.Background()
			s.Assets.Items[25231] = game.ItemDefinition{ID: 25231, Type: 16, EquipSlot: 6}
			s.Assets.RebornClasses = []assets.RebornClass{{Job: game.JobKiller, Name: "Killer", CapeID: 25231, Enabled: true}}
			if full {
				c.character.SetLevel(100)
				for i := range c.character.Bag {
					c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
				}
			}
			if err := s.commit(ctx, c, c.character.Clone()); err != nil {
				t.Fatal(err)
			}
			before := c.character.Clone()
			if err := s.gmReborn(ctx, c, "reborn", []string{"Killer"}); err != nil {
				t.Fatal(err)
			}
			if c.character.Reborn || c.character.Job != 0 || c.character.Bag != before.Bag || c.character.Level != before.Level || c.character.EXP != before.EXP {
				t.Fatal("rejected rebirth changed character")
			}
			got, err := s.Store.Characters(ctx, c.account.ID)
			if err != nil || got[0].Reborn || got[0].Bag != before.Bag {
				t.Fatal("rejected rebirth changed SQL", err)
			}
		})
	}
}
