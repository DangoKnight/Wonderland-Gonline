package assetsql

import (
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestPetFoodDefinitionsNativeTargetsAndPreservingUpgrade(t *testing.T) {
	items := map[uint16]assets.NativeItem{}
	for _, v := range []struct{ id, target uint16 }{{27001, 14156}, {27008, 17914}, {27025, 31148}} {
		i := assets.NativeItem{}
		i.Definition.ID = v.id
		i.Definition.Type = 20
		i.Record[413], i.Record[414] = byte(v.target), byte(v.target>>8)
		items[v.id] = i
	}
	foods := defaultPetFoods(items)
	if foods[27001].Targets[0] != 14156 || foods[27008].Targets[0] != 17914 || len(foods[27025].Targets) != 2 || foods[27025].Targets[1] != 31235 {
		t.Fatal("incorrect food targets", foods)
	}
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = writePetFoods(db, foods); err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&catalogSchema{}).Where("id = ?", 1).Update("version", 18).Error; err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	c, err := LoadDatabase(path)
	if err != nil || len(c.PetFoods) != 3 || c.PetFoods[27008].Targets[0] != 17914 {
		t.Fatal("food definitions overwritten by migration", err)
	}
}
