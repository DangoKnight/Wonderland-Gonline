package world

import (
	"testing"
	"wonderland-gonline/internal/assets"
)

func TestQuestCompanionRequiresEntireReviewedTail(t *testing.T) {
	// Native EVE bytes are independent of the operation builder and executor.
	join := assets.Operation{Index: 2, Data: [21]byte{3, 3, 0, 226, 117, 1, 0, 0, 0, 0, 0, 0, 12}}
	hide := assets.Operation{Index: 3, Data: [21]byte{2, 1, 0, 2, 0, 1, 0, 0, 255, 255, 0, 0, 12}}
	mark := assets.Operation{Index: 4, Data: [21]byte{5, 44, 59, 1, 0, 1, 0, 0, 1, 0, 0, 0, 12}}
	code := byte(3)
	c := &assets.Catalog{NPCs: map[uint16]assets.NPC{30178: {ID: 30178, Name: "Murloc"}}, Story: assets.StoryDefinitions{Rules: []assets.StoryRule{{ID: "escort", Kind: assets.StoryQuestCompanion, Template: 30178, Mark: 15148, Scope: assets.StoryScope{Maps: []uint16{13118}, Events: []uint16{3}, Branches: []byte{5}}, Pattern: assets.StoryPattern{Code: &code}}}}}
	w := New(c)
	ev := assets.Event{ClickID: 3, Branches: []assets.Branch{{Index: 5, Condition: [21]byte{5}, Operations: []assets.Operation{join, hide, mark}}}}
	plan, ok := w.QuestCompanion(13118, &ev, 0, 0)
	if !ok || !plan.Join || plan.Template != 30178 || len(plan.Marks) != 1 || len(plan.Actors) != 1 || !DecodeOp(join).Unsupported() {
		t.Fatal("scoped native escort changed", plan, ok)
	}
	for _, change := range []func(*assets.Event){
		func(e *assets.Event) { e.ClickID++ },
		func(e *assets.Event) { e.Branches[0].Operations[0].Data[3]++ },
		func(e *assets.Event) { e.Branches[0].Operations[0].Data[5] = 2 },
		func(e *assets.Event) { e.Branches[0].Operations[1].Data[0] = 8 },
		func(e *assets.Event) { e.Branches[0].Operations[2].Data[1]++ },
		func(e *assets.Event) { e.Branches[0].Operations = append(e.Branches[0].Operations, join) },
	} {
		copy := ev
		copy.Branches = append([]assets.Branch(nil), ev.Branches...)
		copy.Branches[0].Operations = append([]assets.Operation(nil), ev.Branches[0].Operations...)
		change(&copy)
		if _, ok := w.QuestCompanion(13118, &copy, 0, 0); ok || w.QuestCompanionAction(13118, &copy, 0, 0) {
			t.Fatal("unreviewed escort accepted", copy)
		}
	}
}
