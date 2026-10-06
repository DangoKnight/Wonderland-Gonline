package world

import (
	"encoding/binary"
	"slices"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

// These authored exceptions are from EveEventRuntime.FindBranch and the
// Niss/Xaolan controllers. They amend execution copies, never SQL definitions.
const (
	storyElinWeaponMap               = 11167
	storyElinTemplate                = 14230
	storyElinOriginalWeapon          = 20044
	storyElinReplacementWeapon       = 20045
	storyWeaponEquipmentIndex        = 2
	storyElinNativeWeaponOperand4    = 4
	storyElinNativeWeaponOperand1    = 1
	storyXaolanHomeMap               = 12002
	storyXaolanHomeFirst             = 8
	storyXaolanHomeSecond            = 9
	storyXaolanFateMark              = 13086
	storyXaolanDeathMark             = 13087
	storyAlchemyMap                  = 10071
	storyAlchemyEvent                = 6
	storyAlchemyHandInBranch         = 6
	storyGhostdomMap                 = 11148
	storyGhostdomGuardFirst          = 2
	storyGhostdomGuardSecond         = 4
	storyOracleHandInBranch          = 2
	storyNissInnMap                  = 12052
	storyNissInnEvent                = 6
	storyNissTemplate                = 14081
	storyXaolanFateMap               = 11077
	storyXaolanBattleEvent           = 7
	storyXaolanFarewellEvent         = 12
	storyXaolanUnresolvedActorAction = 11
)

func xaolanFate(c *game.Character) bool {
	q, active := questActive(c, storyXaolanFateMark)
	death, dead := questActive(c, storyXaolanDeathMark)
	return active && q.Step >= 2 || dead && death.Step > 0
}
func storyForwardOnly(mapID, event uint16) bool {
	return mapID == game.MapID11157 || mapID == game.MapID11013 && event == 11 || mapID == game.MapID12211 && event == 1 || mapID == game.MapID11149 && event == 2
}
func storyPreferredBranch(mapID, event uint16) byte {
	if mapID == storyAlchemyMap && event == storyAlchemyEvent {
		return storyAlchemyHandInBranch
	}
	if mapID == storyGhostdomMap && (event == storyGhostdomGuardFirst || event == storyGhostdomGuardSecond) {
		return storyOracleHandInBranch
	}
	return 0
}

func storyOperation(code byte, d1, d2, d3, d4 uint16, dw uint32) assets.Operation {
	var op assets.Operation
	op.Data[0] = code
	for i, v := range []uint16{d1, d2, d3, d4} {
		binary.LittleEndian.PutUint16(op.Data[1+i*2:], v)
	}
	binary.LittleEndian.PutUint32(op.Data[9:], dw)
	return op
}

// PrepareStory returns a copy of the selected branch for two verified reference
// repairs. Checkpoints precede dismissal so reconnects can resume the story.
func PrepareStory(mapID uint16, ev *assets.Event, branch int) *assets.Event {
	if ev == nil || branch < 0 || branch >= len(ev.Branches) {
		return ev
	}
	br := ev.Branches[branch]
	niss := mapID == storyNissInnMap && ev.ClickID == storyNissInnEvent && slices.Contains([]byte{1, 6, 12, 14}, br.Index)
	xaolan := mapID == storyXaolanFateMap && (ev.ClickID == storyXaolanBattleEvent && (br.Index == 5 || br.Index == 7) || ev.ClickID == storyXaolanFarewellEvent && br.Index == 1)
	if !niss && !xaolan {
		return ev
	}
	out := *ev
	out.Branches = append([]assets.Branch(nil), ev.Branches...)
	ops := append([]assets.Operation(nil), br.Operations...)
	if niss {
		ops = slices.DeleteFunc(ops, func(raw assets.Operation) bool {
			op := DecodeOp(raw)
			return (br.Index == 1 || br.Index == 6) && op.Code == ActionCompanion && op.D1 == 2 && op.D2 == storyNissTemplate || (br.Index == 12 || br.Index == 14) && op.Code == ActionWireCode10 && op.D1 == 1 && op.D2 == 0 && op.D3 == 0 && op.Value() == 0
		})
		if br.Index == 12 {
			ops = append([]assets.Operation{storyOperation(ActionCompanion, 2, storyNissTemplate, 0, 0, 0)}, ops...)
		}
	}
	if xaolan {
		if ev.ClickID == storyXaolanBattleEvent && br.Index == 5 {
			ops = moveStoryMark(ops, storyXaolanFateMark, func(op Op) bool { return op.Code == ActionCompanion && op.D1 == 2 })
		} else {
			for i, raw := range ops {
				op := DecodeOp(raw)
				if op.Code == ActionActor && op.D2 == storyXaolanUnresolvedActorAction {
					hides := []assets.Operation{storyOperation(ActionActor, 12, 2, 1, 65280, 255), storyOperation(ActionActor, 13, 2, 1, 65280, 255)}
					ops = slices.Replace(ops, i, i+1, hides...)
					break
				}
			}
			ops = moveStoryMark(ops, storyXaolanDeathMark, func(op Op) bool { return op.Code == ActionQuestMark && op.D1 == storyXaolanFateMark })
		}
	}
	br.Operations = ops
	out.Branches[branch] = br
	return &out
}
func moveStoryMark(ops []assets.Operation, id uint16, before func(Op) bool) []assets.Operation {
	at := slices.IndexFunc(ops, func(raw assets.Operation) bool { op := DecodeOp(raw); return op.Code == ActionQuestMark && op.D1 == id })
	if at < 0 {
		return ops
	}
	mark := ops[at]
	ops = slices.Delete(ops, at, at+1)
	to := slices.IndexFunc(ops, func(raw assets.Operation) bool { return before(DecodeOp(raw)) })
	if to < 0 {
		return slices.Insert(ops, min(at, len(ops)), mark)
	}
	return slices.Insert(ops, to, mark)
}

// OutcomeBranch scopes callbacks to the formation that actually started the
// battle. These reference stories reuse source=1 for multiple fights.
func (w *World) OutcomeBranch(c *game.Character, v *View, mapID uint16, ev *assets.Event, source, result uint16, origin int) int {
	bounded := mapID == game.MapID12380 && ev.ClickID == 2 || mapID == game.MapID12523 && ev.ClickID == 12
	forward := mapID == game.MapID11149 && ev.ClickID == 5
	if !bounded && !forward {
		return w.FindBranch(c, v, mapID, ev, ConditionBattleResult, source, result, -1)
	}
	if origin < 0 || origin >= len(ev.Branches) {
		return -1
	}
	for i := origin + 1; i < len(ev.Branches); i++ {
		br := ev.Branches[i]
		cond := DecodeCond(br.Condition)
		if bounded && cond.Kind != ConditionBattleResult && len(br.Operations) > 0 {
			return -1
		}
		if cond.Kind != ConditionBattleResult || cond.W1 != source || cond.W2 != result {
			continue
		}
		// Reuse ordinary AND-condition validation, restricted to this callback.
		probe := *ev
		probe.Branches = append([]assets.Branch(nil), ev.Branches[i:]...)
		if w.FindBranch(c, v, mapID, &probe, ConditionBattleResult, source, result, -1) == 0 {
			return i
		}
	}
	return -1
}
