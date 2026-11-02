package world

import (
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

// ValidStoryProgram guards compatibility policies against unrelated programs
// that reuse the same event ID. The expected native shape is SQL-authored too.
func (w *World) ValidStoryProgram(mapID uint16, ev *assets.Event) bool {
	if ev == nil {
		return false
	}
	for _, p := range w.catalog.Story.Transformations {
		scope := p.Scope
		scope.Branches = nil
		if StoryScopeMatches(scope, StoryContext{Map: mapID, Event: ev.ClickID}) && validStoryProgram(p, ev) {
			return true
		}
	}
	return false
}
func validStoryProgram(p assets.StoryTransformation, ev *assets.Event) bool {
	if len(p.Program) == 0 {
		return false
	}
	for _, need := range p.Program {
		found := false
		for _, br := range ev.Branches {
			if br.Index == need.Index && need.Condition.Matches(br.Condition) && (need.ExtraConditions < 0 || DecodeCond(br.Condition).ExtraConditions() == need.ExtraConditions) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, br := range ev.Branches {
		if len(p.Scope.Branches) == 0 || containsBranch(p.Scope.Branches, br.Index) {
			for _, o := range br.Operations {
				if p.Pattern.Matches(o.Data) {
					return true
				}
			}
		}
	}
	return false
}
func containsBranch(branches []byte, b byte) bool {
	for _, v := range branches {
		if v == b {
			return true
		}
	}
	return false
}

func (w *World) Transformation(mapID uint16, ev *assets.Event, branch int, op Op) (assets.StoryTransformation, bool) {
	if ev == nil || branch < 0 || branch >= len(ev.Branches) {
		return assets.StoryTransformation{}, false
	}
	x := StoryContext{Map: mapID, Event: ev.ClickID, Branch: ev.Branches[branch].Index}
	for _, p := range w.catalog.Story.Transformations {
		if StoryScopeMatches(p.Scope, x) && p.Pattern.Matches(op.Raw()) && validStoryProgram(p, ev) {
			return p, true
		}
	}
	return assets.StoryTransformation{}, false
}

// StoryExchange validates a speech/debit/credit program. All quantities and item
// identities come from the authored event, then commit as one owned transaction.
func (w *World) StoryExchange(mapID uint16, ev *assets.Event, branch, index int) ([]game.ItemChange, bool) {
	if ev == nil || branch < 0 || branch >= len(ev.Branches) {
		return nil, false
	}
	br := ev.Branches[branch]
	r, ok := w.StoryFirst(assets.StoryExchange, nil, nil, StoryContext{Map: mapID, Event: ev.ClickID, Branch: br.Index})
	if !ok || index != int(r.Value) || len(br.Operations) != 3 {
		return nil, false
	}
	cond := DecodeCond(br.Condition)
	if cond.Kind != ConditionSubject || cond.W1 != 1 || cond.W2 != 1 || cond.W3 == 0 || cond.ExtraConditions() != 0 {
		return nil, false
	}
	speech, debit, credit := DecodeOp(br.Operations[0]), DecodeOp(br.Operations[1]), DecodeOp(br.Operations[2])
	if !speech.NPCSpeech() || speech.D1 != r.TargetActor || debit.Code != ActionPlayer || debit.D1 != PlayerActionReward || debit.D2 != 1 || debit.D3 != cond.W3 || int32(debit.Value()) >= 0 || credit.Code != ActionPlayer || credit.D1 != PlayerActionReward || credit.D2 != 1 || credit.D3 == 0 || int32(credit.Value()) <= 0 {
		return nil, false
	}
	return []game.ItemChange{{ID: debit.D3, Count: int(int32(debit.Value()))}, {ID: credit.D3, Count: int(int32(credit.Value()))}}, true
}
