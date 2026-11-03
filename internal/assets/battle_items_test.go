package assets

import (
	"testing"
	"wonderland-gonline/internal/game"
)

func TestNativeBattleItemProjection(t *testing.T) {
	it := NativeItem{Definition: game.ItemDefinition{ID: 32003, Type: 23, EquipSlot: 8, Status: [2]uint16{25, 26}, Values: [2]int32{180, 120}}}
	if got, ok := it.BattleItem(); !ok || got != (BattleItem{ID: 32003, HP: 80, SP: 20}) {
		t.Fatal(got, ok)
	}
	it.Definition.Type = 24
	if _, ok := it.BattleItem(); ok {
		t.Fatal("battle item missing skill accepted")
	}
	// Independent native record bytes, offset 134: linked Secular Feather 15138.
	it.Record[134], it.Record[135] = 34, 59
	if got, ok := it.BattleItem(); !ok || got.Skill != 15138 {
		t.Fatal(got, ok)
	}
	it.Definition.EquipSlot = 6
	if _, ok := it.BattleItem(); ok {
		t.Fatal("equipment accepted as food")
	}
}

func TestNativeThrowableBattleItems(t *testing.T) {
	// Independent values/skill bytes from the translated Item.dat records.
	for _, tc := range []struct {
		id         uint16
		value      int32
		skillBytes [2]byte
		skill      uint16
		damage     int
	}{
		{33012, 40, [2]byte{29, 39}, 10013, 60}, // Bomb
		{33013, 60, [2]byte{30, 39}, 10014, 40}, // Darts
		{33014, 30, [2]byte{29, 39}, 10013, 70}, // Strong Bomb
		{33015, 50, [2]byte{30, 39}, 10014, 50}, // 4-Way Shuriken
		{33016, 40, [2]byte{30, 39}, 10014, 60}, // 8-Way Shuriken
	} {
		it := NativeItem{Definition: game.ItemDefinition{ID: tc.id, Type: 24, EquipSlot: 8, Status: [2]uint16{25, 0}, Values: [2]int32{tc.value, 100}}}
		it.Record[134], it.Record[135] = tc.skillBytes[0], tc.skillBytes[1]
		if got, ok := it.BattleItem(); !ok || got.Skill != tc.skill || got.HP != -tc.damage {
			t.Fatal(tc.id, got, ok)
		}
	}
}
