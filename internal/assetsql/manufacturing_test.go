package assetsql

import (
	"encoding/json"
	"gorm.io/gorm"
	"testing"
	"wonderland-go/internal/assetdb"
	"wonderland-go/internal/game"
)

func TestManufacturingProjectionPreservesFormulaOrdinalsAndAllInputs(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	raw := `{"record_index":27,"fields":{"result_id":500,"result_count":2,"plan_id":200,"tool_id":300,"build_time":3,"material_1_id":100,"material_1_count":1,"material_2_id":101,"material_2_count":2,"material_3_id":102,"material_3_count":3,"material_4_id":103,"material_4_count":4,"material_5_id":104,"material_5_count":5}}`
	if err = db.Create(&assetdb.Record{Asset: "compound2.dat", Collection: "records", Ordinal: 27, JSON: raw}).Error; err != nil {
		t.Fatal(err)
	}
	items := map[uint16]game.ItemDefinition{}
	for _, id := range []uint16{100, 101, 102, 103, 104, 200, 300, 500} {
		items[id] = game.ItemDefinition{ID: id, Type: 23}
	}
	f, err := importedManufacturing(db, items)
	if err != nil || len(f) != 1 || f[27].DurationSeconds != 180 || f[27].Inputs[4].Count != 5 || f[27].PlanID != 200 || f[27].ToolID != 300 {
		t.Fatal(f, err)
	}
	if err = db.Transaction(func(tx *gorm.DB) error { return migrateCatalog(tx) }); err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(func(tx *gorm.DB) error { return writeManufacturing(tx, f) }); err != nil {
		t.Fatal(err)
	}
	got, err := readManufacturing(db)
	if err != nil || got[27].ID != 27 || got[27].Inputs != f[27].Inputs {
		t.Fatal(got, err)
	}
}
func TestAuthoredRecipeChanceFeeAndCustomization(t *testing.T) {
	rates, err := parseAuthoredSynthesis("100,101 | 500,Crafted,37.5%,123\n")
	if err != nil || len(rates) != 1 || rates[0].SuccessPercent != 37.5 || rates[0].Fee != 123 {
		t.Fatal(rates, err)
	}
	for _, invalid := range []string{"100,101 | 500,Crafted,NaN,0", "100,101 | 500,Crafted,101,0", "100,101 | 500,Crafted,50,-1"} {
		if _, err = parseAuthoredSynthesis(invalid); err == nil {
			t.Fatal(invalid)
		}
	}
	path := runtimeDatabaseFixture(t)
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	economy, err := readEconomy(db)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, rate := range economy.Synthesis.Rates {
		if rate.Input1 == 10006 && rate.Input2 == 10007 && rate.Output == 10002 {
			found = true
			if rate.SuccessPercent != 100 {
				t.Fatal(rate)
			}
		}
	}
	if !found {
		t.Fatal("authored chance not projected")
	}
	economy.Synthesis.Rates[0].SuccessPercent = 12.5
	economy.Synthesis.Rates[0].Fee = 777
	before, _ := json.Marshal(economy.Synthesis.Rates[0])
	if err = applyAuthoredSynthesis(db, &economy); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(economy.Synthesis.Rates[0])
	if string(before) != string(after) {
		t.Fatal("custom rate overwritten")
	}
}

func TestManufacturingProjectionRejectsNarrowingOverflow(t *testing.T) {
	for _, fields := range []map[string]uint32{
		{"result_id": 65536}, {"result_count": 256}, {"plan_id": 65536},
		{"tool_id": 65536}, {"material_5_id": 65536}, {"material_4_count": 256},
		{"build_time": 4294967295},
	} {
		if manufacturingFieldsFit(fields) {
			t.Fatalf("accepted overflowing fields: %v", fields)
		}
	}
}

func TestManufacturingV8UpgradePreservesTypedEdits(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = db.Model(&NativeItemsRow{}).Where("native_items_key = ?", 10002).Update("value_definition_name", "Preserved item edit").Error; err != nil {
		t.Fatal(err)
	}
	economy, err := readEconomy(db)
	if err != nil {
		t.Fatal(err)
	}
	economy.Synthesis.Rates[0].SuccessPercent = 12.5
	if err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("economy_key = ?", catalogMetadataID).Delete(&EconomyRow{}).Error; err != nil {
			return err
		}
		return writeEconomy(tx, economy)
	}); err != nil {
		t.Fatal(err)
	}
	// Recreate the actual v8 table/column layout, not just its version label.
	for _, model := range []any{&ManufacturingRow{}, &RebornClassesRow{}} {
		if err = db.Migrator().DropTable(model); err != nil {
			t.Fatal(err)
		}
	}
	for _, col := range []struct {
		model any
		name  string
	}{
		{&catalogPresence{}, "Manufacturing"}, {&catalogPresence{}, "RebornClasses"},
		{&EconomyManufacturingRow{}, "ValueSuccessPercentPresent"}, {&EconomyManufacturingRow{}, "ValueSuccessPercent"},
		{&EconomyManufacturingRow{}, "ValueFee"}, {&EconomySynthesisRatesRow{}, "ValueFee"},
	} {
		if err = db.Migrator().DropColumn(col.model, col.name); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 8).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDatabase(path)
	if err != nil || got.Items[10002].Name != "Preserved item edit" || got.Economy.Synthesis.Rates[0].SuccessPercent != 12.5 {
		t.Fatal("v8 edits lost", err)
	}
	if !db.Migrator().HasTable(&ManufacturingRow{}) || len(got.RebornClasses) != 6 {
		t.Fatal("missing new typed datasets")
	}
	got.RebornClasses[0].Name = "Custom class"
	if err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&RebornClassesRow{}).Error; err != nil {
			return err
		}
		return writeRebornClasses(tx, got.RebornClasses)
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	classes, err := readRebornClasses(db)
	if err != nil || classes[0].Name != "Custom class" {
		t.Fatal("repeat migration overwrote class edit", err)
	}
}
