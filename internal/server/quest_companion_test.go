package server

import (
	"context"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestInstalledWaterPathEscort(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name        string
		event       uint16
		step        int
		owned, join bool
		branch      byte
		next        int
	}{
		{"join", 3, 2, false, true, 5, 3},
		{"reissue", 5, 3, false, true, 6, 3},
		{"return", 2, 3, true, false, 5, 4},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			s, c, _ := eventFixture(t)
			s.Assets, s.World = catalog, world.New(catalog)
			next := c.character.Clone()
			next.Map = 13118
			next.Quests[15148] = game.Quest{ID: 15148, State: game.InProgress, Step: scenario.step}
			next.Bag[0] = game.Item{ID: 48003, Count: 1}
			if scenario.owned {
				next.Pets = append(next.Pets, game.NewPet(30178, "Murloc", next.FreePetSlot(), s.petTemplate(30178), s.Assets.Items))
				c.pets.register(30178)
			}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			ev, ok := s.World.Event(13118, scenario.event)
			if !ok {
				t.Fatal("missing event")
			}
			branch := s.World.FindBranch(c.character, c.view, 13118, ev, world.TriggerEntry, 0, 0, -1)
			if branch < 0 || ev.Branches[branch].Index != scenario.branch {
				t.Fatal("wrong native branch", branch)
			}
			if err := s.startEvent(context.Background(), c, 1, ev, branch, false); err != nil {
				t.Fatal(err)
			}
			for steps := 0; c.event != nil && steps < 20; steps++ {
				if err := s.worldCommand(context.Background(), c, []byte{20, 6}); err != nil {
					t.Fatal(err)
				}
			}
			_, owned := c.character.Pet(30178)
			if c.event != nil || owned != scenario.join || c.character.Quests[15148].Step != scenario.next {
				t.Fatal("escort transition failed", c.character.Quests[15148], owned)
			}
			saved, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || (len(c.character.Pets) != 0 && !reflect.DeepEqual(saved[0].Pets, c.character.Pets)) || len(saved[0].Pets) != len(c.character.Pets) || !reflect.DeepEqual(saved[0].Quests, c.character.Quests) {
				t.Fatal("escort not durable", err)
			}
		})
	}
}

func TestQuestCompanionRollbackAndOwnership(t *testing.T) {
	for _, scenario := range []string{"success", "cancelled", "invalid_checkpoint", "stored_elsewhere", "duplicate"} {
		t.Run(scenario, func(t *testing.T) {
			s, c, _ := eventFixture(t)
			s.Assets.NPCs = map[uint16]assets.NPC{30178: {ID: 30178, Name: "Murloc", Level: 1}}
			s.Assets.Marks[15148] = 0
			code := byte(3)
			s.Assets.Story.Rules = append(s.Assets.Story.Rules, assets.StoryRule{ID: "escort", Kind: assets.StoryQuestCompanion, Template: 30178, Mark: 15148, Scope: assets.StoryScope{Maps: []uint16{10017}, Events: []uint16{90}, Branches: []byte{1}}, Pattern: assets.StoryPattern{Code: &code}})
			ev := assets.Event{ClickID: 90, Branches: []assets.Branch{{Index: 1, Condition: evCond(5, 15148, 1, 1, 5, 2), Operations: []assets.Operation{evOp(1, 3, 3, 30178, 1, 0), evOp(2, 5, 15148, 1, 1, 1)}}}}
			s.World = world.New(s.Assets)
			next := c.character.Clone()
			next.Quests[15148] = game.Quest{ID: 15148, State: game.InProgress, Step: 2}
			pet := game.NewPet(30178, "Murloc", 1, s.petTemplate(30178), s.Assets.Items)
			if scenario == "stored_elsewhere" {
				next.HotelPets = append(next.HotelPets, pet)
			}
			if scenario == "duplicate" {
				next.Pets = append(next.Pets, pet)
			}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			before := c.character.Clone()
			es := &eventSession{mapID: 10017, ev: &ev, branch: 0}
			c.event = es
			plan, ok := s.World.QuestCompanion(10017, &ev, 0, 0)
			if !ok {
				t.Fatal("missing plan")
			}
			ctx := context.Background()
			if scenario == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if scenario == "invalid_checkpoint" {
				plan.Marks[0].Value = 256
			}
			err := s.changeQuestCompanion(ctx, c, es, plan)
			if scenario == "success" {
				if err != nil || len(c.character.Pets) != 1 || c.character.Quests[15148].Step != 3 {
					t.Fatal("join failed", err)
				}
			} else if err == nil || !reflect.DeepEqual(before, *c.character) {
				t.Fatal("failed join mutated session", err)
			}
			saved, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || !reflect.DeepEqual(saved[0].Pets, c.character.Pets) || !reflect.DeepEqual(saved[0].Quests, c.character.Quests) {
				t.Fatal("non-atomic escort", err)
			}
		})
	}
}
