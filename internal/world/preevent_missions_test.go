package world

import (
	"testing"
	"wonderland-gonline/internal/assets"
)

func TestMissionVariablesAndPreEventGroups(t *testing.T) {
	w, c, v := eventWorld(), character(), NewView()
	v.MissionVariables[7] = 3
	if !w.condition(c, v, c.Map, nil, cond(19, 7, 1, 0, 5, 3)) || w.condition(c, v, c.Map, nil, cond(19, 7, 2, 0, 0, 0)) {
		t.Fatal("mission comparison")
	}
	v.Reset()
	if !w.condition(c, v, c.Map, nil, cond(19, 7, 2, 0, 0, 0)) {
		t.Fatal("mission survived scene reset")
	}
	ev := assets.Event{Branches: []assets.Branch{
		{Condition: cond(0, 0, 0, 0, 0, 0), Operations: []assets.Operation{actor(1, 3)}},
		{Condition: cond(2, 1, 2, 0, 4, 100)}, // Independent orphan, not part of rule 0.
	}}
	if r := rules(ev); len(r) != 1 || len(r[0].conditions) != 1 {
		t.Fatal("orphan attached", r)
	}
	ev.Branches[0].Condition[12] = 3
	if len(rules(ev)) != 0 {
		t.Fatal("truncated group authorized actions")
	}
	m := assets.Map{ID: 12345, NPCs: []assets.MapNPC{{ClickID: 1}}, PreEvents: []assets.Event{{Branches: []assets.Branch{{Condition: cond(19, 7, 1, 0, 5, 3), Operations: []assets.Operation{actor(1, 3)}}}}}}
	w = New(&assets.Catalog{Maps: map[uint16]assets.Map{m.ID: m}})
	if w.VisibleIn(c, v, m.ID, 1) {
		t.Fatal("unmet mission preevent matched")
	}
	v.MissionVariables[7] = 3
	if !w.VisibleIn(c, v, m.ID, 1) {
		t.Fatal("mission preevent ignored session counters")
	}
}
