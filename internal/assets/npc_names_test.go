package assets

import (
	"strings"
	"testing"
)

func TestNPCNamePatchesVerifiedNamesAndEdits(t *testing.T) {
	patches, err := NPCNamePatches()
	if err != nil || len(patches) != 1076 {
		t.Fatal(len(patches), err)
	}
	for id, want := range map[uint16]string{17004: "Fragrance Monster", 17411: "Floating Monster", 17466: "Poisonous Snake", 17834: "Ancient Chameleon", 17712: "Snow Door God", 30218: "Poisonous Flower", 40121: "Pineapple Monster", 17034: "Sky Dragonfly", 17826: "Ruka the Bird God", 40325: "Red Brow Mammoth"} {
		p := patches[id]
		if p.Name != want || p.Apply(p.OriginalName) != want || p.Apply("Custom name") != "Custom name" {
			t.Fatal("reviewed creature identity lost", id, p)
		}
	}
	for _, id := range []uint16{17230, 12083, 17281, 40103, 19252, 20503, 17246} {
		if _, exists := patches[id]; exists {
			t.Fatal("unverified spelling, context or species expanded", id)
		}
	}
	for id, want := range map[uint16]string{11000: "Persian Cat", 17001: "Pineapple Monster", 17003: "Grape Monster", 17067: "Blue Beetle", 17414: "Hot-blooded Starfish"} {
		p := patches[id]
		if p.Name != want || p.Apply(p.OriginalName) != want || p.Apply(want) != want || p.Apply("Custom name") != "Custom name" {
			t.Fatal(id, p)
		}
	}
	if patches[11004].Name != "Persian Cat" {
		t.Fatal("accepted suggestion missing")
	}
	for id, name := range map[uint16]string{14469: "Inspecting Soldier", 17738: "Half Beast Man", 17739: "Half Beast Man", 30310: "Yellow Rock Monster", 30334: "Yellow Rock Monster", 40089: "Tyrannosaur Lord"} {
		if patches[id].Name != name {
			t.Fatal("owner choice missing", id, patches[id])
		}
	}
	for id, p := range patches {
		if strings.Contains(strings.ToLower(p.Name), "quest") {
			t.Fatal("Quest annotation retained", id, p)
		}
		if p.PreviousName != "" && p.Apply(p.PreviousName) != p.Name {
			t.Fatal("old display name not upgraded", id)
		}
	}
}

func TestNPCNameOwnerReviewCertaintyAndShorterNames(t *testing.T) {
	patches, err := NPCNamePatches()
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[uint16]string{11008: "Little Persian Cat", 11013: "Tabby Cat", 12072: "Old Man", 14056: "Drunk Man", 12113: "Tutankhamun", 14493: "Terracotta Soldier", 14498: "Terracotta General", 12046: "Pirate", 14438: "Monk"} {
		p, known := patches[id]
		if !known || p.Name != want || p.Apply(p.OriginalName) != want || p.Apply("Custom keeper") != "Custom keeper" {
			t.Fatal(id, p)
		}
	}
	for _, id := range []uint16{12165, 14223, 15357} {
		if _, known := patches[id]; known {
			t.Fatal("tentative review applied", id)
		}
	}
}
