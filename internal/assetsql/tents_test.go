package assetsql

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
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
	if err != nil || got.Economy.Marriage.Fee != 123 || got.Tents.SpawnX != 460 || len(got.Tents.Furniture) != 2 || got.Tents.Furniture[0].ItemID != 38055 {
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

func TestTentDefaultsMigrationPreserveAdministratorEdits(t *testing.T) {
	for _, edited := range []bool{false, true} {
		t.Run(map[bool]string{false: "original", true: "edited"}[edited], func(t *testing.T) {
			path := runtimeDatabaseFixture(t)
			if err := MigrateDatabase(path); err != nil {
				t.Fatal(err)
			}
			c, err := LoadDatabase(path)
			if err != nil {
				t.Fatal(err)
			}
			c.Tents = assets.TentRules{SpawnX: 460, SpawnY: 700, Floor: 39062, Wallpaper: 39064, Furniture: []assets.TentFurnitureDefault{{ItemID: 38027, X: 43, Y: 42}, {ItemID: 38049, X: 45, Y: 42}}}
			if edited {
				c.Tents.Furniture[0].X = 40
			}
			db, err := assetdb.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer assetdb.Close(db)
			if err = SaveEditedCatalog(db, c); err != nil {
				t.Fatal(err)
			}
			if err = db.Model(&catalogSchema{}).Where("id = ?", 1).Update("version", 13).Error; err != nil {
				t.Fatal(err)
			}
			if err = MigrateDatabase(path); err != nil {
				t.Fatal(err)
			}
			got, err := LoadDatabase(path)
			if err != nil {
				t.Fatal(err)
			}
			if edited {
				if got.Tents.Furniture[0].X != 40 || got.Tents.Furniture[0].ItemID != 38027 {
					t.Fatal("custom defaults overwritten")
				}
			} else if got.Tents.Furniture[0].ItemID != 38055 || got.Tents.Furniture[0].X != 34 || got.Tents.Furniture[1].X != 37 {
				t.Fatal("old defaults not upgraded", got.Tents)
			}
		})
	}
}
