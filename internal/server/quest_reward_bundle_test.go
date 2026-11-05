package server

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestQuestRewardBundleAtomicCompanionsSkillsAndReceipt(t *testing.T) {
	for _, scenario := range []string{"success", "invalid_checkpoint", "hotel_conflict", "cancelled", "receipt_failure", "already_trained"} {
		t.Run(scenario, func(t *testing.T) {
			s, c, _ := eventFixture(t)
			s.Assets.NPCs = map[uint16]assets.NPC{30178: {ID: 30178, Name: "Guest", Type: 7, Level: 1}, 30179: {ID: 30179, Name: "Returning", Type: 7, Level: 1}}
			s.Assets.Skills = map[uint16]assets.Skill{15998: {ID: 15998, TableOrder: 1}}
			s.Assets.Marks[13000], s.Assets.Marks[13001] = 0, 1
			code := byte(1)
			s.Assets.Story.Rules = append(s.Assets.Story.Rules, assets.StoryRule{ID: "bundle", Kind: assets.StoryQuestRewardBundle, Scope: assets.StoryScope{Maps: []uint16{10017}, Events: []uint16{90}, Branches: []byte{1}}, Mark: 13000, Pattern: assets.StoryPattern{Code: &code}})
			s.World = world.New(s.Assets)
			condition := evCond(5, 13000, 1, 1, 5, 2)
			condition[12] = 2
			ev := assets.Event{ClickID: 90, Branches: []assets.Branch{{Index: 1, Condition: condition, Operations: []assets.Operation{
				evOp(1, 1, 1, 1, 32176, 4294967295), evOp(2, 3, 1, 30178, 1, 0), evOp(3, 11, 15998, 1, 0, 1), evOp(4, 3, 2, 30179, 0, 0), evOp(5, 5, 13000, 2, 1, 0), evOp(6, 5, 13001, 1, 1, 1),
			}}, {Index: 2, Condition: evCond(5, 13001, 1, 1, 5, 0)}}}
			if scenario == "invalid_checkpoint" {
				ev.Branches[0].Operations[5] = evOp(6, 5, 13001, 1, 1, 256)
			}
			next := c.character.Clone()
			next.Bag[0] = game.Item{ID: 32176, Count: 1}
			next.Quests[13000] = game.Quest{ID: 13000, State: game.InProgress, Step: 2}
			next.Pets = []game.Pet{game.NewPet(30179, "Returning", 1, s.petTemplate(30179), s.Assets.Items)}
			next.ActivePet = 30179
			if scenario == "hotel_conflict" {
				next.HotelPets = []game.Pet{game.NewPet(30178, "Guest", 1, s.petTemplate(30178), s.Assets.Items)}
			}
			if scenario == "already_trained" {
				next.Skills = append(next.Skills, game.LearnedSkill{ID: 15998, Grade: 4, EXP: 9})
			}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			c.pets.register(30179)
			before := c.character.Clone()
			es := &eventSession{mapID: 10017, ev: &ev, branch: 0}
			c.event = es
			plan, ok := s.World.QuestCompletion(10017, &ev, 0, 0)
			if !ok {
				t.Fatal("native bundle not planned")
			}
			ctx := context.Background()
			if scenario == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if scenario == "receipt_failure" {
				c.conn = &failedWorldConn{}
			}
			err := s.completeNativeQuest(ctx, c, es, plan)
			fail := scenario == "invalid_checkpoint" || scenario == "hotel_conflict" || scenario == "cancelled"
			if fail {
				if err == nil || !reflect.DeepEqual(before, *c.character) || c.pets.slot(30178) != 0 || c.pets.slot(30179) == 0 {
					t.Fatal("failed bundle changed resources or presentation", err)
				}
			} else {
				if (err != nil) != (scenario == "receipt_failure") {
					t.Fatal("unexpected receipt result", err)
				}
				_, joined := c.character.Pet(30178)
				_, removed := c.character.Pet(30179)
				if !joined || removed || c.character.ActivePet != 0 || bagCount(c.character.Bag, 32176) != 0 || c.character.Quests[13001].Step != 1 || c.character.Quests[13000].State != game.Completed {
					t.Fatal("incomplete reward bundle", c.character.Quests)
				}
				skill := slices.IndexFunc(c.character.Skills, func(skill game.LearnedSkill) bool { return skill.ID == 15998 })
				if skill < 0 {
					t.Fatal("skill not learned")
				}
				if scenario == "already_trained" && c.character.Skills[skill] != (game.LearnedSkill{ID: 15998, Grade: 4, EXP: 9}) {
					t.Fatal("trained skill reset")
				}
				if s.World.FindBranch(c.character, c.view, 10017, &ev, world.TriggerEntry, 0, 0, -1) >= 0 {
					t.Fatal("bundle remained replayable")
				}
			}
			saved, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || saved[0].Bag != c.character.Bag || !reflect.DeepEqual(saved[0].Pets, c.character.Pets) || !reflect.DeepEqual(saved[0].Skills, c.character.Skills) || !reflect.DeepEqual(saved[0].Quests, c.character.Quests) {
				t.Fatal("bundle not durable", err)
			}
		})
	}
}

func TestInstalledQuestRewardBundleEndings(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name                                       string
		mapID, event, active, receipt, item, skill uint16
		branch, step, count                        byte
		pet                                        uint32
	}{
		{"collection", 11062, 2, 13108, 13109, 41007, 15983, 8, 1, 5, 0},
		{"xaolan_rejoins", 11052, 3, 13114, 13115, 0, 0, 24, 5, 0, 12068},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			s, c, _ := eventFixture(t)
			s.Assets, s.World = catalog, world.New(catalog)
			next := c.character.Clone()
			next.Map = fixture.mapID
			next.Quests[uint32(fixture.active)] = game.Quest{ID: uint32(fixture.active), State: game.InProgress, Step: int(fixture.step)}
			if fixture.item != 0 {
				next.Bag[0] = game.Item{ID: fixture.item, Count: fixture.count}
			}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			ev, known := s.World.Event(fixture.mapID, fixture.event)
			if !known {
				t.Fatal("missing native event")
			}
			branch := s.World.FindBranch(c.character, c.view, fixture.mapID, ev, world.TriggerEntry, 0, 0, -1)
			if branch < 0 || ev.Branches[branch].Index != fixture.branch {
				t.Fatal("native ending not eligible", branch)
			}
			var plan world.QuestCompletionPlan
			index, valid := 0, false
			for index = range ev.Branches[branch].Operations {
				plan, valid = s.World.QuestCompletion(fixture.mapID, ev, branch, index)
				if valid {
					break
				}
			}
			if !valid {
				t.Fatal("missing native reward bundle")
			}
			es := &eventSession{mapID: fixture.mapID, ev: ev, branch: branch, index: index}
			c.event = es
			if err := s.completeNativeQuest(context.Background(), c, es, plan); err != nil {
				t.Fatal(err)
			}
			if c.character.Quests[uint32(fixture.active)].State != game.Completed || c.character.Quests[uint32(fixture.receipt)].Step != 1 {
				t.Fatal("missing native completion receipts")
			}
			if fixture.item != 0 && bagCount(c.character.Bag, fixture.item) != 0 {
				t.Fatal("skill cost retained")
			}
			if fixture.skill != 0 && !slices.ContainsFunc(c.character.Skills, func(skill game.LearnedSkill) bool { return skill.ID == fixture.skill }) {
				t.Fatal("native skill not learned")
			}
			if fixture.pet != 0 {
				if _, joined := c.character.Pet(fixture.pet); !joined || c.pets.slot(fixture.pet) == 0 {
					t.Fatal("native companion not recruited")
				}
			}
			saved, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || saved[0].Bag != c.character.Bag || !reflect.DeepEqual(saved[0].Skills, c.character.Skills) || !reflect.DeepEqual(saved[0].Pets, c.character.Pets) || !reflect.DeepEqual(saved[0].Quests, c.character.Quests) {
				t.Fatal("native ending not durable", err)
			}
		})
	}
}
