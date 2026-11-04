package world

import (
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestPetStartingEquipmentUsesCatalogAndAliases(t *testing.T) {
	c := &assets.Catalog{NPCs: map[uint16]assets.NPC{14161: {ID: 14161}, 14162: {ID: 14162}}, Items: map[uint16]game.ItemDefinition{10063: {ID: 10063, EquipSlot: 3}}, Story: assets.StoryDefinitions{CompanionEquipment: []assets.StoryCompanionEquipment{{Template: 14162, Equipment: []assets.StoryEquipment{{Slot: 2, ItemID: 10063}}}}}}
	w := New(c)
	for _, id := range []uint32{14161, 14162} {
		tpl, ok := w.Template(id)
		if !ok || tpl.StartingEquipment != (game.Equipment{2: {ID: 10063, Count: 1}}) {
			t.Fatal("catalog equipment alias lost", id, tpl)
		}
	}
}
