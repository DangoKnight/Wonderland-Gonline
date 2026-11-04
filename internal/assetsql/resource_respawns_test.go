package assetsql

import (
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestResourceRespawnMigrationPreservesOverridesAndEmptyLists(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	rows := []assets.ResourceRespawn{{Kind: game.ResourceCollectionActor, MapID: 10017, NodeID: 5, CooldownSeconds: 45}, {Kind: game.ResourceGroundItem, MapID: 10017, NodeID: 5, CooldownSeconds: 90}}
	if err := writeResourceRespawns(db, rows); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := db.Model(&catalogSchema{}).Where("id=?", catalogMetadataID).Update("version", 29).Error; err != nil {
			t.Fatal(err)
		}
		if err := MigrateDatabase(path); err != nil {
			t.Fatal(err)
		}
		got, err := readResourceRespawns(db)
		if err != nil || len(got) != 2 || got[0] != rows[0] || got[1] != rows[1] {
			t.Fatal("migration overwrote per-source durations", got, err)
		}
	}
	if err := db.Where("1=1").Delete(&ResourceRespawnsRow{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&catalogSchema{}).Where("id=?", catalogMetadataID).Update("version", 29).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	if got, err := readResourceRespawns(db); err != nil || len(got) != 0 {
		t.Fatal("migration repopulated empty overrides", got, err)
	}
}
