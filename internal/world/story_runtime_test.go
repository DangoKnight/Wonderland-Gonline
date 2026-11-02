package world

import (
	"fmt"
	"reflect"
	"slices"
	"sync"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestStoryIteratorPreservesPrecedenceAndSelection(t *testing.T) {
	gate := []assets.StoryGate{{Source: "mark_step", ID: 100000, Op: "eq", Value: 3}}
	rules := []assets.StoryRule{
		{ID: "global_first", Kind: assets.StoryClickMark, Gates: gate},
		{ID: "local", Kind: assets.StoryClickMark, Scope: assets.StoryScope{Maps: []uint16{65001}}, Gates: gate},
		{ID: "foreign", Kind: assets.StoryClickMark, Scope: assets.StoryScope{Maps: []uint16{65002}}},
		{ID: "global_last", Kind: assets.StoryClickMark, Gates: gate},
	}
	w := New(&assets.Catalog{Story: assets.StoryDefinitions{Rules: rules}})
	c := character(game.Quest{ID: 100000, State: game.InProgress, Step: 3})
	x := StoryContext{Map: 65001}
	if first, ok := w.StoryFirst(assets.StoryClickMark, c, nil, x); !ok || first.ID != "global_first" {
		t.Fatal("global precedence", first, ok)
	}
	var got []string
	for r := range w.StoryRules(assets.StoryClickMark, c, nil, x) {
		got = append(got, r.ID)
		// The first action must not change which later actions were selected.
		c.Quests[100000] = game.Quest{ID: 100000, State: game.Completed, Step: 4}
	}
	if !slices.Equal(got, []string{"global_first", "local", "global_last"}) {
		t.Fatal("selection/order changed during actions", got)
	}
	if _, ok := w.StoryFirst(assets.StoryClickMark, c, nil, x); ok {
		t.Fatal("player predicate result was cached")
	}
	c.Quests[100000] = game.Quest{ID: 100000, State: game.InProgress, Step: 3}
	got = nil
	for r := range w.StoryRules(assets.StoryClickMark, c, nil, StoryContext{Map: 65003}) {
		got = append(got, r.ID)
	}
	if !slices.Equal(got, []string{"global_first", "global_last"}) {
		t.Fatal("global fallback", got)
	}
	selected := w.StoryRules(assets.StoryClickMark, c, nil, x)
	w.ReloadStory(assets.StoryDefinitions{})
	if len(slices.Collect(selected)) != 3 || len(slices.Collect(w.StoryRules(assets.StoryClickMark, c, nil, x))) != 0 {
		t.Fatal("iterator did not retain its definitions or reload did not clear them")
	}
}

func TestStoryIteratorLargeSelectionAndEarlyStop(t *testing.T) {
	rules := make([]assets.StoryRule, 300)
	for i := range rules {
		rules[i] = assets.StoryRule{ID: fmt.Sprint(i), Kind: assets.StoryVisibility, Value: int64(i)}
	}
	w := New(&assets.Catalog{Story: assets.StoryDefinitions{Rules: rules}})
	got := slices.Collect(w.StoryRules(assets.StoryVisibility, nil, nil, StoryContext{Map: 1}))
	if len(got) != len(rules) || got[len(got)-1].Value != 299 {
		t.Fatal("large selection truncated or reordered")
	}
	count := 0
	for range w.StoryRules(assets.StoryVisibility, nil, nil, StoryContext{Map: 1}) {
		count++
		break
	}
	if count != 1 {
		t.Fatal("iterator ignored early stop")
	}
}

func TestStoryCompiledGatesRetainGroupingAndFreshState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		gates []assets.StoryGate
		want  bool
	}{
		{"empty", nil, true},
		{"interleaved_groups", []assets.StoryGate{
			{Source: "mark_state", ID: 100000, Op: "eq", Value: 1, Group: 65535},
			{Source: "active_vehicle", Op: "eq", Value: 99, Group: 0},
			{Source: "mark_step", ID: 100000, Op: "ge", Value: 3, Group: 65535},
		}, true},
		{"failed_and", []assets.StoryGate{
			{Source: "mark_state", ID: 100000, Op: "eq", Value: 1},
			{Source: "mark_step", ID: 100000, Op: "lt", Value: 3},
		}, false},
		{"alternative", []assets.StoryGate{
			{Source: "mark_step", ID: 100000, Op: "gt", Value: 3, Group: 1},
			{Source: "mark_exists", ID: 100000, Op: "ne", Value: 0, Group: 2},
		}, true},
		{"invalid_source_fails_closed", []assets.StoryGate{
			{Source: "mark_step", ID: 100000, Op: "eq", Value: 3, Group: 1},
			{Source: "unknown", Op: "eq", Value: 0, Group: 2},
		}, false},
		{"invalid_comparison_group", []assets.StoryGate{
			{Source: "mark_step", ID: 100000, Op: "unknown", Value: 3, Group: 1},
			{Source: "mark_step", ID: 100000, Op: "le", Value: 3, Group: 2},
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := assets.StoryDefinitions{
				Rules:           []assets.StoryRule{{Kind: assets.StoryBlock, Gates: tc.gates}},
				Transformations: []assets.StoryTransformation{{Gates: slices.Clone(tc.gates)}},
			}
			w := New(&assets.Catalog{Story: d})
			c := character(game.Quest{ID: 100000, State: game.InProgress, Step: 3})
			_, got := w.StoryFirst(assets.StoryBlock, c, nil, StoryContext{})
			if got != tc.want {
				t.Fatal("compiled rules", got)
			}
			for _, gates := range [][]assets.StoryGate{d.Transformations[0].Gates, slices.Clone(tc.gates)} {
				if w.StoryGates(gates, c, nil, StoryContext{}) != tc.want {
					t.Fatal("transformation or ad hoc gate mismatch")
				}
				if w.StoryGates(gates, nil, nil, StoryContext{}) != (len(gates) == 0) {
					t.Fatal("missing player gate")
				}
			}
		})
	}
	gates := []assets.StoryGate{{Source: "active_mark_step", ID: 100000, Op: "eq", Value: 3}}
	w := New(&assets.Catalog{Story: assets.StoryDefinitions{Transformations: []assets.StoryTransformation{{Gates: gates}}}})
	c := character(game.Quest{ID: 100000, State: game.InProgress, Step: 3})
	if !w.StoryGates(gates, c, nil, StoryContext{}) {
		t.Fatal("fresh active mark rejected")
	}
	c.Quests[100000] = game.Quest{ID: 100000, State: game.Completed, Step: 3}
	if w.StoryGates(gates, c, nil, StoryContext{}) {
		t.Fatal("transaction recheck used cached player result")
	}
}

func TestStoryRepairCacheIsolationAndReload(t *testing.T) {
	event := assets.Event{ClickID: 1, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{{Data: [21]byte{2}}}}}}
	rule := assets.StoryRule{ID: "prepend", Kind: assets.StoryRepair, Scope: assets.StoryScope{Maps: []uint16{65001}}, Edits: []assets.StoryEdit{{Kind: "prepend", Operations: []assets.StoryOperation{{Code: 5, D1: 9}}}}}
	d := assets.StoryDefinitions{Rules: []assets.StoryRule{rule}}
	w := New(&assets.Catalog{Story: d, Maps: map[uint16]assets.Map{65001: {ID: 65001, Events: []assets.Event{event}}}})
	source, _ := w.Event(65001, 1)
	prepared := w.PrepareStory(nil, nil, 65001, source, 0)
	if prepared == source || len(prepared.Branches[0].Operations) != 2 || !reflect.DeepEqual(*source, event) {
		t.Fatal("repair mutated source or failed to apply")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := character()
			for range 50 {
				if got := w.PrepareStory(c, nil, 65001, source, 0); got != prepared || len(got.Branches[0].Operations) != 2 {
					t.Error("immutable program was not shared safely")
				}
			}
		}()
	}
	wg.Wait()
	// Preserve order around conditional edits: prepend 5 then remove it only
	// for the eligible character. Caching the unconditional subset is unsafe.
	code := byte(5)
	d.Rules = append(d.Rules, assets.StoryRule{ID: "conditional_remove", Kind: assets.StoryRepair, Scope: rule.Scope,
		Gates: []assets.StoryGate{{Source: "mark_exists", ID: 100000, Op: "eq", Value: 1}},
		Edits: []assets.StoryEdit{{Kind: "remove", Match: assets.StoryPattern{Code: &code}}}})
	w.ReloadStory(d)
	for _, tc := range []struct {
		c    *game.Character
		want int
	}{{character(), 2}, {character(game.Quest{ID: 100000}), 1}} {
		if got := w.PrepareStory(tc.c, nil, 65001, source, 0); len(got.Branches[0].Operations) != tc.want {
			t.Fatal("conditional/unconditional ordering or player leakage", got)
		}
	}
	w.ReloadStory(assets.StoryDefinitions{Rules: []assets.StoryRule{rule}})
	updated := assets.Event{ClickID: 1, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{{Data: [21]byte{3}}}}}}
	w.ReloadEvents(map[uint16]assets.Map{65001: {Events: []assets.Event{updated}}})
	source, _ = w.Event(65001, 1)
	after := w.PrepareStory(nil, nil, 65001, source, 0)
	if after == prepared || after.Branches[0].Operations[1].Data[0] != 3 {
		t.Fatal("event reload reused obsolete repair")
	}
	w.ReloadCatalog(&assets.Catalog{Maps: map[uint16]assets.Map{65001: {ID: 65001, Events: []assets.Event{event}}}})
	source, _ = w.Event(65001, 1)
	if w.PrepareStory(nil, nil, 65001, source, 0) != source {
		t.Fatal("catalog reload retained cleared repair")
	}
}
