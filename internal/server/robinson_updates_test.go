package server

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/world"
)

func TestRobinsonRecoveryCandidateScopeAndOrdering(t *testing.T) {
	for _, scenario := range []string{"rescue", "fresh", "completed", "other_map", "other_actor", "owned_npc_alias", "owned_pet_alias", "already_linked", "missing_event"} {
		t.Run(scenario, func(t *testing.T) {
			s, c, _ := eventFixture(t)
			c.character.Map = 10035
			c.character.Quests[12047] = game.Quest{ID: 12047, State: game.InProgress, Step: 1}
			greeting := assets.Event{ClickID: 8, Branches: []assets.Branch{{Index: 1}}}
			recruitment := assets.Event{ClickID: 19, Branches: []assets.Branch{{Index: 5}}}
			m := assets.Map{ID: 10035, Events: []assets.Event{greeting, recruitment}}
			click := uint16(1)
			switch scenario {
			case "fresh":
				delete(c.character.Quests, 12047)
			case "completed":
				c.character.Quests[12047] = game.Quest{ID: 12047, State: game.Completed}
			case "other_map":
				c.character.Map = 11016
			case "other_actor":
				click = 7
			case "owned_npc_alias":
				c.character.Pets = []game.Pet{{ID: 12032}}
			case "owned_pet_alias":
				c.character.Pets = []game.Pet{{ID: 12178}}
			case "missing_event":
				m.Events = m.Events[:1]
			}
			s.Assets.Maps[10035] = m
			s.World = world.New(s.Assets)
			greet, _ := s.World.Event(10035, 8)
			rescue, _ := s.World.Event(10035, 19)
			before := []*assets.Event{greet}
			if scenario == "already_linked" {
				before = append(before, rescue)
			}
			got := s.robinsonRecoveryEvents(c.character, click, before)
			if scenario == "rescue" {
				if len(got) != 2 || got[0] != rescue || got[1] != greet {
					t.Fatal("recovery priority", got)
				}
			} else if !reflect.DeepEqual(got, before) {
				t.Fatal("changed unrelated/existing candidate order", got)
			}
		})
	}
}

// The same unmodified SQL EVE scripts drive both original rescue maps. This
// reproduces upstream Test-RobinsonFullChain without its mocked Windows runtime.
func TestNativeRobinsonUpstreamFullChainAndRecovery(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, mapID := range []uint16{10035, 10039} {
		for _, resume := range []bool{false, true} {
			t.Run(fmt.Sprintf("map_%d_resume_%t", mapID, resume), func(t *testing.T) {
				s, c, wire := eventFixture(t)
				s.Assets, s.World = catalog, world.New(catalog)
				next := c.character.Clone()
				next.Map = mapID
				next.Bag = game.Inventory{}
				next.Quests = map[uint32]game.Quest{}
				next.Pets = nil
				if resume {
					next.Quests[12046] = game.Quest{ID: 12046, State: game.Completed, Step: 1}
					next.Quests[12047] = game.Quest{ID: 12047, State: game.InProgress, Step: 1}
				}
				if err := s.commit(context.Background(), c, next); err != nil {
					t.Fatal(err)
				}
				ev, ok := s.World.Event(mapID, 19)
				if !ok {
					t.Fatal("native recruitment event absent")
				}
				wire.Reset()
				if resume {
					handled, err := s.runNPCEvent(context.Background(), c, 1)
					if err != nil || !handled {
						t.Fatal("recovery click", handled, err)
					}
				} else {
					branch := s.World.FindBranch(c.character, c.view, mapID, ev, world.TriggerEntry, 0, 0, -1)
					if branch < 0 || ev.Branches[branch].Index != 1 {
						t.Fatal("initial native branch", branch)
					}
					if err := s.startEvent(context.Background(), c, 7, ev, branch, true); err != nil {
						t.Fatal(err)
					}
					if c.event == nil || c.event.ev.Branches[c.event.branch].Index != 3 || bagCount(c.character.Bag, 48016) != 1 {
						t.Fatal("raft-to-dialogue handoff")
					}
					for step := 0; c.event != nil && step < 32; step++ {
						if err := s.worldCommand(context.Background(), c, []byte{20, 6}); err != nil {
							t.Fatal(err)
						}
					}
				}
				if c.event != nil || len(c.character.Pets) != 1 || c.character.Pets[0].ID != 12032 || c.character.Quests[12046].State != game.Completed || c.character.Quests[12047].Step != 1 || c.character.Quests[15282].State != game.Completed || c.character.Quests[15283].Step != 1 {
					t.Fatal("recruitment chain incomplete", c.character.Quests, c.character.Pets)
				}
				if !c.view.Hidden[1] || s.World.Visible(c.character, mapID, 1) {
					t.Fatal("Robinson actor remained visible")
				}
				if !resume && bagCount(c.character.Bag, 48016) != 1 || resume && bagCount(c.character.Bag, 48016) != 0 {
					t.Fatal("raft redelivered during recovery")
				}
				saved, err := s.Store.Characters(context.Background(), c.account.ID)
				if err != nil || len(saved[0].Pets) != 1 || saved[0].Quests[15283].Step != 1 || saved[0].Bag != c.character.Bag {
					t.Fatal("recruitment not durable", err)
				}
				// Re-entering Event 19 cannot deliver another raft, pet or mark increment.
				if branch := s.World.FindBranch(c.character, c.view, mapID, ev, world.TriggerEntry, 0, 0, -1); branch >= 0 {
					t.Fatal("completed recruitment has a replayable branch", branch)
				}
			})
		}
	}
}

func TestNativeRobinsonRecoveryCapacityAndFailedSave(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"full_party", "failed_save"} {
		t.Run(scenario, func(t *testing.T) {
			s, c, wire := eventFixture(t)
			s.Assets, s.World = catalog, world.New(catalog)
			ctx := context.Background()
			next := c.character.Clone()
			next.Map = 10035
			next.Bag = game.Inventory{}
			next.Quests = map[uint32]game.Quest{12046: {ID: 12046, State: game.Completed, Step: 1}, 12047: {ID: 12047, State: game.InProgress, Step: 1}}
			if scenario == "full_party" {
				for i, id := range []uint32{14162, 14081, 14156, 14175} {
					next.Pets = append(next.Pets, game.NewPet(id, "Existing", byte(i+1), s.petTemplate(id), catalog.Items))
				}
			}
			if err := s.commit(ctx, c, next); err != nil {
				t.Fatal(err)
			}
			before := c.character.Clone()
			requestCtx := ctx
			if scenario == "failed_save" {
				var cancel context.CancelFunc
				requestCtx, cancel = context.WithCancel(ctx)
				cancel()
			}
			wire.Reset()
			handled, err := s.runNPCEvent(requestCtx, c, 1)
			if !handled || (scenario == "failed_save" && err == nil) || (scenario == "full_party" && err != nil) {
				t.Fatal("rejected recovery", handled, err)
			}
			if !reflect.DeepEqual(c.character.Quests, before.Quests) || !reflect.DeepEqual(c.character.Pets, before.Pets) || c.character.Bag != before.Bag {
				t.Fatal("failed recovery published rewards/progress")
			}
			saved, err := s.Store.Characters(ctx, c.account.ID)
			if err != nil || !reflect.DeepEqual(saved[0].Quests, before.Quests) || !reflect.DeepEqual(saved[0].Pets, before.Pets) {
				t.Fatal("failed recovery persisted changes", err)
			}
			if scenario == "full_party" {
				next = c.character.Clone()
				next.Pets = next.Pets[:game.MaxPets-1]
				if err := s.commit(ctx, c, next); err != nil {
					t.Fatal(err)
				}
			}
			s.endEvent(c)
			if handled, err := s.runNPCEvent(ctx, c, 1); !handled || err != nil {
				t.Fatal("recovery retry", handled, err)
			}
			if _, owned := c.character.Pet(12032); !owned || c.character.Quests[15283].Step != 1 || bagCount(c.character.Bag, 48016) != 0 {
				t.Fatal("retry did not recruit exactly once without a raft")
			}
		})
	}
}
