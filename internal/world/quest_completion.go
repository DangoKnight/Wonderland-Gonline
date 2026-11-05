package world

import (
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

// QuestCompletionPlan contains only the native resource/checkpoint tail after
// the last dialogue. An authored SQL policy opts each reviewed branch in.
type QuestCompletionPlan struct {
	Companions []QuestCompanionReward
	Skills     []uint16
	Items      []game.ItemChange
	Marks      []assets.StoryMark
	Gold       []int32
	EXP        uint64
}

type QuestCompanionReward struct {
	ID      uint32
	Recruit bool
}

func (w *World) QuestCompletion(mapID uint16, ev *assets.Event, branch, index int) (QuestCompletionPlan, bool) {
	var plan QuestCompletionPlan
	if ev == nil || branch < 0 || branch >= len(ev.Branches) {
		return plan, false
	}
	br := ev.Branches[branch]
	if index < 0 || index >= len(br.Operations) {
		return plan, false
	}
	var rule assets.StoryRule
	matched := false
	for _, kind := range []string{assets.StoryQuestRewardBundle, assets.StoryQuestCompletion} {
		for candidate := range w.StoryRules(kind, nil, nil, StoryContext{Map: mapID, Event: ev.ClickID, Branch: br.Index, Raw: &br.Operations[index].Data}) {
			if candidate.Value == int64(index) {
				rule = *candidate
				matched = true
				break
			}
		}
		if matched {
			break
		}
	}
	if !matched || DecodeCond(br.Condition).Kind != ConditionQuest {
		return plan, false
	}
	// Existing authored exceptional rewards own their entire delivery contract.
	// A general native completion must never omit their configured extra grants.
	for _, reward := range w.catalog.Story.Rewards {
		if !StoryScopeMatches(reward.Scope, StoryContext{Map: mapID, Event: ev.ClickID, Branch: br.Index}) || !reward.Condition.Matches(br.Condition) {
			continue
		}
		for _, raw := range br.Operations[index:] {
			if reward.Pattern.Matches(raw.Data) {
				return plan, false
			}
		}
	}
	checkpoint := false
	for _, raw := range br.Operations[index:] {
		op := DecodeOp(raw)
		switch {
		case rule.Kind == assets.StoryQuestRewardBundle && op.Code == ActionCompanion && op.Value() == 0 && ((op.D1 == CompanionRecruit && op.D3 == 1) || (op.D1 == CompanionDismiss && op.D3 == 0)):
			if _, known := w.Template(uint32(op.D2)); !known {
				return QuestCompletionPlan{}, false
			}
			plan.Companions = append(plan.Companions, QuestCompanionReward{ID: uint32(op.D2), Recruit: op.D1 == CompanionRecruit})
		case rule.Kind == assets.StoryQuestRewardBundle && op.Code == ActionLearnSkill && op.D2 == 1 && op.D3 == 0 && op.Value() == 1:
			if _, known := w.catalog.Skills[op.D1]; !known {
				return QuestCompletionPlan{}, false
			}
			plan.Skills = append(plan.Skills, op.D1)
		case op.Code == ActionPlayer && op.D1 == PlayerActionReward && op.D2 == PlayerRewardItem && op.D3 != 0:
			if _, exists := w.catalog.Items[op.D3]; !exists {
				return QuestCompletionPlan{}, false
			}
			if count := int32(op.Value()); count != 0 {
				plan.Items = append(plan.Items, game.ItemChange{ID: op.D3, Count: int(count)})
			}
		case op.Code == ActionPlayer && op.D1 == PlayerActionReward && op.D2 == PlayerRewardGold && op.D3 == 0:
			plan.Gold = append(plan.Gold, int32(op.Value()))
		case op.Code == ActionPlayer && op.D1 == PlayerActionReward && op.D2 == PlayerRewardExperience && op.D3 == PlayerExperienceValue:
			if int32(op.Value()) < 0 {
				return QuestCompletionPlan{}, false
			}
			plan.EXP += uint64(op.Value())
		case op.Code == ActionQuestMark && op.D1 != 0 && (op.D2 == QuestMarkAdvance || op.D2 == QuestMarkComplete):
			if _, exists := w.catalog.Marks[op.D1]; !exists {
				return QuestCompletionPlan{}, false
			}
			mark := assets.StoryMark{ID: uint32(op.D1), Mode: "advance", Value: int(int32(op.Value()))}
			if op.D2 == QuestMarkComplete {
				mark.Mode, mark.Value, mark.PreserveStep = "complete", 0, true
				checkpoint = checkpoint || uint32(op.D1) == rule.Mark
			}
			plan.Marks = append(plan.Marks, mark)
		default:
			return QuestCompletionPlan{}, false
		}
	}
	if !checkpoint || len(plan.Items) == 0 && len(plan.Gold) == 0 && plan.EXP == 0 && len(plan.Companions) == 0 && len(plan.Skills) == 0 {
		return QuestCompletionPlan{}, false
	}
	return plan, true
}
