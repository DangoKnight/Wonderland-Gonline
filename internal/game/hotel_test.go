package game

import (
	"encoding/hex"
	"testing"
	"wonderland-gonline/internal/protocol"
)

func TestHotelRecordGolden(t *testing.T) {
	p := Pet{Slot: 2, ID: 12032, Name: "Robinson", Level: 1, HP: 100, SP: 20, Base: Attributes{Intelligence: 1, Strength: 2, Constitution: 3, Agility: 4, Wisdom: 5}, Reborn: true, Job: 2, StatPoints: 9}
	p.Equipment[0] = Item{ID: 10063, Count: 1}
	want := "02922f060000000164000000140001000200030004000500000001090008526f62696e736f6e0000000000000000000000000000004f270000000000000000000001000200000000"
	// Robinson's native pet template alias is 12178 (0x2f92).
	got := hex.EncodeToString(p.HotelRecord(protocol.Builder{}, PetTemplate{}))
	if got != want {
		t.Fatalf("hotel record\n got %s\nwant %s", got, want)
	}
	p.Skills = []PetSkill{{ID: 11001, Grade: 1}}
	original := Character{HotelPets: []Pet{p}}
	clone := original.Clone()
	clone.HotelPets[0].Name = "Changed"
	clone.HotelPets[0].Skills[0].Grade = 9
	if original.HotelPets[0].Name != "Robinson" || original.HotelPets[0].Skills[0].Grade != 1 {
		t.Fatal("clone mutated original")
	}
}

func TestHotelRecordPreservesNativePotentialAndAmity(t *testing.T) {
	p := Pet{Slot: 1, ID: 17004, Name: "Fragrance", Level: 5, Potential: 9, Amity: 64, Reborn: true, Job: 2}
	packet := p.HotelRecord(protocol.Builder{}, PetTemplate{})
	// Header 25 is amity; tail 57/58/59 is rebirth/potential/job.
	// Native effective level uses tail 57 as level + 100*rebirth.
	if packet[25] != 64 || packet[26] != 1 || packet[57+len(p.Name)] != 1 || packet[58+len(p.Name)] != 9 || packet[59+len(p.Name)] != 2 {
		t.Fatal("hotel progression lost", packet)
	}
}
