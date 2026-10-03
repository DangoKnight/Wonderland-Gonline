package game

import (
	"math"
	"reflect"
	"testing"
)

func TestPetRebirthPreservesProgressAndRefillsVitals(t *testing.T) {
	pet := Pet{ID: 14156, Slot: 3, Name: "Xaolan", Level: 100, Exp: 123, Amity: 100, StatPoints: 7, Base: Attributes{Strength: 83, Constitution: 61, Intelligence: 92, Wisdom: 71, Agility: 104}, Battle: true, Job: 2, Skills: []PetSkill{{ID: 11001, Grade: 5, Exp: 73}}}
	pet.Equipment[1] = Item{ID: 21001, Count: 1, Damage: 7}
	items := map[uint16]ItemDefinition{21001: {ID: 21001, Status: [2]uint16{207, 208}, Values: [2]int32{150, 130}}}
	before := pet
	if !pet.Rebirth(items) {
		t.Fatal("eligible pet rejected")
	}
	if !pet.Reborn || pet.Level != 1 || pet.Exp != 0 || pet.StatPoints != 57 || pet.Amity != 100 || pet.HP != pet.MaxHP || pet.SP != pet.MaxSP || pet.HP != pet.CalculatedMaxHP()+50 || pet.SP != pet.CalculatedMaxSP()+30 {
		t.Fatal(pet)
	}
	if pet.ID != before.ID || pet.Slot != 3 || pet.Name != before.Name || pet.Base != before.Base || pet.Equipment != before.Equipment || !pet.Battle || pet.Job != 2 || !reflect.DeepEqual(pet.Skills, before.Skills) {
		t.Fatal("rebirth discarded progress", pet)
	}
	after := pet
	if pet.Rebirth(items) || !reflect.DeepEqual(pet, after) {
		t.Fatal("rebirth repeated")
	}
}

func TestPetRebirthRejectionsAreAtomic(t *testing.T) {
	for _, pet := range []Pet{
		{ID: 1, Level: 99, Amity: 100}, {ID: 1, Level: 100, Amity: 99},
		{ID: 1, Level: 100, Amity: 100, Reborn: true}, {Level: 100, Amity: 100},
		{ID: 1, Level: 100, Amity: 100, StatPoints: math.MaxUint16 - 49},
	} {
		before := pet
		if pet.Rebirth(nil) || !reflect.DeepEqual(pet, before) {
			t.Fatal("invalid rebirth changed state", pet)
		}
	}
	pet := Pet{ID: 1, Level: 101, Amity: 101, StatPoints: math.MaxUint16 - 50}
	if !pet.Rebirth(nil) || pet.StatPoints != math.MaxUint16 || pet.Amity != 100 {
		t.Fatal("eligible boundary rejected", pet)
	}
}
