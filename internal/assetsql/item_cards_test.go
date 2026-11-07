package assetsql

import (
	"path/filepath"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestItemCardsDefaultsAndPreservingMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cards.db")
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err := db.AutoMigrate(&catalogPresence{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&catalogPresence{ID: catalogMetadataID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateItemCards); err != nil {
		t.Fatal(err)
	}
	cards, err := readItemCards(db)
	if err != nil || len(cards) != 236 {
		t.Fatal(len(cards), err)
	}
	if cards[50534].MaleItem != 35451 || cards[50534].FemaleItem != 35450 || !reflect.DeepEqual(cards[51609].Rewards, []assets.CardReward{{ItemID: 30908, Quantity: 1}, {ItemID: 34707, Quantity: 1}}) {
		t.Fatal("authored mappings")
	}
	if err := db.Where("item_cards_key = ?", 50534).Delete(&ItemCardsRewardsRow{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("item_cards_key = ?", 50534).Delete(&ItemCardsRow{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateItemCards); err != nil {
		t.Fatal(err)
	}
	again, err := readItemCards(db)
	if err != nil || len(again) != 235 {
		t.Fatal("migration overwrote administrator deletion", err)
	}
}

func TestItemCardsRepositoryDefinitions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets.db")
	if _, err := InitializeRepositoryDatabase(path); err != nil {
		t.Fatal(err)
	}
	c, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := assets.ParseItemCards(itemCardsDefaults, c.Items)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed, c.ItemCards) {
		t.Fatal("repository card defaults differ from SQL")
	}
	for id, want := range map[uint16][]assets.CardReward{
		50025: {{ItemID: 34123, Quantity: 1}, {ItemID: 61080, Quantity: 1}},                                                             // Pure Chaos Crystal, not the ordinary crystal.
		51528: {{ItemID: 21726, Quantity: 1}, {ItemID: 21727, Quantity: 1}},                                                             // Strawberry/tangerine dresses, not fruit.
		51215: {{ItemID: 34089, Quantity: 1}, {ItemID: 34139, Quantity: 1}, {ItemID: 34239, Quantity: 1}, {ItemID: 61062, Quantity: 1}}, // Jade Emperor is the quest name.
	} {
		if !reflect.DeepEqual(parsed[id].Rewards, want) {
			t.Fatal("partial-name match", id, parsed[id])
		}
	}
}
