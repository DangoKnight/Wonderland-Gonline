package assetsql

import (
	"gorm.io/gorm"
	"reflect"
	"strings"
	"testing"
	"wonderland-gonline/internal/assetdb"
)

func TestEconomyVersionOneUpgradePreservesDefinitions(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = db.Model(&NativeItemsRow{}).Where("native_items_key = ?", 10002).Update("value_definition_name", "Preserve upgrade").Error; err != nil {
		t.Fatal(err)
	}
	// Build v1 by removing only newly-added economy ownership tables.
	models := catalogTables()
	for i := len(models) - 1; i >= 0; i-- {
		named, ok := models[i].(interface{ TableName() string })
		if ok && (strings.HasPrefix(named.TableName(), "catalog_economy") || (strings.HasPrefix(named.TableName(), "catalog_tents") || strings.HasPrefix(named.TableName(), "catalog_arcades") || strings.HasPrefix(named.TableName(), "catalog_terrains"))) {
			if err = db.Migrator().DropTable(models[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = db.Migrator().DropColumn(&catalogPresence{}, "Terrains"); err != nil {
		t.Fatal(err)
	}
	if err = db.Migrator().DropColumn(&catalogPresence{}, "Arcades"); err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 1).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	c, err := LoadDatabase(path)
	if err != nil || c.Items[10002].Name != "Preserve upgrade" || len(c.Economy.Manufacturing) != 7 || len(c.Economy.Gathering) != 3 || c.Economy.Marriage.Fee != 60000 {
		t.Fatal("upgrade lost data or seed", err)
	}
	// Admin edits survive repeat migration; transport JSON doesn't persist as documents.
	c.Economy.Marriage.Fee = 123
	if err = db.Transaction(func(tx *gorm.DB) error { return SaveEditedCatalog(tx, c) }); err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	again, err := LoadDatabase(path)
	if err != nil || !reflect.DeepEqual(c.Economy, again.Economy) {
		t.Fatal("repeat migration overwrote edits", err)
	}
}
