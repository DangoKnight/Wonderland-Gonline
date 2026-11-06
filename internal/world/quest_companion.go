package world

import "wonderland-gonline/internal/assets"

// QuestCompanionPlan commits an escort and its checkpoint together. Visibility
// is presentation only and is published after the durable change succeeds.
type QuestCompanionPlan struct {
	Template uint32
	Mark     uint32
	Join     bool
	Marks    []assets.StoryMark
	Actors   []Op
}

func (w *World) QuestCompanion(mapID uint16, ev *assets.Event, branch, index int) (QuestCompanionPlan, bool) {
	var plan QuestCompanionPlan
	if ev == nil || branch < 0 || branch >= len(ev.Branches) {
		return plan, false
	}
	br := ev.Branches[branch]
	if index < 0 || index >= len(br.Operations) || DecodeCond(br.Condition).Kind != ConditionQuest {
		return plan, false
	}
	rule, ok := w.StoryFirst(assets.StoryQuestCompanion, nil, nil, StoryContext{Map: mapID, Event: ev.ClickID, Branch: br.Index, Raw: &br.Operations[index].Data})
	if !ok || int64(index) != rule.Value || rule.Template == 0 {
		return plan, false
	}
	if _, known := w.Template(rule.Template); !known {
		return plan, false
	}
	plan.Template, plan.Mark = rule.Template, rule.Mark
	found := false
	for _, raw := range br.Operations[index:] {
		op := DecodeOp(raw)
		switch {
		case op.Code == ActionCompanion && uint32(op.D2) == rule.Template && op.Value() == 0:
			if found {
				return QuestCompanionPlan{}, false
			}
			if op.D1 == CompanionQuestJoin && op.D3 == CompanionQuestJoinCompatibility {
				plan.Join = true
			} else if op.D1 != CompanionQuestLeave || op.D3 != 0 {
				return QuestCompanionPlan{}, false
			}
			found = true
		case op.Code == ActionQuestMark && uint32(op.D1) == rule.Mark && op.D2 == QuestMarkAdvance && op.Value() == 1:
			plan.Marks = append(plan.Marks, assets.StoryMark{ID: rule.Mark, Mode: "advance", Value: 1})
		case op.Code == ActionActor && op.D1 != 0 && (op.D2 == ActorActionHide || op.D2 == ActorActionShow) && op.D3 == 1 && op.Value() == ActorVisibilityCompatibility:
			plan.Actors = append(plan.Actors, op)
		default:
			return QuestCompanionPlan{}, false
		}
	}
	return plan, found
}

// QuestCompanionAction permits an otherwise unsupported operand only when the
// entire remaining tail matches its SQL-owned escort contract.
func (w *World) QuestCompanionAction(mapID uint16, ev *assets.Event, branch, index int) bool {
	for start := 0; start <= index; start++ {
		if _, ok := w.QuestCompanion(mapID, ev, branch, start); ok {
			return true
		}
	}
	return false
}
