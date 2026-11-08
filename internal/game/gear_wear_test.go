package game

import "testing"

func TestCombatGearWearCurveAndExemptions(t *testing.T) {
	for _, v := range []struct {
		enemy  byte
		chance int
	}{{9, 0}, {10, 100}, {20, 1050}, {30, 2000}, {99, 2000}} {
		if got := CombatWearChance(10, v.enemy); got != v.chance {
			t.Fatal(v, got)
		}
	}
	items := map[uint16]ItemDefinition{1: {EquipSlot: 1}, 2: {EquipSlot: 2, Indestructible: true}}
	e := Equipment{{ID: 1, Count: 1, Damage: 99}, {ID: 2, Count: 1, Damage: 9}}
	if !e.ApplyCombatWear(items, 2000, 6, func(int, int) int { return 0 }) || e[0].Damage != 100 || e[1].Damage != 9 {
		t.Fatal(e)
	}
	before := e
	if e.ApplyCombatWear(items, 0, 100, func(int, int) int { t.Fatal("rolled against weaker enemy"); return 0 }) || e != before {
		t.Fatal(e)
	}
}
