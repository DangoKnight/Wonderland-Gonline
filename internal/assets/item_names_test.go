package assets

import (
	"testing"
	"wonderland-gonline/internal/game"
)

func TestItemNamePatchesPreserveNativeDataAndEdits(t *testing.T) {
	patches, err := ItemNamePatches()
	if err != nil || len(patches) != 696 {
		t.Fatal(len(patches), err)
	}
	for id, name := range map[uint16]string{10034: "Pure Iron Knife", 10036: "Copper Falchion", 10039: "Pure Iron Falchion"} {
		if patches[id].Name != name {
			t.Fatal(id, patches[id])
		}
	}
	if _, exists := patches[15053]; exists {
		t.Fatal("+1 expanded to +10")
	}
	if _, exists := patches[14076]; exists {
		t.Fatal("complete name expanded to enhanced variant")
	}
	items := map[uint16]NativeItem{
		10034: {Definition: game.ItemDefinition{ID: 10034, Name: "Pure Iron Knif"}, Record: [451]byte{42}, Description: "Keep description"},
		10036: {Definition: game.ItemDefinition{ID: 10036, Name: "Custom sword"}},
	}
	before := items[10034]
	for range 2 {
		if err := ApplyItemNamePatches(items); err != nil {
			t.Fatal(err)
		}
	}
	if items[10034].Definition.Name != "Pure Iron Knife" || items[10034].Record != before.Record || items[10034].Description != before.Description || items[10036].Definition.Name != "Custom sword" || len(items) != 2 {
		t.Fatal(items)
	}
}
