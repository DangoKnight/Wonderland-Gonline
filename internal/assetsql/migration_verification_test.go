package assetsql

import (
	"reflect"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestMigrationAlchemySourceOrderSurvivesSQLRestart(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Both native archives have competing outputs; source loads Compound2 first.
	for _, asset := range []string{"compound2.dat", "compound.dat"} {
		output := 10002
		if asset == "compound.dat" {
			output = 10003
		}
		fields := `{"fields":{"result_id":10002,"material_1_id":10004,"material_2_id":10005}}`
		if output == 10003 {
			fields = `{"fields":{"result_id":10003,"material_1_id":10004,"material_2_id":10005}}`
		}
		if err = db.Model(&assetdb.Record{}).Where("asset = ?", asset).Update("json", fields).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Model(&assetdb.Document{}).Where("asset = ?", "alchemy_recipes.txt").Update("json", `{"schema_version":1,"text":"10004,10005 | 10005,Authored first,100%\n10005,10004 | 10001,Authored second,100%"}`).Error; err != nil {
		t.Fatal(err)
	}
	if err = assetdb.Close(db); err != nil {
		t.Fatal(err)
	}
	legacy, err := loadLegacyFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []assets.AlchemyRecipe{{Input1: 10004, Input2: 10005, Output: 10005}, {Input1: 10005, Input2: 10004, Output: 10001}, {Input1: 10004, Input2: 10005, Output: 10002}, {Input1: 10004, Input2: 10005, Output: 10003}}
	if !reflect.DeepEqual(legacy.AlchemyRecipes, want) {
		t.Fatal("source order changed during import", legacy.AlchemyRecipes)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	// Remove snapshots: runtime order must come solely from typed SQL ordinals.
	db, err = assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Where("1 = 1").Delete(&assetdb.Document{}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Where("1 = 1").Delete(&assetdb.Record{}).Error; err != nil {
		t.Fatal(err)
	}
	if err = assetdb.Close(db); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		catalog, err := LoadDatabase(path)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(catalog.AlchemyRecipes, want) {
			t.Fatal("SQL restart reordered recipes", catalog.AlchemyRecipes)
		}
		for _, inputs := range [][2]uint16{{10004, 10005}, {10005, 10004}} {
			if output, ok := catalog.AlchemyResult(inputs[0], inputs[1]); !ok || output != 10005 {
				t.Fatal("first symmetric recipe lost", output)
			}
		}
	}
}
