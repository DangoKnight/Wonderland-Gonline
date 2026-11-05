package world

import (
	"testing"
	"wonderland-gonline/internal/assets"
)

func TestQuestRewardBundleNativeTail(t *testing.T) {
	code := byte(3)
	c := &assets.Catalog{NPCs: map[uint16]assets.NPC{30178: {ID: 30178}}, Skills: map[uint16]assets.Skill{15998: {ID: 15998}}, Marks: map[uint16]uint16{13000: 0}, Story: assets.StoryDefinitions{Rules: []assets.StoryRule{{ID: "bundle", Kind: assets.StoryQuestRewardBundle, Mark: 13000, Scope: assets.StoryScope{Maps: []uint16{10017}, Events: []uint16{90}, Branches: []byte{1}}, Pattern: assets.StoryPattern{Code: &code}}}}}
	ev := assets.Event{ClickID: 90, Branches: []assets.Branch{{Index: 1, Condition: [21]byte{5}, Operations: []assets.Operation{
		{Index: 1, Data: [21]byte{3, 1, 0, 226, 117, 1}},
		{Index: 2, Data: [21]byte{11, 126, 62, 1, 0, 0, 0, 0, 1}},
		{Index: 3, Data: [21]byte{5, 200, 50, 2, 0, 1}},
	}}}}
	plan, ok := New(c).QuestCompletion(10017, &ev, 0, 0)
	if !ok || len(plan.Companions) != 1 || plan.Companions[0] != (QuestCompanionReward{ID: 30178, Recruit: true}) || len(plan.Skills) != 1 || plan.Skills[0] != 15998 {
		t.Fatal("native companion/skill tail lost", plan)
	}
	c.Story.Rules[0].Kind = assets.StoryQuestCompletion
	if _, ok := New(c).QuestCompletion(10017, &ev, 0, 0); ok {
		t.Fatal("resource-only contract enabled extra effects")
	}
	c.Story.Rules[0].Kind = assets.StoryQuestRewardBundle
	delete(c.Skills, 15998)
	if _, ok := New(c).QuestCompletion(10017, &ev, 0, 0); ok {
		t.Fatal("missing skill allowed")
	}
}
