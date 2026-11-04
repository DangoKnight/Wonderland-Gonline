package world

import "wonderland-gonline/internal/assets"

func (w *World) QuestDisguise(mapID uint16, ev *assets.Event, branch int, op Op) (uint16, bool) {
	if ev == nil || branch < 0 || branch >= len(ev.Branches) || op.Code != ActionTransform || op.D3 != 0 || op.Value() != 0 {
		return 0, false
	}
	if op.D1 == TransformNPCDisguise {
		if _, known := w.Template(uint32(op.D2)); !known || op.D2 == 0 {
			return 0, false
		}
	} else if op.D1 != TransformRestoreShape || op.D2 != 0 {
		return 0, false
	}
	raw := op.Raw()
	_, ok := w.StoryFirst(assets.StoryQuestDisguise, nil, nil, StoryContext{Map: mapID, Event: ev.ClickID, Branch: ev.Branches[branch].Index, Raw: &raw})
	return op.D2, ok
}
