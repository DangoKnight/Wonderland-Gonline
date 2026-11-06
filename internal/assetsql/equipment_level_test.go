package assetsql

import (
	"testing"
	"wonderland-gonline/internal/assetdb"
)

func TestEquipmentLevelMigrationPreservesDefinitions(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	var row NativeItemsRow
	if err = db.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	row.ValueRecord[113] = 10 // Independent native record fixture.
	row.ValueDefinitionName = "Local name"
	if err = db.Save(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Migrator().DropColumn(&NativeItemsRow{}, "value_definition_equip_level"); err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&catalogSchema{}).Where("id = 1").Update("version", 30).Error; err != nil {
		t.Fatal(err)
	}

	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	fresh, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(fresh)
	var after NativeItemsRow
	if err = fresh.First(&after, row.NativeItemsKey).Error; err != nil {
		t.Fatal(err)
	}
	if after.ValueDefinitionEquipLevel != 10 || after.ValueDefinitionName != "Local name" {
		t.Fatal(after.ValueDefinitionEquipLevel, after.ValueDefinitionName, after.ValueRecord[113])
	}
}
