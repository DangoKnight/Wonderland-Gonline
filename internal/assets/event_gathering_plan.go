package assets

import (
	"encoding/binary"
	"slices"
	"time"
)

// Explicit native EVE compatibility values used by the collection validator.
const (
	ActionPlayer            = 1
	ActionQuestMark         = 5
	ActionGather            = 14
	PlayerActionReward      = 1
	PlayerActionSpeech      = 2
	waterRewardKind         = 6
	waterTimerStart         = 1
	waterGatheringMarkState = 1
	waterGatheringMarkStep  = 1
)

type EventGatheringPlan struct {
	Timer    uint16
	Pools    [][]uint16
	Cooldown time.Duration
}
type gatheringOp struct {
	Code           byte
	D1, D2, D3, D4 uint16
	dw1            uint32
}

func decodeGatheringOp(o Operation) gatheringOp {
	d := o.Data[:]
	return gatheringOp{d[0], binary.LittleEndian.Uint16(d[1:]), binary.LittleEndian.Uint16(d[3:]), binary.LittleEndian.Uint16(d[5:]), binary.LittleEndian.Uint16(d[7:]), binary.LittleEndian.Uint32(d[9:])}
}
func (o gatheringOp) Value() uint32  { return uint32(o.D4>>8) | o.dw1<<8 }
func (o gatheringOp) TalkID() uint32 { return uint32(o.D2) | uint32(o.D3)<<16 }
func (c *Catalog) PlanEventGathering(ev *Event, branch int) (EventGatheringPlan, bool) {
	var plan EventGatheringPlan
	if ev == nil || branch < 0 || branch >= len(ev.Branches) {
		return plan, false
	}
	ops := ev.Branches[branch].Operations
	involved := false
	for _, raw := range ops {
		o := decodeGatheringOp(raw)
		if o.Code == ActionGather || o.Code == ActionPlayer && o.D1 == PlayerActionReward && o.D2 == waterRewardKind {
			involved = true
		}
	}
	if !involved {
		return plan, false
	}
	i := 0
	for i < len(ops) {
		o := decodeGatheringOp(ops[i])
		if o.Code != ActionPlayer || o.D1 != PlayerActionReward || o.D2 != waterRewardKind {
			break
		}
		pool := c.EventGatheringPool(o.D3, o.Value())
		if len(pool) == 0 {
			return EventGatheringPlan{}, true
		}
		plan.Pools = append(plan.Pools, pool)
		i++
	}
	if i == 0 || i >= len(ops) || len(ops) > i+2 {
		return EventGatheringPlan{}, true
	}
	timer := decodeGatheringOp(ops[i])
	if timer.Code != ActionGather || !slices.Contains(c.EventGathering.Timers, timer.D1) || timer.D2 != waterTimerStart || timer.D3 != 0 || timer.Value() == 0 {
		return EventGatheringPlan{}, true
	}
	if len(ops) == i+2 {
		mark := decodeGatheringOp(ops[i+1])
		if mark.Code != ActionQuestMark || mark.D1 != timer.D1 || mark.D2 != waterGatheringMarkState || mark.D3 != waterGatheringMarkStep || mark.Value() != waterGatheringMarkStep {
			return EventGatheringPlan{}, true
		}
	}
	plan.Timer = timer.D1
	plan.Cooldown = time.Duration(timer.Value()) * time.Second
	return plan, true
}
