package assetsql

import (
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestBattleBackdropV49Migration(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = db.Migrator().DropTable(&BattleBackdropsRegionsRow{}, &BattleBackdropsRow{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 49).Error; err != nil {
		t.Fatal(err)
	}
	// Independent bytes, not emitted by the decoder: default 80 and lake rectangle.
	data := []byte{20, 0, 0, 0, 20, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 100, 80, 1, 91, 10, 0, 20, 0, 44, 1, 144, 1}
	raw, _ := json.Marshal(map[string]any{"name": "10017.map", "decoded_hex": hex.EncodeToString(data)})
	if err = db.Create(&assetdb.Record{Asset: "ground.mmg", Collection: "entries", Ordinal: 0, JSON: string(raw)}).Error; err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[uint16]assets.BattleBackdrop{10017: {Default: 80, Regions: []assets.BattleBackdropRegion{{Selector: 91, Left: 10, Top: 20, Right: 300, Bottom: 400}}}}
	if !reflect.DeepEqual(got.BattleBackdrops, want) {
		t.Fatal(got.BattleBackdrops)
	}
	// Runtime survives removal of provenance and preserves an explicit empty edit.
	if err = db.Where("asset = ?", "ground.mmg").Delete(&assetdb.Record{}).Error; err != nil {
		t.Fatal(err)
	}
	got.BattleBackdrops = map[uint16]assets.BattleBackdrop{}
	if err = SaveEditedCatalog(db, got); err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	next, err := LoadDatabase(path)
	if err != nil || len(next.BattleBackdrops) != 0 {
		t.Fatal(next, err)
	}
}

func TestBattleBackdropMigrationRollback(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = db.Migrator().DropTable(&BattleBackdropsRegionsRow{}, &BattleBackdropsRow{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 49).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&assetdb.Record{Asset: "ground.mmg", Collection: "entries", Ordinal: 0, JSON: `{"name":"10017.map","decoded_hex":"00"}`}).Error; err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err == nil {
		t.Fatal("accepted truncated map")
	}
	var schema catalogSchema
	if err = db.First(&schema, catalogMetadataID).Error; err != nil {
		t.Fatal(err)
	}
	if schema.Version != 49 || db.Migrator().HasTable(&BattleBackdropsRow{}) {
		t.Fatal("failed migration was not atomic")
	}
}

func TestBattleBackdropRevisionRetiresOnlyOriginalPolicies(t *testing.T) {
	defs := historicalBackdropDefaults()
	for i := range defs.Rules {
		if defs.Rules[i].ID == "north_island_xaolan_battle_background" {
			defs.Rules[i].Value = 82
		}
	}
	db := storyReadFixture(t, defs)
	for range 2 {
		if err := migrateStoryRevision(db, nativeQuestRevisionV50); err != nil {
			t.Fatal(err)
		}
	}
	got, err := readStory(db)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, r := range got.Rules {
		if r.Kind == assets.StoryBattleBackground {
			count++
			if r.ID != "north_island_xaolan_battle_background" || r.Value != 82 {
				t.Fatal(r)
			}
		}
	}
	if count != 1 {
		t.Fatal("operator override removed", count)
	}
}
