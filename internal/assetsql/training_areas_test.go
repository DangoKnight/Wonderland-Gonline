package assetsql

import (
	"path/filepath"
	"testing"

	"wonderland-gonline/internal/assetdb"
)

func TestTrainingAreaMigrationPreservesMapAndLaterEdits(t *testing.T) {
	db, err := assetdb.Open(filepath.Join(t.TempDir(), "assets.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err := db.AutoMigrate(&catalogSchema{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&catalogSchema{ID: 1, Version: 66}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE catalog_maps (maps_key INTEGER PRIMARY KEY, value_scene INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO catalog_maps VALUES (13056,777),(12000,12000)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	var row MapsRow
	if err := db.First(&row, "maps_key = ?", 13056).Error; err != nil {
		t.Fatal(err)
	}
	if row.ValueScene != 777 || row.ValueBattleRoundRecoveryHP != 100 || row.ValueBattleRoundRecoverySP != 50 || row.ValueBattleVictoryEXP != 200 {
		t.Fatal("training policy or original map changed", row)
	}
	if err := db.Model(&MapsRow{}).Where("maps_key = ?", 13056).Update("value_battle_victory_exp", 17).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	row = MapsRow{}
	if err := db.First(&row, "maps_key = ?", 13056).Error; err != nil || row.ValueBattleVictoryEXP != 17 {
		t.Fatal("migration overwrote administrator edit", err, row)
	}
	row = MapsRow{}
	if err := db.First(&row, "maps_key = ?", 12000).Error; err != nil || row.ValueBattleVictoryEXP != 0 || row.ValueBattleRoundRecoveryHP != 0 {
		t.Fatal("non-training map changed", err, row)
	}
}
