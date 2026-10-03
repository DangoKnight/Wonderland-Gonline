package game

import (
	"math"
	"reflect"
	"testing"
)

func TestPetTrainingPointAllocation(t *testing.T) {
	pet := Pet{StatPoints: 5, Base: Attributes{Strength: 10, Constitution: 20, Intelligence: 30, Wisdom: 40, Agility: 50}}
	for _, stat := range []byte{28, 29, 27, 33, 30} {
		if !pet.AllocatePoint(stat) {
			t.Fatal("valid point rejected", stat)
		}
	}
	if pet.StatPoints != 0 || pet.Base != (Attributes{Strength: 11, Constitution: 21, Intelligence: 31, Wisdom: 41, Agility: 51}) {
		t.Fatal(pet)
	}
	before := pet
	if pet.AllocatePoint(28) || !reflect.DeepEqual(pet, before) {
		t.Fatal("empty budget spent")
	}
}

func TestPetTrainingPointRejectionsAreAtomic(t *testing.T) {
	for _, stat := range []byte{0, 1, 255, 28, 29, 27, 33, 30} {
		pet := Pet{StatPoints: 2, Base: Attributes{Strength: math.MaxUint16, Constitution: math.MaxUint16, Intelligence: math.MaxUint16, Wisdom: math.MaxUint16, Agility: math.MaxUint16}}
		before := pet
		if pet.AllocatePoint(stat) || !reflect.DeepEqual(pet, before) {
			t.Fatal("invalid training spent points", stat, pet)
		}
	}
	pet := Pet{StatPoints: 1, Base: Attributes{Strength: math.MaxUint16 - 1}}
	if !pet.AllocatePoint(28) || pet.Base.Strength != math.MaxUint16 || pet.StatPoints != 0 {
		t.Fatal("boundary allocation rejected", pet)
	}
}
