package assetsql

import (
	"encoding/json"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestCollectionSpotAuthoredDefaults(t *testing.T) {
	var spots map[uint16]assets.CollectionSpot
	if err := json.Unmarshal(collectionSpotDefaults, &spots); err != nil {
		t.Fatal(err)
	}
	if len(spots) != 162 || spots[2].Location != "Fishing Village" || spots[2].X != 1822 || spots[2].Y != 1035 {
		t.Fatal("reference locations", len(spots), spots[2])
	}
	if s := spots[102]; s.Kind != assets.CollectionKindVacuum || s.MapID != 60001 || len(s.Rewards) != 6 {
		t.Fatal("clay vacuum pool", s)
	}
	if s := spots[65001]; s.Source != "owner" || s.X != 5300 || s.Y != 1900 || s.MapID != 60001 || s.Resolved() {
		t.Fatal("north supplemental spot", s)
	}
	if s := spots[65002]; s.X != 1362 || s.Y != 3575 || s.MapID != 0 || len(s.MapCandidates) != 2 || s.Resolved() {
		t.Fatal("south supplemental spot", s)
	}
	vacuums, rewards, positions := 0, 0, 0
	for _, s := range spots {
		if s.Kind == assets.CollectionKindVacuum {
			vacuums++
		}
		rewards += len(s.Rewards)
		if s.PositionKnown {
			positions++
		}
	}
	if vacuums != 30 || rewards != 251 || positions != 152 {
		t.Fatal(vacuums, rewards, positions)
	}
}

func TestCollectionSpotMigrationPreservesEditsAndEmptyDefinitions(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err := db.Model(&CollectionSpotsRow{}).Where("collection_spots_key = ?", 102).Update("value_location", "Local clay spot").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&catalogSchema{}).Where("id = ?", 1).Update("version", 24).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	c, err := LoadDatabase(path)
	if err != nil || c.CollectionSpots[102].Location != "Local clay spot" {
		t.Fatal("edit overwritten", err)
	}
	if err := db.Where("1 = 1").Delete(&CollectionSpotsRow{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&catalogSchema{}).Where("id = ?", 1).Update("version", 24).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	c, err = LoadDatabase(path)
	if err != nil || len(c.CollectionSpots) != 0 {
		t.Fatal("empty definitions reseeded", err)
	}
}
