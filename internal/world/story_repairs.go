package world

import (
	"slices"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

type storyRepairKey struct {
	event  *assets.Event
	branch int
	mapID  uint16
}

// Only branches whose applicable repairs are all unconditional are cached.
// Conditional and unconditional edits must otherwise retain their relative order.
func (w *World) indexStoryRepairs() {
	w.story.repairs = map[storyRepairKey]*assets.Event{}
	for mapID, m := range w.maps {
		ids := w.story.candidates(assets.StoryRepair, mapID)
		if len(ids) == 0 {
			continue
		}
		for _, events := range [][]assets.Event{m.data.Events, m.data.PreEvents} {
			for i := range events {
				ev := &events[i]
				for branch, br := range ev.Branches {
					x := StoryContext{Map: mapID, Event: ev.ClickID, Branch: br.Index}
					applicable, dynamic := false, false
					for _, id := range ids {
						r := w.story.rules[id].source
						if !storyEventScopeMatches(r.Scope, x) || r.Pattern != (assets.StoryPattern{}) {
							continue
						}
						applicable = true
						dynamic = dynamic || len(r.Gates) > 0
					}
					if applicable && !dynamic {
						w.story.repairs[storyRepairKey{ev, branch, mapID}] = w.prepareStory(nil, nil, mapID, ev, branch)
					}
				}
			}
		}
	}
}

// PrepareStory returns an immutable execution program. Cached programs contain
// no player state; session counters and callbacks remain owned by the session.
func (w *World) PrepareStory(c *game.Character, v *View, mapID uint16, ev *assets.Event, branch int) *assets.Event {
	if ev == nil || branch < 0 || branch >= len(ev.Branches) {
		return ev
	}
	if prepared := w.story.repairs[storyRepairKey{ev, branch, mapID}]; prepared != nil {
		return prepared
	}
	return w.prepareStory(c, v, mapID, ev, branch)
}
func (w *World) prepareStory(c *game.Character, v *View, mapID uint16, ev *assets.Event, branch int) *assets.Event {
	var out *assets.Event
	var br assets.Branch
	var ops []assets.Operation
	for r := range w.StoryRules(assets.StoryRepair, c, v, StoryContext{Map: mapID, Event: ev.ClickID, Branch: ev.Branches[branch].Index}) {
		if out == nil {
			copy := *ev
			copy.Branches = append([]assets.Branch(nil), ev.Branches...)
			out = &copy
			br = out.Branches[branch]
			ops = append([]assets.Operation(nil), br.Operations...)
		}
		for _, e := range r.Edits {
			var insert []assets.Operation
			for _, o := range e.Operations {
				insert = append(insert, o.Operation())
			}
			match := func(raw assets.Operation) bool { return e.Match.Matches(raw.Data) }
			switch e.Kind {
			case "remove":
				ops = slices.DeleteFunc(ops, match)
			case "prepend":
				ops = append(insert, ops...)
			case "replace_first":
				if i := slices.IndexFunc(ops, match); i >= 0 {
					ops = slices.Replace(ops, i, i+1, insert...)
				}
			case "copy_speech":
				for _, source := range ev.Branches {
					if source.Index == e.SourceBranch {
						for _, o := range source.Operations {
							if DecodeOp(o).NPCSpeech() {
								insert = append(insert, o)
							}
						}
					}
				}
				ops = append(insert, ops...)
			case "move_before":
				at := slices.IndexFunc(ops, match)
				if at < 0 {
					continue
				}
				mark := ops[at]
				ops = slices.Delete(ops, at, at+1)
				to := slices.IndexFunc(ops, func(raw assets.Operation) bool { return e.Before.Matches(raw.Data) })
				if to < 0 {
					to = min(at, len(ops))
				}
				ops = slices.Insert(ops, to, mark)
			}
		}
	}
	if out == nil {
		return ev
	}
	br.Operations = ops
	out.Branches[branch] = br
	return out
}
