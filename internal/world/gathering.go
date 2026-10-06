package world

import (
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

const (
	waterRewardKind         = 6
	waterTimerStart         = 1
	waterGatheringMarkState = 1
	waterGatheringMarkStep  = 1
	seaWaterEvent60001      = 88
	seaWaterEvent60003      = 48
	freshWaterEvent12268    = 2
	freshWaterEvent60005    = 12
	waterRewardPool         = 7
	seaWaterVariant         = 1
	freshWaterVariant       = 2
	waterRewardBranch       = 2
	waterMarkedRewardBranch = 5
)

type WaterGatheringPlan struct{ Timer, Item uint16 }

func waterReward(op Op) bool {
	return op.Code == ActionPlayer && op.D1 == PlayerActionReward && op.D2 == waterRewardKind && op.D3 == waterRewardPool && (op.Value() == seaWaterVariant || op.Value() == freshWaterVariant)
}
func waterTimer(op Op) bool {
	return op.Code == ActionGather && game.IsWaterGatheringTimer(op.D1) && op.D2 == waterTimerStart && op.D3 == 0 && op.Value() == uint32(game.WaterGatheringCooldown.Seconds())
}

// WaterGathering recognizes the complete authored branch from EveGatheringRuntime.
// involved is true even if a water action occurs in an invalid shape; callers must
// refuse that entire branch before any ordinary rewards or mutations execute.
func WaterGathering(mapID uint16, ev *assets.Event, branch int) (plan WaterGatheringPlan, involved bool) {
	if ev == nil || branch < 0 || branch >= len(ev.Branches) {
		return plan, false
	}
	br := ev.Branches[branch]
	for _, raw := range br.Operations {
		op := DecodeOp(raw)
		if waterReward(op) || waterTimer(op) {
			involved = true
		}
	}
	if !involved {
		return plan, false
	}
	switch {
	case mapID == game.MapID60001 && ev.ClickID == seaWaterEvent60001:
		plan.Timer = game.WaterGatheringTimer11009
	case mapID == game.MapID60003 && ev.ClickID == seaWaterEvent60003:
		plan.Timer = game.WaterGatheringTimer11016
	case mapID == game.MapID12268 && ev.ClickID == freshWaterEvent12268:
		plan.Timer = game.WaterGatheringTimer11021
	case mapID == game.MapID60005 && ev.ClickID == freshWaterEvent60005:
		plan.Timer = game.WaterGatheringTimer11035
	default:
		return WaterGatheringPlan{}, true
	}
	plan.Item, _ = game.WaterGatheringItem(plan.Timer)
	variant := uint32(freshWaterVariant)
	if plan.Timer == game.WaterGatheringTimer11009 || plan.Timer == game.WaterGatheringTimer11016 {
		variant = seaWaterVariant
	}
	valid := br.Index == waterRewardBranch && len(br.Operations) == 2 || br.Index == waterMarkedRewardBranch && len(br.Operations) == 3
	if valid {
		reward, timer := DecodeOp(br.Operations[0]), DecodeOp(br.Operations[1])
		valid = waterReward(reward) && reward.Value() == variant && waterTimer(timer) && timer.D1 == plan.Timer
	}
	if valid && len(br.Operations) == 3 {
		mark := DecodeOp(br.Operations[2])
		valid = mark.Code == ActionQuestMark && mark.D1 == plan.Timer && mark.D2 == waterGatheringMarkState && mark.D3 == waterGatheringMarkStep && mark.Value() == waterGatheringMarkStep
	}
	if !valid {
		return WaterGatheringPlan{}, true
	}
	return plan, true
}
