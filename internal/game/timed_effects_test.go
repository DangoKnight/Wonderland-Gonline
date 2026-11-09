package game

import "testing"

func TestTimedEffectsActivationAndClocks(t *testing.T) {
	statue := ItemDefinition{TimedKind: TimedItemStatue, TimedMinutes: 480, EXPTenths: 20}
	potion := ItemDefinition{TimedKind: TimedItemPotion, TimedMinutes: 120, EXPTenths: 100}
	for _, first := range []ItemDefinition{statue, potion} {
		var e TimedEffects
		if err := e.Activate(first); err != nil {
			t.Fatal(err)
		}
		before := e
		other := statue
		if first.TimedKind == TimedItemStatue {
			other = potion
		}
		if err := e.Activate(other); err != ErrTimedIncompatible || e != before {
			t.Fatal("incompatible activation changed state", err, e)
		}
		if first.TimedKind == TimedItemPotion {
			if err := e.Activate(first); err != ErrTimedAlreadyActive || e != before {
				t.Fatal("repeat potion", err)
			}
		} else {
			if err := e.Activate(first); err != nil || e.EXPSeconds != before.EXPSeconds*2 {
				t.Fatal("statue extension", err, e)
			}
		}
	}
	var e TimedEffects
	_ = e.Activate(potion)
	_ = e.Activate(ItemDefinition{TimedKind: TimedItemTraining, TimedMinutes: 60})
	e.Advance(60, false)
	if e.EXPSeconds != 7140 || e.TrainingSeconds != 3600 || e.Multiplier() != 10 {
		t.Fatal(e)
	}
	_ = e.Activate(ItemDefinition{TimedKind: TimedItemStopwatch})
	e.Advance(60, true)
	if e.EXPSeconds != 7140 || e.TrainingSeconds != 3540 || e.Multiplier() != 1 {
		t.Fatal(e)
	}
	_ = e.Activate(ItemDefinition{TimedKind: TimedItemStopwatch})
	e.Advance(10000, true)
	if e != (TimedEffects{}) {
		t.Fatal("expiry", e)
	}
}
