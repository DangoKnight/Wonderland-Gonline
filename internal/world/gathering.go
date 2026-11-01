package world

import (
	"slices"
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

type EventGatheringPlan struct {
	Timer uint16
	Pools [][]uint16
}

// PlanEventGathering recognizes a complete reward/cooldown branch. An unknown
// pool or a partial shape stays involved, so the ordinary interpreter cannot
// grant any preceding rewards without acquiring the node's timer.
func (w *World) PlanEventGathering(mapID uint16, ev *assets.Event, branch int) (EventGatheringPlan, bool) {
	if w.catalog.EventGathering.Timers == nil {
		old, involved := WaterGathering(mapID, ev, branch)
		if old.Timer == 0 {
			return EventGatheringPlan{}, involved
		}
		return EventGatheringPlan{Timer: old.Timer, Pools: [][]uint16{{old.Item}}}, true
	}
	var plan EventGatheringPlan
	if ev == nil || branch < 0 || branch >= len(ev.Branches) {
		return plan, false
	}
	ops := ev.Branches[branch].Operations
	involved := false
	for _, raw := range ops {
		o := DecodeOp(raw)
		if o.Code == ActionGather || o.Code == ActionPlayer && o.D1 == PlayerActionReward && o.D2 == waterRewardKind {
			involved = true
		}
	}
	if !involved {
		return plan, false
	}
	i := 0
	for i < len(ops) {
		o := DecodeOp(ops[i])
		if o.Code != ActionPlayer || o.D1 != PlayerActionReward || o.D2 != waterRewardKind {
			break
		}
		pool := w.catalog.EventGatheringPool(o.D3, o.Value())
		if len(pool) == 0 {
			return EventGatheringPlan{}, true
		}
		plan.Pools = append(plan.Pools, pool)
		i++
	}
	if i == 0 || i >= len(ops) || len(ops) > i+2 {
		return EventGatheringPlan{}, true
	}
	timer := DecodeOp(ops[i])
	if timer.Code != ActionGather || !slices.Contains(w.catalog.EventGathering.Timers, timer.D1) || timer.D2 != waterTimerStart || timer.D3 != 0 || timer.Value() != uint32(game.WaterGatheringCooldown.Seconds()) {
		return EventGatheringPlan{}, true
	}
	if len(ops) == i+2 {
		mark := DecodeOp(ops[i+1])
		if mark.Code != ActionQuestMark || mark.D1 != timer.D1 || mark.D2 != waterGatheringMarkState || mark.D3 != waterGatheringMarkStep || mark.Value() != waterGatheringMarkStep {
			return EventGatheringPlan{}, true
		}
	}
	plan.Timer = timer.D1
	return plan, true
}

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
