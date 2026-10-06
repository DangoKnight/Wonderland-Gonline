package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

// These scenarios use the exported native scripts in SQL, without editing them.
func TestNativeFredFinaleCompanionRewards(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, following := range []bool{false, true} {
		name := "not_following"
		if following {
			name = "following"
		}
		t.Run(name, func(t *testing.T) {
			s, c, wire := eventFixture(t)
			s.Assets, s.World = catalog, world.New(catalog)
			next := c.character.Clone()
			next.Map = 11149
			next.Quests[13172] = game.Quest{ID: 13172, State: game.InProgress, Step: 4}
			next.Pets = []game.Pet{game.NewPet(14162, "Roca", 1, s.petTemplate(14162), catalog.Items)}
			next.ReservePets = []game.Pet{game.NewPet(14081, "Niss", 1, s.petTemplate(14081), catalog.Items)}
			if following {
				next.ActivePet = 14162
			}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			c.pets.register(14162)
			ev, ok := s.World.Event(11149, 2)
			if !ok {
				t.Fatal("native finale absent")
			}
			branch := s.World.FindBranch(c.character, c.view, 11149, ev, world.TriggerEntry, 0, 0, -1)
			wantBranch := byte(10)
			if following {
				wantBranch = 20
			}
			if branch < 0 || ev.Branches[branch].Index != wantBranch {
				t.Fatal("native finale gate", branch)
			}
			wire.Reset()
			if err := s.startEvent(context.Background(), c, 2, ev, branch, true); err != nil {
				t.Fatal(err)
			}
			sawStar := false
			for i := 0; c.event != nil && i < 80; i++ {
				packets := wire.packets(t)
				if contains(packets, []byte{15, 20, 6, 3}) {
					sawStar = true
					if c.character.Quests[13173].Step != 0 || bagCount(c.character.Bag, 34064) != 0 {
						t.Fatal("reward before constellation completes")
					}
				}
				if c.event.onChoice != nil || c.event.onMinigame != nil || c.battle != nil {
					t.Fatal("unexpected native finale barrier")
				}
				if err := s.worldCommand(context.Background(), c, []byte{20, 6}); err != nil {
					t.Fatal(err)
				}
			}
			if !sawStar || c.event != nil || c.character.Quests[13172].State != game.Completed || c.character.Quests[13173].Step != 1 || bagCount(c.character.Bag, 34064) != 1 {
				t.Fatal("native finale incomplete", sawStar, c.event, c.character.Quests[13173], bagCount(c.character.Bag, 34064))
			}
			if _, ok := c.character.Pet(12095); !ok {
				t.Fatal("Fred not recruited")
			}
			if len(c.character.ReservePets) != 1 || c.character.ReservePets[0].ID != 14081 {
				t.Fatal("Niss reserve lost")
			}
			chars, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || chars[0].Quests[13173].Step != 1 || bagCount(chars[0].Bag, 34064) != 1 || len(chars[0].Pets) != 2 {
				t.Fatal("finale not durable", err)
			}
		})
	}
}

func TestNativeMonkeyRescueController(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, c, _ := eventFixture(t)
	s.Assets, s.World = catalog, world.New(catalog)
	next := c.character.Clone()
	next.Map = 11016
	npc, ok := s.World.NPC(11016, 1)
	if !ok {
		t.Fatal("native Monkey actor absent")
	}
	next.X, next.Y = npc.X, npc.Y
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	c.view = world.NewView()
	if err := s.worldCommand(context.Background(), c, []byte{20, 1, 1, 0}); err != nil {
		t.Fatal(err)
	}
	for i := 0; c.event != nil && i < 20; i++ {
		if err := s.worldCommand(context.Background(), c, []byte{20, 6}); err != nil {
			t.Fatal(err)
		}
	}
	if c.event != nil || c.character.Quests[12002].State != game.Completed {
		t.Fatal("native Monkey rescue incomplete")
	}
	if _, ok := c.character.Pet(17162); !ok {
		t.Fatal("native Monkey absent")
	}
}

func TestNativeElinReplacementWeaponConditions(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, c, _ := eventFixture(t)
	s.Assets, s.World = catalog, world.New(catalog)
	native, ok := catalog.Maps[11167]
	if !ok {
		t.Fatal("native Elin weapon map absent")
	}
	matched := 0
	for _, ev := range native.Events {
		for _, br := range ev.Branches {
			rule := world.DecodeCond(br.Condition)
			if rule.Kind != 17 {
				continue
			}
			matched++
			copy := ev
			positive := &assets.Event{ClickID: copy.ClickID, Branches: []assets.Branch{{Index: 1, Condition: br.Condition, Operations: []assets.Operation{evOp(1, 2, 1, 0, 10000, 0)}}}}
			weapon := rule.W3
			if byte(rule.W4) == world.CompareNotEqual {
				weapon = 0
			}
			c.character.Pets = []game.Pet{{ID: 14230, Slot: 1, Equipment: game.Equipment{2: {ID: weapon, Count: 1}}}}
			if got := s.World.FindBranch(c.character, c.view, 11167, positive, 0, 0, 0, -1); got != 0 {
				t.Fatal("native equipped weapon gate", rule, got)
			}
			c.character.Pets = nil
			if got := s.World.FindBranch(c.character, c.view, 11167, positive, 0, 0, 0, -1); got >= 0 {
				t.Fatal("native missing Elin gate", rule, got)
			}
		}
	}
	if matched != 3 {
		t.Fatal("native Elin gate census", matched)
	}
}
