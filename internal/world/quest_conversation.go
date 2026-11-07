package world

import (
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

// QuestConversation is presentation/checkpoint work that needs no intervening
// gameplay. The rewards flag also permits automatic resource/skill delivery.
func QuestConversation(br assets.Branch, rewards bool) (speech, reward bool, ok bool) {
	ok = true
	for _, raw := range br.Operations {
		op := DecodeOp(raw)
		switch {
		case op.PlayerSpeech() || op.NPCSpeech():
			speech = true
		case op.Code == ActionQuestMark:
			if op.D1 == 0 || op.D2 != QuestMarkAdvance && op.D2 != QuestMarkComplete {
				return false, false, false
			}
		case op.Code == ActionActor && (op.D2 == ActorActionTurn || op.D2 == ActorActionPose || op.D2 == ActorActionAnimation || op.D2 == ActorActionExpression || op.D2 == ActorActionHide || op.D2 == ActorActionShow):
		case op.Code == ActionPlayer && (op.D1 == PlayerActionAnimation || op.D1 == PlayerActionEffect):
		case op.Code == ActionEffect || op.Code == ActionMinimapMarker:
		case op.Code == ActionMovie && (op.D1 == MovieActionPlay || op.D1 == MovieActionPlayAlternate):
		case op.Code == ActionPlayer && op.D1 == PlayerActionChoice:
			speech = true
		case op.Code == ActionActor && op.D2 == ActorActionChoice:
			speech = true
		case rewards && op.Code == ActionPlayer && op.D1 == PlayerActionReward && (op.D2 == PlayerRewardItem || op.D2 == PlayerRewardItemCompatibility || op.D2 == PlayerRewardGold || op.D2 == PlayerRewardExperience):
			if int32(op.Value()) > 0 {
				reward = true
			}
		case rewards && op.Code == ActionLearnSkill:
			reward = true
		case rewards && op.FullRecovery():
			reward = true
		case rewards && op.Code == ActionActor && op.D2 == ActorActionConstellation:
			reward = true
		case rewards && op.Code == ActionCompanion && (op.D1 == CompanionRecruit || op.D1 == CompanionQuestJoin):
			reward = true
		case rewards && op.Code == ActionCompanion && (op.D1 == CompanionDismiss || op.D1 == CompanionQuestLeave):
		default:
			return false, false, false
		}
	}
	return
}

// ImmediateQuestReward follows exactly the event that another click on this NPC
// would select, and only if this conversation newly enabled it through a quest
// checkpoint. It does not skip unmet inventory, level, timer or other conditions.
// Authored StoryContinue links have precedence in the server adapter.
func (w *World) ImmediateQuestReward(before, after *game.Character, v *View, mapID, actor uint16, source *assets.Event, branch int) (*assets.Event, int) {
	if before == nil || after == nil || after.Map != mapID || source == nil || branch < 0 || branch >= len(source.Branches) {
		return nil, -1
	}
	if _, exists := w.NPC(mapID, actor); !exists {
		return nil, -1
	}
	if v != nil && (v.Hidden[actor] || !w.VisibleIn(after, v, mapID, actor)) {
		return nil, -1
	}
	speech, _, ok := QuestConversation(source.Branches[branch], true)
	if !speech || !ok {
		return nil, -1
	}
	changed := map[uint16]bool{}
	for _, raw := range source.Branches[branch].Operations {
		op := DecodeOp(raw)
		if op.Code != ActionQuestMark {
			continue
		}
		a, was := before.Quests[uint32(op.D1)]
		b, now := after.Quests[uint32(op.D1)]
		if was != now || a.State != b.State || a.Step != b.Step {
			changed[op.D1] = true
		}
	}
	if len(changed) == 0 {
		return nil, -1
	}
	// Compare the actual selected branch, rather than only the destination's
	// condition: an unconditional fallback reward can have been masked by the
	// first-time conversation until its checkpoint changed.
	var priorEvent uint16
	priorBranch := -1
	for _, ev := range w.NPCEvents(mapID, actor) {
		if i := w.FindBranch(before, v, mapID, ev, TriggerEntry, 0, 0, -1); i >= 0 {
			priorEvent, priorBranch = ev.ClickID, i
			break
		}
	}
	for _, ev := range w.NPCEvents(mapID, actor) {
		next := w.FindBranch(after, v, mapID, ev, TriggerEntry, 0, 0, -1)
		if next < 0 {
			continue
		}
		// Do not jump past the branch the NPC would actually execute.
		if w.Disabled(mapID, ev.ClickID) || ev.ClickID == source.ClickID && next == branch {
			return nil, -1
		}
		_, reward, safe := QuestConversation(ev.Branches[next], true)
		if !safe || !reward {
			return nil, -1
		}
		if ev.ClickID != priorEvent || next != priorBranch {
			return ev, next
		}
		return nil, -1
	}
	return nil, -1
}
