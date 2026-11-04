package assetsql

import (
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestNPCNameMigrationPreservesCustomNamesAndMissingRecords(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	for _, row := range []NPCsRow{
		{NPCsKey: 11000, ValueID: 11000, ValueName: "Persian Ca", ValueHP: 123},
		{NPCsKey: 17003, ValueID: 17003, ValueName: "Custom grape"},
		{NPCsKey: 11004, ValueID: 11004, ValueName: "Persian Ca"},
		{NPCsKey: 17001, ValueID: 17001, ValueName: "Pineapple"},
		{NPCsKey: 30310, ValueID: 30310, ValueName: "Yellow Roc"},
		{NPCsKey: 12073, ValueID: 12073, ValueName: "Cool Ronin (Quest)"},
		{NPCsKey: 17250, ValueID: 17250, ValueName: "Custom bear (Quest)"},
		{NPCsKey: 17466, ValueID: 17466, ValueName: "Snake", ValueHP: 62},
		{NPCsKey: 17834, ValueID: 17834, ValueName: "Custom chameleon"},
	} {
		if err := db.Save(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&catalogSchema{}).Where("id = ?", 1).Update("version", 26).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := MigrateDatabase(path); err != nil {
			t.Fatal(err)
		}
	}
	npcs, err := readNPCs(db)
	if err == nil && npcs[17001].Name != "Pineapple Monster" {
		t.Fatal("short-name owner choice not migrated")
	}
	if err == nil && npcs[30310].Name != "Yellow Rock Monster" {
		t.Fatal("owner choice not migrated")
	}
	if err == nil && (npcs[17466].Name != "Poisonous Snake" || npcs[17466].HP != 62 || npcs[17834].Name != "Custom chameleon") {
		t.Fatal("reviewed name migration overwrote unrelated fields or operator names")
	}
	if err != nil || npcs[11000].Name != "Persian Cat" || npcs[11000].HP != 123 || npcs[17003].Name != "Custom grape" || npcs[11004].Name != "Persian Cat" || npcs[12073].Name != "Cool Ronin" || npcs[17250].Name != "Custom bear (Quest)" {
		t.Fatal(npcs, err)
	}
	if _, exists := npcs[17067]; exists {
		t.Fatal("migration created absent monster")
	}
}

func TestNPCNameInitialization(t *testing.T) {
	c := &assets.Catalog{NPCs: map[uint16]assets.NPC{17067: {ID: 17067, Name: "Blue Beetl"}}}
	if err := initializeNPCNames(c); err != nil || c.NPCs[17067].Name != "Blue Beetle" {
		t.Fatal(c.NPCs, err)
	}
}
