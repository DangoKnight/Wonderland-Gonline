package assetsql

import (
	"encoding/json"
	"gorm.io/gorm"
	"reflect"
	"strings"
	"testing"
	"wonderland-go/internal/assetdb"
	"wonderland-go/internal/assets"
)

func TestQuestDefinitionsSQLRoundTripValidationAndAuthority(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	q := assets.QuestDefinition{ID: 123, MapID: 10017, Title: "Authored bounty", Type: assets.QuestMonsterBattle, BattleMonsterID: 42, RequiredKillCount: 2, InProgressMarkID: 9, CompletedMarkID: 9, AllLinkedMarkIDs: []uint32{9}, PrerequisiteQuestIDs: []uint32{9}, Reward: assets.QuestReward{Gold: 1, EXP: 2, Items: []assets.QuestItem{{ItemID: 10002, Count: 2}}, CompanionID: 42}, RequiredItems: []assets.QuestItem{{ItemID: 10002, Count: 1}}, Repeatable: true, Daily: true, CooldownMinutes: 10, Steps: []assets.QuestStep{{Index: 1, Type: assets.QuestMonsterBattle, BattleMonsterID: 42, RequiredKillCount: 3, Reward: assets.QuestReward{Items: []assets.QuestItem{{ItemID: 10002, Count: 1}}}}}}
	defs := map[uint32]assets.QuestDefinition{q.ID: q}
	raw, _ := json.Marshal(defs)
	if err = db.Transaction(func(tx *gorm.DB) error {
		c, err := DefinitionCandidate(tx, "QuestDefinitions", raw, nil)
		if err != nil {
			return err
		}
		return SaveEditedCatalog(tx, c)
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Where("1 = 1").Delete(&assetdb.Document{}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := LoadDatabase(path)
	if err != nil || !reflect.DeepEqual(got.QuestDefinitions, defs) {
		t.Fatal("typed quest round trip", got, err)
	}
	for _, bad := range []assets.QuestDefinition{{ID: 123, Title: "Bad", BattleMonsterID: 65000}, {ID: 123, Title: "Bad", Steps: []assets.QuestStep{{Index: 2}}}, {ID: 123, Title: "Bad", Reward: assets.QuestReward{Items: []assets.QuestItem{{ItemID: 65000, Count: 1}}}}, {ID: 123, Title: "Bad", PrerequisiteQuestIDs: []uint32{123}}} {
		raw, _ := json.Marshal(map[uint32]assets.QuestDefinition{123: bad})
		if err = db.Transaction(func(tx *gorm.DB) error {
			c, err := DefinitionCandidate(tx, "QuestDefinitions", raw, nil)
			if err != nil {
				return err
			}
			return SaveEditedCatalog(tx, c)
		}); err == nil {
			t.Fatal("invalid definition accepted", bad)
		}
	}
	got, err = LoadDatabase(path)
	if err != nil || !reflect.DeepEqual(got.QuestDefinitions, defs) {
		t.Fatal("failed edit changed SQL", err)
	}
}
func TestQuestDefinitionsV9MigrationPreservesTypedEditsAndSeedsOnce(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = db.Model(&NativeItemsRow{}).Where("native_items_key = ?", 10002).Update("value_definition_name", "Keep edit").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Where("1 = 1").Delete(&QuestDefinitionsRow{}).Error; err != nil {
		t.Fatal(err)
	}
	models := catalogTables()
	for i := len(models) - 1; i >= 0; i-- {
		if named, ok := models[i].(interface{ TableName() string }); ok && strings.HasPrefix(named.TableName(), "catalog_quest_definitions") {
			if err = db.Migrator().DropTable(models[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = db.Migrator().DropColumn(&catalogPresence{}, "QuestDefinitions"); err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 9).Error; err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDatabase(path)
	if err != nil || got.Items[10002].Name != "Keep edit" || got.QuestDefinitions[9].Title != "SQL name" || got.QuestDefinitions[9].Reward.Gold != 0 {
		t.Fatal("additive migration/default metadata", got, err)
	}
	if err = db.Model(&QuestDefinitionsRow{}).Where("quest_definitions_key = ?", 9).Update("value_title", "Operator edit").Error; err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	got, err = LoadDatabase(path)
	if err != nil || got.QuestDefinitions[9].Title != "Operator edit" {
		t.Fatal("startup overwrote edit", err)
	}
}

func TestQuestDefinitionCrossDatasetEditCannotBreakReferences(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = db.Transaction(func(tx *gorm.DB) error {
		c, err := LoadTransaction(tx)
		if err != nil {
			return err
		}
		c.QuestDefinitions[123] = assets.QuestDefinition{ID: 123, Title: "Bounty", BattleMonsterID: 42, Type: assets.QuestMonsterBattle}
		return writeCatalog(tx, c)
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(func(tx *gorm.DB) error { _, err := DefinitionCandidate(tx, "NPCs", []byte(`{}`), nil); return err }); err == nil {
		t.Fatal("dangling quest NPC accepted")
	}
}

func TestQuestDefinitionNativeStepTextUsesNativeIDWithoutGuessedRewards(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	raw := `{"fields":{"id":9},"name":{"text":"Roca bounty"},"description":{"text":"#01Defeat the guard.#02Return to Roca.#99Done."}}`
	if err = db.Model(&assetdb.Record{}).Where("asset = ?", "mark.dat").Update("json", raw).Error; err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	q := got.QuestDefinitions[9]
	if q.ID != 9 || q.Reward.Gold != 0 || q.NPCTemplateID != 0 || q.MapID != 0 || len(q.Steps) != 2 || q.Steps[0].Type != assets.QuestDialogue || q.Steps[1].CompleteDialogue != "Done." {
		t.Fatal("native metadata inferred rules", q)
	}
}
