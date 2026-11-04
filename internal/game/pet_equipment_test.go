package game

import "testing"

func TestFreshPetStartingEquipmentAndVitals(t *testing.T) {
	items := map[uint16]ItemDefinition{25028: {ID: 25028, EquipSlot: 6, Status: [2]uint16{207, 208}, Values: [2]int32{140, 120}}}
	tpl := PetTemplate{Stats: Attributes{8, 6, 9, 7, 10}, StartingEquipment: Equipment{5: {ID: 25028, Count: 50, Damage: 99}}}
	for _, pet := range []Pet{NewPet(14156, "Xaolan", 1, tpl, items), NewCapturedPet(14156, "Xaolan", 1, tpl, items, func(int) int { return 0 })} {
		if pet.Equipment[5] != (Item{ID: 25028, Count: 1}) || pet.HP != pet.MaxHP || pet.SP != pet.MaxSP {
			t.Fatal("fresh gear or vitals incorrect", pet)
		}
		if pet.MaxHP != pet.CalculatedMaxHP(tpl)+40 || pet.MaxSP != pet.CalculatedMaxSP(tpl)+20 {
			t.Fatal("gear bonuses missing from fresh vitals", pet)
		}
	}
	tpl.StartingEquipment = Equipment{0: {ID: 25028}, 1: {ID: 9999}}
	if pet := NewPet(14156, "Xaolan", 1, tpl, items); pet.Equipment != (Equipment{}) {
		t.Fatal("invalid or missing equipment granted", pet.Equipment)
	}
}
