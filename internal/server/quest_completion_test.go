package server

import (
	"context"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestNativeQuestCompletionCommitsResourcesAndMarksTogether(t *testing.T) {
	for _, failure := range []string{"success", "cancelled", "invalid_checkpoint", "receipt_failure"} {
		t.Run(failure, func(t *testing.T) {
			receiptFailure := failure == "receipt_failure"
			fail := failure != "success" && !receiptFailure
			s, c, wire := eventFixture(t)
			s.Assets.Items[30023] = game.ItemDefinition{ID: 30023}
			s.Assets.Items[34058] = game.ItemDefinition{ID: 34058}
			s.Assets.Marks[13000], s.Assets.Marks[13001] = 0, 1
			code := byte(1)
			s.Assets.Story.Rules = append(s.Assets.Story.Rules, assets.StoryRule{ID: "atomic_test", Kind: assets.StoryQuestCompletion, Scope: assets.StoryScope{Maps: []uint16{10017}, Events: []uint16{80}, Branches: []byte{1}}, Mark: 13000, Pattern: assets.StoryPattern{Code: &code}})
			condition := evCond(5, 13000, 1, 1, 5, 2)
			condition[12] = 2 // Native header plus one condition-only receipt guard.
			ev := assets.Event{ClickID: 80, Branches: []assets.Branch{{Index: 1, Condition: condition, Operations: []assets.Operation{
				evOp(1, 1, 1, 1, 30023, 4294967295), evOp(2, 5, 13000, 2, 1, 0), evOp(3, 5, 13001, 1, 1, 1), evOp(4, 1, 1, 1, 34058, 1),
				evOp(5, 1, 1, 2, 0, 25), evOp(6, 1, 1, 5, 1, 1),
			}}, {Index: 2, Condition: evCond(5, 13001, 1, 1, 5, 0)}}}
			if failure == "invalid_checkpoint" {
				// Failure after item application must still roll back every field.
				ev.Branches[0].Operations[2] = evOp(3, 5, 13001, 1, 1, 256)
			}
			m := s.Assets.Maps[10017]
			m.Events = append(m.Events, ev)
			s.Assets.Maps[10017] = m
			s.World = world.New(s.Assets)
			next := c.character.Clone()
			next.Bag[0] = game.Item{ID: 30023, Count: 1}
			next.Quests[13000] = game.Quest{ID: 13000, State: game.InProgress, Step: 2}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			before := c.character.Clone()
			ctx := context.Background()
			if failure == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			wire.Reset()
			var err error
			if receiptFailure {
				es := &eventSession{mapID: 10017, ev: &ev, branch: 0}
				c.event = es
				plan, ok := s.World.QuestCompletion(10017, &ev, 0, 0)
				if !ok {
					t.Fatal("missing native completion plan")
				}
				c.conn = &failedWorldConn{}
				err = s.completeNativeQuest(ctx, c, es, plan)
			} else {
				err = s.startEvent(ctx, c, 5, &ev, 0, false)
			}
			if fail {
				if err == nil || !reflect.DeepEqual(before, *c.character) {
					t.Fatal("failed completion mutated session", err)
				}
			} else if (err != nil) != receiptFailure || bagCount(c.character.Bag, 30023) != 0 || bagCount(c.character.Bag, 34058) != 1 || c.character.Quests[13000].State != game.Completed || c.character.Quests[13001].Step != 1 || c.character.Gold != before.Gold+25 || c.character.EXP == before.EXP && c.character.Level == before.Level {
				t.Fatal("completion not delivered", c.character.Bag, c.character.Quests, err)
			}
			stored, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || stored[0].Bag != c.character.Bag || stored[0].Gold != c.character.Gold || stored[0].EXP != c.character.EXP || stored[0].Level != c.character.Level || !reflect.DeepEqual(stored[0].Quests, c.character.Quests) {
				t.Fatal("completion not atomic/durable", err)
			}
			if !fail && s.World.FindBranch(c.character, c.view, 10017, &ev, world.TriggerEntry, 0, 0, -1) >= 0 {
				t.Fatal("completion remains replayable")
			}
		})
	}
}

func TestNativeQuestCompletionRejectsStaleCheckpoint(t *testing.T) {
	s, c, _ := eventFixture(t)
	s.Assets.Marks[13000] = 0
	code := byte(1)
	s.Assets.Story.Rules = append(s.Assets.Story.Rules, assets.StoryRule{ID: "stale_checkpoint", Kind: assets.StoryQuestCompletion, Scope: assets.StoryScope{Maps: []uint16{10017}, Events: []uint16{90}, Branches: []byte{1}}, Mark: 13000, Pattern: assets.StoryPattern{Code: &code}})
	s.World = world.New(s.Assets)
	ev := assets.Event{ClickID: 90, Branches: []assets.Branch{{Index: 1, Condition: evCond(5, 13000, 1, 1, 5, 2), Operations: []assets.Operation{evOp(1, 1, 1, 2, 0, 25), evOp(2, 5, 13000, 2, 1, 0)}}}}
	next := c.character.Clone()
	next.Quests[13000] = game.Quest{ID: 13000, State: game.InProgress, Step: 2}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(current *game.Character) error {
		q := current.Quests[13000]
		q.State = game.Completed
		current.Quests[13000] = q
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	es := &eventSession{mapID: 10017, ev: &ev, branch: 0}
	c.event = es
	plan, ok := s.World.QuestCompletion(10017, &ev, 0, 0)
	if !ok {
		t.Fatal("missing completion plan")
	}
	if err := s.completeNativeQuest(context.Background(), c, es, plan); err == nil {
		t.Fatal("stale checkpoint accepted")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Gold != next.Gold || saved[0].Quests[13000].State != game.Completed || c.character.Gold != next.Gold {
		t.Fatal("stale session overwrote durable receipt", err)
	}
}

func TestInstalledWikiQuestCompletionRewards(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name                                                   string
		mapID, event, actor, active, receipt, consumed, reward uint16
		branch                                                 byte
		count                                                  int
	}{
		{"auntie_soup", 12050, 35, 4, 13000, 13001, 30023, 34058, 11, 1},
		{"dinner_incident", 12000, 14, 12, 12018, 12019, 30028, 34076, 10, 1},
		{"squirrel_mission", 12050, 37, 20, 12012, 12013, 0, 41141, 15, 12},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			s, c, _ := eventFixture(t)
			s.Assets, s.World = catalog, world.New(catalog)
			next := c.character.Clone()
			next.Map = scenario.mapID
			next.Quests[uint32(scenario.active)] = game.Quest{ID: uint32(scenario.active), State: game.InProgress, Step: 2}
			if scenario.consumed != 0 {
				next.Bag[0] = game.Item{ID: scenario.consumed, Count: 1}
			}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			ev, ok := s.World.Event(scenario.mapID, scenario.event)
			if !ok {
				t.Fatal("native event absent")
			}
			branch := s.World.FindBranch(c.character, c.view, scenario.mapID, ev, world.TriggerEntry, 0, 0, -1)
			if branch < 0 || ev.Branches[branch].Index != scenario.branch {
				t.Fatal("native completion not eligible", branch)
			}
			if err := s.startEvent(context.Background(), c, scenario.actor, ev, branch, false); err != nil {
				t.Fatal(err)
			}
			for steps := 0; c.event != nil && steps < 32; steps++ {
				if c.event.onChoice != nil || c.event.onMinigame != nil || c.battle != nil {
					t.Fatal("unexpected completion barrier")
				}
				if err := s.worldCommand(context.Background(), c, []byte{20, 6}); err != nil {
					t.Fatal(err)
				}
			}
			if c.event != nil || bagCount(c.character.Bag, scenario.reward) != scenario.count || c.character.Quests[uint32(scenario.active)].State != game.Completed || c.character.Quests[uint32(scenario.receipt)].Step != 1 {
				t.Fatal("native completion failed", c.character.Quests, bagCount(c.character.Bag, scenario.reward))
			}
			if scenario.consumed != 0 && bagCount(c.character.Bag, scenario.consumed) != 0 {
				t.Fatal("quest material retained")
			}
			saved, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || saved[0].Bag != c.character.Bag || !reflect.DeepEqual(saved[0].Quests, c.character.Quests) {
				t.Fatal("native completion not durable", err)
			}
			again := s.World.FindBranch(c.character, c.view, scenario.mapID, ev, world.TriggerEntry, 0, 0, -1)
			if again >= 0 && ev.Branches[again].Index == scenario.branch {
				t.Fatal("native reward completion still eligible")
			}
		})
	}
}

func TestInstalledWikiQuestContracts(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := assets.ValidateStoryCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	w := world.New(catalog)
	character := &game.Character{Quests: map[uint32]game.Quest{}}
	counts := map[string]int{}
	for _, rule := range catalog.Story.Rules {
		if rule.Kind != assets.StoryQuestCompletion && rule.Kind != assets.StoryQuestCompanion && rule.Kind != assets.StoryQuestRewardBundle {
			continue
		}
		counts[rule.Kind]++
		mid, eid, bid := rule.Scope.Maps[0], rule.Scope.Events[0], rule.Scope.Branches[0]
		ev, ok := w.Event(mid, eid)
		if !ok {
			t.Fatalf("%s: missing event", rule.ID)
		}
		branch := -1
		for i, b := range ev.Branches {
			if b.Index == bid {
				branch = i
			}
		}
		if branch < 0 {
			t.Fatalf("%s: missing branch", rule.ID)
		}
		for _, program := range []*assets.Event{ev, w.PrepareStory(character, nil, mid, ev, branch)} {
			valid := false
			if rule.Kind == assets.StoryQuestCompletion || rule.Kind == assets.StoryQuestRewardBundle {
				_, valid = w.QuestCompletion(mid, program, branch, int(rule.Value))
			} else {
				_, valid = w.QuestCompanion(mid, program, branch, int(rule.Value))
			}
			if !valid {
				t.Fatalf("%s: native/prepared contract mismatch", rule.ID)
			}
		}
	}
	if counts[assets.StoryQuestCompletion] != 350 || counts[assets.StoryQuestCompanion] != 15 || counts[assets.StoryQuestRewardBundle] != 25 {
		t.Fatal("incomplete installed quest profile", counts)
	}
}
