package world

import (
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

const (
	BreillatInitialBranch    = 1
	BreillatGreetingBranch   = 3
	BreillatOfferBranch      = 7
	BreillatAcceptanceBranch = 9
	breillatEvent            = 4
	breillatAcceptAnswer     = 30
	breillatQuestion         = 1
	transformPlayer          = 1
	questConditionActive     = 1
	BreillatActor            = 5
)

func (o Op) BreillatTransform() bool {
	return o.Code == ActionTransform && o.D1 == transformPlayer && o.D2 == game.BreillatModel && o.D3 == 0 && o.Value() == 0
}

// IsBreillat recognizes the authored script rather than treating every event 4
// as a transformation. The eleven ship maps share this script.
func IsBreillat(ev *assets.Event) bool {
	if ev == nil || ev.ClickID != breillatEvent || len(ev.Branches) < BreillatAcceptanceBranch {
		return false
	}
	initial, greeting, offer, accept := ev.Branches[BreillatInitialBranch-1], ev.Branches[BreillatGreetingBranch-1], ev.Branches[BreillatOfferBranch-1], ev.Branches[BreillatAcceptanceBranch-1]
	if initial.Index != BreillatInitialBranch || greeting.Index != BreillatGreetingBranch || offer.Index != BreillatOfferBranch || accept.Index != BreillatAcceptanceBranch {
		return false
	}
	condition := DecodeCond(offer.Condition)
	callback := DecodeCond(accept.Condition)
	if condition.Kind != ConditionQuest || condition.W2 != questConditionActive || condition.ExtraConditions() != 1 || condition.W1 != game.BreillatTalkMark || byte(condition.W4) != CompareGreater || (uint32(condition.W4>>8)|uint32(condition.W5)<<8|uint32(condition.W6&255)<<24) != game.BreillatRequiredTalks-1 || callback.Kind != ConditionChoiceResult || callback.W1 != breillatQuestion || callback.W2 != breillatAcceptAnswer {
		return false
	}
	gate := DecodeCond(ev.Branches[BreillatOfferBranch].Condition)
	if len(ev.Branches[BreillatOfferBranch].Operations) != 0 || gate.Kind != ConditionQuest || gate.W1 != game.BreillatConversionMark || gate.W2 != questConditionActive || gate.W4 != CompareEqual || gate.W5 != 0 || gate.W6&255 != 0 {
		return false
	}
	if DecodeCond(initial.Condition).W1 != game.BreillatTalkMark || DecodeCond(greeting.Condition).W1 != game.BreillatTalkMark {
		return false
	}
	for _, raw := range accept.Operations {
		if DecodeOp(raw).BreillatTransform() {
			return true
		}
	}
	return false
}

// PrepareBreillat preserves the SQL catalog. The first greeting counts once;
// acceptance marks and visibility are deferred to the atomic model conversion.
func PrepareBreillat(ev *assets.Event, branch int) *assets.Event {
	if !IsBreillat(ev) {
		return ev
	}
	result := *ev
	result.Branches = append([]assets.Branch(nil), ev.Branches...)
	br := &result.Branches[branch]
	switch br.Index {
	case BreillatInitialBranch:
		var ops []assets.Operation
		for _, raw := range ev.Branches[BreillatGreetingBranch-1].Operations {
			if DecodeOp(raw).NPCSpeech() {
				ops = append(ops, raw)
			}
		}
		br.Operations = append(ops, br.Operations...)
	case BreillatAcceptanceBranch:
		var ops []assets.Operation
		for _, raw := range br.Operations {
			op := DecodeOp(raw)
			if op.Code == ActionQuestMark && (op.D1 == game.BreillatTalkMark || op.D1 == game.BreillatConversionMark) || op.Code == ActionActor && op.D1 == BreillatActor && op.D2 == ActorActionHide {
				continue
			}
			ops = append(ops, raw)
		}
		br.Operations = ops
	}
	return &result
}
