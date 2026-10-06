package assetsql

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wonderland-gonline/internal/assetdb"
)

func TestTentVersionTwoUpgradePreservesEconomyAndDefinitions(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	c, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Economy.Marriage.Fee = 123
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = SaveEditedCatalog(db, c); err != nil {
		t.Fatal(err)
	}
	models := catalogTables()
	for i := len(models) - 1; i >= 0; i-- {
		named, ok := models[i].(interface{ TableName() string })
		if ok && (strings.HasPrefix(named.TableName(), "catalog_tents") || strings.HasPrefix(named.TableName(), "catalog_arcades") || strings.HasPrefix(named.TableName(), "catalog_terrains")) {
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
	if err = db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 2).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDatabase(path)
	if err != nil || got.Economy.Marriage.Fee != 123 || got.Tents.SpawnX != 460 || len(got.Tents.Furniture) != 2 || got.Tents.Furniture[0].ItemID != 38027 {
		t.Fatal("v2 upgrade failed", err)
	}
	got.Tents.SpawnX = 600
	if err = SaveEditedCatalog(db, got); err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	got, err = LoadDatabase(path)
	if err != nil || got.Tents.SpawnX != 600 {
		t.Fatal("initialization overwrote edit", err)
	}
}

func TestInstalledSQLTentDefaultsReferenceNativeItems(t *testing.T) {
	path := os.Getenv("WONDERLAND_TEST_ASSETS_DB")
	if path == "" {
		t.Skip("set WONDERLAND_TEST_ASSETS_DB")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join("..", "..", path)
	}
	c, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Tents.SpawnX != 460 || c.Tents.SpawnY != 700 || c.Items[36002].Type != 27 {
		t.Fatal("tent projection changed native defaults")
	}
	for _, row := range c.Tents.Furniture {
		def, ok := c.Items[row.ItemID]
		if !ok || (def.Type != 28 && def.Type != 29 && def.Type != 30) {
			t.Fatal("unavailable native furniture", row)
		}
	}
}
