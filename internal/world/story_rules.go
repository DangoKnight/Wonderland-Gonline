package world

import (
	"iter"
	"slices"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

// StoryContext identifies the authored operation being evaluated. It contains no
// mutable player state and can be reused when rechecking a SQL transaction.
type StoryContext struct {
	Map, Event, Actor uint16
	Branch            byte
	Raw               *[21]byte
}
type storyIndexKey struct {
	kind  string
	mapID uint16
}

type compiledStoryRule struct {
	source *assets.StoryRule
	gates  storyGateProgram
}
type storyRuntime struct {
	index   map[storyIndexKey][]int
	globals map[string][]int
	rules   []compiledStoryRule
	gates   map[storyGatesKey]*storyGateProgram
	repairs map[storyRepairKey]*assets.Event
}

func (w *World) indexStory() {
	snapshot := &storyRuntime{
		index:   map[storyIndexKey][]int{},
		globals: map[string][]int{},
		rules:   make([]compiledStoryRule, len(w.catalog.Story.Rules)),
		gates:   map[storyGatesKey]*storyGateProgram{},
	}
	for i := range w.catalog.Story.Rules {
		r := &w.catalog.Story.Rules[i]
		snapshot.rules[i] = compiledStoryRule{source: r, gates: compileStoryGates(r.Gates)}
		if len(r.Gates) > 0 {
			snapshot.gates[storyGatesKey{&r.Gates[0], len(r.Gates)}] = &snapshot.rules[i].gates
		}
		maps := r.Scope.Maps
		if len(maps) == 0 {
			snapshot.globals[r.Kind] = append(snapshot.globals[r.Kind], i)
			maps = []uint16{0}
		}
		for _, m := range maps {
			k := storyIndexKey{r.Kind, m}
			snapshot.index[k] = append(snapshot.index[k], i)
		}
	}
	for _, p := range w.catalog.Story.Transformations {
		if len(p.Gates) > 0 {
			program := compileStoryGates(p.Gates)
			snapshot.gates[storyGatesKey{&p.Gates[0], len(p.Gates)}] = &program
		}
	}
	// Source-order indexes are already sorted. Merge global and local candidates
	// once, preserving precedence without sorting or copying during a query.
	for key, ids := range snapshot.index {
		if key.mapID == 0 {
			continue
		}
		global := snapshot.globals[key.kind]
		if len(global) == 0 {
			continue
		}
		merged := make([]int, 0, len(global)+len(ids))
		a, b := 0, 0
		for a < len(global) && b < len(ids) {
			if global[a] < ids[b] {
				merged = append(merged, global[a])
				a++
			} else {
				merged = append(merged, ids[b])
				b++
			}
		}
		merged = append(merged, global[a:]...)
		merged = append(merged, ids[b:]...)
		snapshot.index[key] = merged
	}
	w.story = snapshot
	w.indexStoryRepairs()
}
func (s *storyRuntime) candidates(kind string, mapID uint16) []int {
	if ids, ok := s.index[storyIndexKey{kind, mapID}]; ok {
		return ids
	}
	return s.globals[kind]
}
func StoryScopeMatches(s assets.StoryScope, x StoryContext) bool {
	return (len(s.Maps) == 0 || slices.Contains(s.Maps, x.Map)) && storyEventScopeMatches(s, x)
}
func storyEventScopeMatches(s assets.StoryScope, x StoryContext) bool {
	return (len(s.Events) == 0 || slices.Contains(s.Events, x.Event)) && (len(s.Branches) == 0 || slices.Contains(s.Branches, x.Branch)) && (len(s.Actors) == 0 || slices.Contains(s.Actors, x.Actor))
}
func (w *World) storyRuleMatches(r *compiledStoryRule, c *game.Character, x StoryContext) bool {
	if !storyEventScopeMatches(r.source.Scope, x) {
		return false
	}
	if r.source.Pattern != (assets.StoryPattern{}) && (x.Raw == nil || !r.source.Pattern.Matches(*x.Raw)) {
		return false
	}
	return w.storyProgramMatches(&r.gates, c, x)
}

const storyMatchWordBits = 64
const storyLocalMatchWords = 4

// StoryRules yields immutable snapshot references in source order. Match flags
// are captured before the first action, preserving selection semantics when a
// caller changes quest state during iteration. Ordinary candidate lists fit on
// the stack; unusually large lists use one bounded temporary bitmap.
func (w *World) StoryRules(kind string, c *game.Character, v *View, x StoryContext) iter.Seq[*assets.StoryRule] {
	snapshot := w.story
	return func(yield func(*assets.StoryRule) bool) {
		ids := snapshot.candidates(kind, x.Map)
		var local [storyLocalMatchWords]uint64
		flags := local[:]
		words := (len(ids) + storyMatchWordBits - 1) / storyMatchWordBits
		if words > len(local) {
			flags = make([]uint64, words)
		}
		for i, id := range ids {
			if w.storyRuleMatches(&snapshot.rules[id], c, x) {
				flags[i/storyMatchWordBits] |= uint64(1) << uint(i%storyMatchWordBits)
			}
		}
		for i, id := range ids {
			if flags[i/storyMatchWordBits]&(uint64(1)<<uint(i%storyMatchWordBits)) != 0 {
				if !yield(snapshot.rules[id].source) {
					return
				}
			}
		}
	}
}
func (w *World) StoryFirst(kind string, c *game.Character, v *View, x StoryContext) (assets.StoryRule, bool) {
	snapshot := w.story
	for _, id := range snapshot.candidates(kind, x.Map) {
		r := &snapshot.rules[id]
		if w.storyRuleMatches(r, c, x) {
			return *r.source, true
		}
	}
	return assets.StoryRule{}, false
}

func (w *World) OutcomeBranch(c *game.Character, v *View, mapID uint16, ev *assets.Event, source, result uint16, origin int) int {
	policy, ok := w.StoryFirst(assets.StoryOutcome, c, v, StoryContext{Map: mapID, Event: ev.ClickID})
	if !ok {
		return w.FindBranch(c, v, mapID, ev, ConditionBattleResult, source, result, -1)
	}
	if origin < 0 || origin >= len(ev.Branches) {
		return -1
	}
	for i := origin + 1; i < len(ev.Branches); i++ {
		br := ev.Branches[i]
		cond := DecodeCond(br.Condition)
		if policy.Value == assets.StoryOutcomeBounded && cond.Kind != ConditionBattleResult && len(br.Operations) > 0 {
			return -1
		}
		if cond.Kind != ConditionBattleResult || cond.W1 != source || cond.W2 != result {
			continue
		}
		probe := *ev
		probe.Branches = append([]assets.Branch(nil), ev.Branches[i:]...)
		if w.FindBranch(c, v, mapID, &probe, ConditionBattleResult, source, result, -1) == 0 {
			return i
		}
	}
	return -1
}
