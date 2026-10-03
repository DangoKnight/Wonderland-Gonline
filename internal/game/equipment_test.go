package game

import (
	"encoding/json"
	"testing"
)

func TestEquipmentLegacyJSON(t *testing.T) {
	var c Character
	if err := json.Unmarshal([]byte(`{"equipment":[0,21001,0,0,24001,0]}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Equipment[1] != (Item{ID: 21001, Count: 1}) || c.Equipment[4].ID != 24001 || c.Equipment[0].ID != 0 {
		t.Fatal(c.Equipment)
	}
	c.Equipment[1].Damage, c.Equipment[1].Metadata[18] = 3, 2
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var back Character
	if err := json.Unmarshal(data, &back); err != nil || back.Equipment != c.Equipment {
		t.Fatal("round trip", err)
	}
	if err := json.Unmarshal([]byte(`{"equipment":[1,2]}`), &back); err == nil {
		t.Fatal("accepted short equipment")
	}
}

func TestCombatAndBonuses(t *testing.T) {
	items := map[uint16]ItemDefinition{
		30000: {ID: 30000, EquipSlot: 3, Status: [2]uint16{210, 0}, Values: [2]int32{105, 0}},
		30001: {ID: 30001, EquipSlot: 2, Status: [2]uint16{211, 207}, Values: [2]int32{95, 120}},
	}
	// Body 1 head 0 adds INT 2 and AGI 1 to a base of 5.
	c := Character{Level: 1, Element: Fire, Body: 1, Base: Attributes{5, 5, 5, 5, 5}}
	if got := c.Combat(items); got != (Combat{201, 121, 12, 12, 16, 12, 15}) {
		t.Fatalf("base: %+v", got)
	}
	weapon := Item{ID: 30000, Count: 1}
	weapon.Metadata[18] = 2 // A single forged status gains two per level.
	c.Equipment[2] = weapon
	c.Equipment[1] = Item{ID: 30001, Count: 1}
	got := c.Combat(items)
	// The DEF penalty wraps like the C# UInt16 total; HP adds the second status.
	if got.ATK != 21 || got.DEF != 12+65531 || got.MaxHP != 221 {
		t.Fatalf("equipped: %+v", got)
	}
	if b := ChangeBanner("Equipment: ", Combat{ATK: 12, SPD: 15}, Combat{ATK: 21, SPD: 14}); b != "Equipment: ATK +9, SPD -1" {
		t.Fatal(b)
	}
	if ChangeBanner("x", got, got) != "" {
		t.Fatal("banner without changes")
	}
}

func TestWearAndUnwear(t *testing.T) {
	items := map[uint16]ItemDefinition{1: {ID: 1, EquipSlot: 3}, 2: {ID: 2, EquipSlot: 3}, 3: {ID: 3}}
	var c Character
	c.Equipment[2] = Item{ID: 1, Count: 1, Damage: 9}
	c.Bag[4] = Item{ID: 2, Count: 1}
	c.Bag[5] = Item{ID: 3, Count: 1}
	c.Bag[6] = Item{ID: 2, Count: 2}
	if c.Wear(5, items) != nil || c.Equipment[2].ID != 2 || c.Bag[4] != (Item{ID: 1, Count: 1, Damage: 9}) {
		t.Fatal("swap", c.Equipment, c.Bag[4])
	}
	if c.Wear(6, items) == nil || c.Wear(7, items) == nil || c.Wear(9, items) == nil {
		t.Fatal("wore an unequippable, stacked or empty slot")
	}
	if c.Unwear(3, 5) == nil {
		t.Fatal("removed onto an occupied slot")
	}
	if c.Unwear(3, 10) != nil || c.Equipment[2].ID != 0 || c.Bag[9].ID != 2 || c.Unwear(3, 11) == nil {
		t.Fatal("remove")
	}
}

func TestLevelProgress(t *testing.T) {
	if LevelExp(1) != 14 || LevelForExp(6) != 1 || LevelForExp(13) != 1 || LevelForExp(14) != 2 {
		t.Fatal(LevelExp(1), LevelForExp(14))
	}
	c := Character{Level: 1, EXP: 6}
	need := uint64(14 - 6 + LevelExp(2))
	if got := c.AddExp(need); got != 2 || c.Level != 3 || c.StatPoints != 6 {
		t.Fatal(got, c.Level, c.StatPoints)
	}
	if c.AddExp(1<<40) == 0 || c.Level != MaxLevel || c.EXP != 1<<32-1 {
		t.Fatal("level cap", c.Level, c.EXP)
	}
	if p := c.ExpPacket(); len(p) != 12 || p[2] != 36 {
		t.Fatal(p)
	}
}

func TestStatAllocation(t *testing.T) {
	for _, tc := range []struct {
		sub    byte
		data   []byte
		target byte
		want   []StatAllocation
	}{
		{StatSTR, nil, 0, []StatAllocation{{StatSTR, 1}}},
		{StatCON, []byte{3, 0}, 0, []StatAllocation{{StatCON, 3}}},
		{1, []byte{StatWIS, 2}, 0, []StatAllocation{{StatWIS, 2}}},
		{2, []byte{0, StatAGI}, 0, []StatAllocation{{StatAGI, 1}}},
		{2, []byte{0, 2, StatSTR, 1, 0, 0, 0, StatINT, 2, 0}, 0, []StatAllocation{{StatSTR, 1}, {StatINT, 2}}},
		{2, []byte{3, StatSTR, 1}, 3, []StatAllocation{{StatSTR, 1}}},
		{9, []byte{StatSTR}, 0, nil},
	} {
		target, got := ParseStatAllocation(tc.sub, tc.data)
		if target != tc.target || len(got) != len(tc.want) {
			t.Fatalf("%v: %v %v", tc.data, target, got)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%v: %v", tc.data, got)
			}
		}
	}
	c := Character{StatPoints: 3}
	if !c.Allocate([]StatAllocation{{StatSTR, 2}, {StatCON, 2}, {StatAGI, 1}}) || c.Base.Strength != 2 || c.Base.Constitution != 0 || c.Base.Agility != 1 || c.StatPoints != 0 {
		t.Fatal(c.Base, c.StatPoints)
	}
	if c.Allocate([]StatAllocation{{StatSTR, 1}}) {
		t.Fatal("allocated without points")
	}
}
