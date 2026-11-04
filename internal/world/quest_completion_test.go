package world

import (
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestQuestCompletionRequiresReviewedEntireResourceTail(t *testing.T) {
	code := byte(1)
	c := &assets.Catalog{Items: map[uint16]game.ItemDefinition{30023: {ID: 30023}, 34058: {ID: 34058}}, Marks: map[uint16]uint16{13000: 0, 13001: 1}, Story: assets.StoryDefinitions{Rules: []assets.StoryRule{{ID: "soup", Kind: assets.StoryQuestCompletion, Scope: assets.StoryScope{Maps: []uint16{12050}, Events: []uint16{35}, Branches: []byte{11}}, Mark: 13000, Pattern: assets.StoryPattern{Code: &code}}}}}
	w := New(c)
	ev := &assets.Event{ClickID: 35, Branches: []assets.Branch{{Index: 11, Condition: [21]byte{5}, Operations: []assets.Operation{
		(assets.StoryOperation{Code: 1, D1: 1, D2: 1, D3: 30023, Value: 4294967295}).Operation(),
		(assets.StoryOperation{Code: 5, D1: 13000, D2: 2}).Operation(),
		(assets.StoryOperation{Code: 5, D1: 13001, D2: 1, Value: 1}).Operation(),
		(assets.StoryOperation{Code: 1, D1: 1, D2: 1, D3: 34058, Value: 1}).Operation(),
	}}}}
	plan, ok := w.QuestCompletion(12050, ev, 0, 0)
	if !ok || len(plan.Items) != 2 || plan.Items[0] != (game.ItemChange{ID: 30023, Count: -1}) || plan.Items[1] != (game.ItemChange{ID: 34058, Count: 1}) || len(plan.Marks) != 2 || !plan.Marks[0].PreserveStep {
		t.Fatal("native completion tail changed", plan, ok)
	}
	for _, mutate := range []func(*assets.Event){
		func(e *assets.Event) { e.ClickID++ },
		func(e *assets.Event) { e.Branches[0].Index++ },
		func(e *assets.Event) { e.Branches[0].Operations[1].Data[1]++ },
		func(e *assets.Event) { e.Branches[0].Operations[3].Data[0] = 8 },
		func(e *assets.Event) { e.Branches[0].Operations[3].Data[5] = 0 },
	} {
		copyEvent := *ev
		copyEvent.Branches = append([]assets.Branch(nil), ev.Branches...)
		copyEvent.Branches[0].Operations = append([]assets.Operation(nil), ev.Branches[0].Operations...)
		mutate(&copyEvent)
		if _, ok := w.QuestCompletion(12050, &copyEvent, 0, 0); ok {
			t.Fatal("unreviewed program accepted", copyEvent)
		}
	}
}
