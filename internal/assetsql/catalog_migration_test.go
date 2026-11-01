package assetsql

import (
	"reflect"
	"strings"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestCatalogDefinitionMigrationsPreserveEdits(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	models := catalogTables()
	for i := len(models) - 1; i >= 0; i-- {
		name := models[i].(interface{ TableName() string }).TableName()
		if strings.HasPrefix(name, "catalog_map_transitions") || strings.HasPrefix(name, "catalog_tent_surfaces") || strings.HasPrefix(name, "catalog_n_p_c_shop_defaults") || strings.HasPrefix(name, "catalog_travel") || strings.HasPrefix(name, "catalog_chest_prop_categories") || strings.HasPrefix(name, "catalog_tent_tool_hosts") || strings.HasPrefix(name, "catalog_boxed_items") || strings.HasPrefix(name, "catalog_skill_evolutions") || strings.HasPrefix(name, "catalog_event_gathering") || strings.HasPrefix(name, "catalog_n_p_c_buy_prices") {
			if err := db.Migrator().DropTable(models[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, name := range []string{"MapTransitions", "TentToolHosts", "ChestPropCategories", "NPCShopDefaults", "TentSurfaces", "BoxedItems", "SkillEvolutions", "NPCBuyPrices"} {
		if err := db.Migrator().DropColumn(&catalogPresence{}, name); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&catalogSchema{}).Where("id = ?", 1).Update("version", 13).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&NativeItemsRow{}).Where("native_items_key = ?", 10002).Update("value_definition_name", "Local edit").Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	c, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Items[10002].Name != "Local edit" || c.BoxedItems[26001].ItemID != 11101 || c.BoxedItems[26001].Quantity != 1 || len(c.SkillEvolutions) < 100 || len(c.EventGathering.Pools) < 10 || c.NPCBuyPrices[10002] != 10 {
		t.Fatal("missing content or overwritten edits")
	}
	if err := db.Model(&BoxedItemsRow{}).Where("boxed_items_key = ?", 26001).Update("value_quantity", 2).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&NPCBuyPricesRow{}).Where("n_p_c_buy_prices_key = ?", 10002).Update("value", 99).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("1 = 1").Delete(&SkillEvolutionsRow{}).Error; err != nil {
		t.Fatal(err)
	}
	// Reenter the v14 upgrade with existing feature tables, including an
	// intentionally empty definition. Each migration must preserve these edits.
	if err := db.Model(&catalogSchema{}).Where("id = ?", 1).Update("version", 13).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	boxes, err := readBoxedItems(db)
	if err != nil || boxes[26001].Quantity != 2 {
		t.Fatal("reinitialization overwrote box", err)
	}
	prices, err := readNPCBuyPrices(db)
	if err != nil || prices[10002] != 99 {
		t.Fatal("reinitialization overwrote NPC price", err)
	}
	evolutions, err := readSkillEvolutions(db)
	if err != nil || len(evolutions) != 0 {
		t.Fatal("reinitialization reseeded deleted skill evolutions", err)
	}
}

func TestCatalogDefinitionDefaultsStructuredRoundTrip(t *testing.T) {
	c := &assets.Catalog{}
	c.MapTransitions = catalogDefaultForTest(t, defaultMapTransitions)
	c.TentToolHosts = catalogDefaultForTest(t, defaultTentToolHosts)
	c.ChestPropCategories = catalogDefaultForTest(t, defaultChestPropCategories)
	c.NPCShopDefaults = catalogDefaultForTest(t, defaultNPCShopDefaults)
	c.NPCBuyPrices = catalogDefaultForTest(t, defaultNPCBuyPrices)
	c.Travel = catalogDefaultForTest(t, defaultTravel)
	c.TentSurfaces = catalogDefaultForTest(t, defaultTentSurfaces)
	c.BoxedItems = catalogDefaultForTest(t, defaultBoxedItems)
	c.SkillEvolutions = catalogDefaultForTest(t, defaultSkillEvolutions)
	c.EventGathering = catalogDefaultForTest(t, defaultEventGathering)

	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	loaded, err := LoadTransaction(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"BoxedItems", "SkillEvolutions", "EventGathering", "NPCBuyPrices", "NPCShopDefaults", "Travel", "MapTransitions", "TentSurfaces", "TentToolHosts", "ChestPropCategories"} {
		want := reflect.ValueOf(c).Elem().FieldByName(name).Interface()
		got := reflect.ValueOf(loaded).Elem().FieldByName(name).Interface()
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("structured content round trip: %s", name)
		}
	}
}

func catalogDefaultForTest[T any](t *testing.T, read func() (T, error)) T {
	t.Helper()
	value, err := read()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
