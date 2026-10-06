package world

import (
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
)

func TestWaterGatheringCompleteShapeValidation(t *testing.T) {
	build := func() assets.Event {
		return assets.Event{ClickID: 88, Branches: []assets.Branch{{Index: 2, Operations: []assets.Operation{action(1, 1, 1, 6, 7, 1), action(2, 14, 11009, 1, 0, 180)}}}}
	}
	ev := build()
	plan, involved := WaterGathering(60001, &ev, 0)
	if !involved || plan.Timer != 11009 || plan.Item != 60001 {
		t.Fatal(plan, involved)
	}
	ev.Branches[0].Index = 5
	ev.Branches[0].Operations = append(ev.Branches[0].Operations, action(3, 5, 11009, 1, 1, 1))
	if plan, involved := WaterGathering(60001, &ev, 0); !involved || plan.Timer != 11009 {
		t.Fatal(plan, involved)
	}
	for _, kind := range []string{"event", "branch", "variant", "timer", "duration", "extra mutation", "order", "mark"} {
		t.Run(kind, func(t *testing.T) {
			ev := build()
			switch kind {
			case "event":
				ev.ClickID = 89
			case "branch":
				ev.Branches[0].Index = 3
			case "variant":
				ev.Branches[0].Operations[0] = action(1, 1, 1, 6, 7, 2)
			case "timer":
				ev.Branches[0].Operations[1] = action(2, 14, 11016, 1, 0, 180)
			case "duration":
				ev.Branches[0].Operations[1] = action(2, 14, 11009, 1, 0, 181)
			case "extra mutation":
				ev.Branches[0].Operations = append(ev.Branches[0].Operations, action(3, 1, 1, 2, 0, 100))
			case "order":
				ev.Branches[0].Operations[0], ev.Branches[0].Operations[1] = ev.Branches[0].Operations[1], ev.Branches[0].Operations[0]
			case "mark":
				ev.Branches[0].Index = 5
				ev.Branches[0].Operations = append(ev.Branches[0].Operations, action(3, 5, 11016, 1, 1, 1))
			}
			if plan, involved := WaterGathering(60001, &ev, 0); !involved || plan.Timer != 0 {
				t.Fatal(plan, involved)
			}
		})
	}
	ordinary := assets.Event{Branches: []assets.Branch{{Operations: []assets.Operation{action(1, 1, 1, 1, 60001, 1)}}}}
	if _, involved := WaterGathering(60001, &ordinary, 0); involved {
		t.Fatal("ordinary item reward intercepted")
	}
	if !DecodeOp(action(1, 14, 11009, 1, 0, 180)).Unsupported() {
		t.Fatal("standalone timer action enabled without atomic branch")
	}
}

func TestWaterTimerBranchConditions(t *testing.T) {
	w := eventWorld()
	c := character()
	now := time.Now().UTC()
	c.EventTimers = map[uint16]time.Time{11009: now.Add(180 * time.Second)}
	for _, timer := range []uint16{11009, 11016, 11021, 11035} {
		active := w.conditionAt(c, nil, 60001, nil, cond(14, timer, 1, 0, 0, 0), now)
		inactive := w.conditionAt(c, nil, 60001, nil, cond(14, timer, 2, 0, 0, 0), now)
		if active != (timer == 11009) || inactive == active {
			t.Fatal(timer, active, inactive)
		}
	}
	if w.conditionAt(c, nil, 60001, nil, cond(14, 11009, 1, 0, 0, 0), now.Add(180*time.Second)) {
		t.Fatal("expired timer active")
	}
	if !w.conditionAt(c, nil, 60001, nil, cond(14, 11009, 2, 0, 0, 0), now.Add(180*time.Second)) {
		t.Fatal("expiry is not inclusive")
	}
	for _, invalid := range [][21]byte{cond(14, 9999, 2, 0, 0, 0), cond(14, 11009, 3, 0, 0, 0), cond(14, 11009, 2, 1, 0, 0), cond(14, 11009, 2, 0, 0, 1)} {
		if w.conditionAt(c, nil, 60001, nil, invalid, now) {
			t.Fatal("unknown timer operand matched", invalid)
		}
	}
}
