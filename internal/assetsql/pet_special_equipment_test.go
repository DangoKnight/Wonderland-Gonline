package assetsql

import (
	"encoding/json"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"

	"gorm.io/gorm"
)

func TestPetSpecialEquipmentAuthoredMappings(t *testing.T) {
	want := map[uint32]assets.StoryEquipment{
		14243: {Slot: 5, ItemID: 25537}, 12148: {Slot: 2, ItemID: 12503}, 17914: {Slot: 5, ItemID: 25534},
		14175: {Slot: 2, ItemID: 18065}, 14230: {Slot: 2, ItemID: 20044}, 14242: {Slot: 2, ItemID: 12085},
		12095: {Slot: 2, ItemID: 11109}, 15689: {Slot: 2, ItemID: 10075}, 35253: {Slot: 2, ItemID: 11143},
		14548: {Slot: 2, ItemID: 13067}, 12147: {Slot: 2, ItemID: 20046}, 12068: {Slot: 2, ItemID: 11100},
		14081: {Slot: 2, ItemID: 14073}, 14615: {Slot: 2, ItemID: 14074}, 14162: {Slot: 2, ItemID: 10063},
		14274: {Slot: 2, ItemID: 11119}, 14637: {Slot: 2, ItemID: 15059}, 12149: {Slot: 5, ItemID: 25630},
		15657: {Slot: 2, ItemID: 11124}, 14156: {Slot: 5, ItemID: 25028},
	}
	var defs []assets.StoryCompanionEquipment
	if err := json.Unmarshal(petSpecialEquipmentDefaults, &defs); err != nil {
		t.Fatal(err)
	}
	if len(defs) != len(want) {
		t.Fatal("incorrect starter count", len(defs))
	}
	for _, def := range defs {
		gear, ok := want[def.Template]
		if !ok || len(def.Equipment) != 1 || def.Equipment[0] != gear {
			t.Fatal("incorrect normal equipment", def)
		}
		delete(want, def.Template)
	}
}

func TestPetSpecialEquipmentMigrationPreservesEditsAndOptOut(t *testing.T) {
	defs := assets.DefaultStoryDefinitions()
	defs.CompanionEquipment = append(defs.CompanionEquipment, assets.StoryCompanionEquipment{Template: 14081, Equipment: []assets.StoryEquipment{{Slot: 2, ItemID: 12345}}})
	// Deliberately removed historical defaults must not be restored.
	defs.CompanionEquipment = defs.CompanionEquipment[1:]
	db := storyReadFixture(t, defs)
	if err := db.Transaction(migratePetSpecialEquipment); err != nil {
		t.Fatal(err)
	}
	after, err := readStory(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.CompanionEquipment) != 20 {
		t.Fatal("missing or duplicate starter entries", len(after.CompanionEquipment))
	}
	if !reflect.DeepEqual(after.CompanionEquipment[:3], defs.CompanionEquipment) {
		t.Fatal("edited starter rows overwritten")
	}
	if err := db.Transaction(migratePetSpecialEquipment); err != nil {
		t.Fatal(err)
	}
	again, err := readStory(db)
	if err != nil || !reflect.DeepEqual(after, again) {
		t.Fatal("migration not idempotent", err)
	}
	for _, empty := range [][]assets.StoryCompanionEquipment{nil, {}} {
		emptyDB := storyReadFixture(t, assets.StoryDefinitions{CompanionEquipment: empty})
		if err := emptyDB.Transaction(migratePetSpecialEquipment); err != nil {
			t.Fatal(err)
		}
		got, err := readStory(emptyDB)
		if err != nil || !reflect.DeepEqual(got.CompanionEquipment, empty) {
			t.Fatal("intentional opt-out lost", err)
		}
	}
	rollbackDB := storyReadFixture(t, assets.DefaultStoryDefinitions())
	if err := rollbackDB.Transaction(func(tx *gorm.DB) error {
		if err := migratePetSpecialEquipment(tx); err != nil {
			return err
		}
		return gorm.ErrInvalidTransaction
	}); err == nil {
		t.Fatal("expected rollback")
	}
	got, err := readStory(rollbackDB)
	if err != nil || !reflect.DeepEqual(got, assets.DefaultStoryDefinitions()) {
		t.Fatal("partial migration survived rollback", err)
	}
}
