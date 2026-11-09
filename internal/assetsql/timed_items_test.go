package assetsql

import (
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/game"
)

func TestTimedItemMigrationPreservesLaterEdits(t *testing.T) {
	db, err := assetdb.Open(filepath.Join(t.TempDir(), "assets.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = db.AutoMigrate(&catalogSchema{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&catalogSchema{ID: 1, Version: 67}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("CREATE TABLE catalog_native_items (native_items_key INTEGER PRIMARY KEY, value_definition_name TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO catalog_native_items VALUES (34173,'Kept name')").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	var row NativeItemsRow
	if err = db.First(&row, "native_items_key = ?", 34173).Error; err != nil {
		t.Fatal(err)
	}
	if row.ValueDefinitionTimedKind != 2 || row.ValueDefinitionTimedMinutes != 120 || row.ValueDefinitionEXPTenths != 100 || row.ValueDefinitionName != "Kept name" {
		t.Fatal(row)
	}
	if err = db.Model(&NativeItemsRow{}).Where("native_items_key = ?", 34173).Update("value_definition_timed_minutes", 17).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	row = NativeItemsRow{}
	if err = db.First(&row, "native_items_key = ?", 34173).Error; err != nil || row.ValueDefinitionTimedMinutes != 17 {
		t.Fatal("migration reset administrator duration", err, row)
	}
}

func TestTimedDurationsMatchDescriptionAudit(t *testing.T) {
	defs, err := defaultTimedItems()
	if err != nil {
		t.Fatal(err)
	}
	// Independent description audit: Copper/Fast Growth two hours, Silver
	// four, Gold eight; Training Voucher six and Training Fare one.
	for id, minutes := range map[uint16]uint32{34099: 120, 34100: 240, 34101: 480, 34124: 120, 34191: 480, 34181: 360, 34253: 360, 34258: 60} {
		if defs[id].TimedMinutes != minutes {
			t.Fatalf("item %d: %d minutes, description says %d", id, defs[id].TimedMinutes, minutes)
		}
	}
	potions := 0
	for id, d := range defs {
		if d.TimedKind == game.TimedItemPotion {
			potions++
			if d.TimedMinutes != 120 {
				t.Fatalf("potion %d does not grant its described two hours", id)
			}
		}
	}
	if potions != 24 {
		t.Fatal("description audit must cover all 24 potion variants", potions)
	}
}

func TestTimedDurationCorrectionPreservesCustomValues(t *testing.T) {
	for _, tt := range []struct {
		name          string
		kind          game.TimedItemKind
		minutes, want uint32
	}{
		{"previous default", game.TimedItemTraining, 60, 360},
		{"custom duration", game.TimedItemTraining, 90, 90},
		{"disabled", game.TimedItemTraining, 0, 0},
		{"custom kind", game.TimedItemPotion, 60, 60},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, err := assetdb.Open(filepath.Join(t.TempDir(), "assets.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer assetdb.Close(db)
			if err = db.AutoMigrate(&catalogSchema{}); err != nil {
				t.Fatal(err)
			}
			if err = db.Create(&catalogSchema{ID: 1, Version: 68}).Error; err != nil {
				t.Fatal(err)
			}
			if err = db.Exec("CREATE TABLE catalog_native_items (native_items_key INTEGER PRIMARY KEY, value_definition_name TEXT, value_description TEXT, value_definition_timed_kind INTEGER, value_definition_timed_minutes INTEGER, value_definition_exp_tenths INTEGER)").Error; err != nil {
				t.Fatal(err)
			}
			if err = db.Exec("INSERT INTO catalog_native_items VALUES (34181,'Kept voucher name','Training for 6 hours',?,?,0),(34258,'Training Fare','Training for 1 hour',3,60,0)", tt.kind, tt.minutes).Error; err != nil {
				t.Fatal(err)
			}
			if err = db.Transaction(migrateCatalog); err != nil {
				t.Fatal(err)
			}
			var row NativeItemsRow
			if err = db.First(&row, "native_items_key = ?", 34181).Error; err != nil {
				t.Fatal(err)
			}
			if row.ValueDefinitionTimedMinutes != tt.want || row.ValueDefinitionName != "Kept voucher name" || row.ValueDescription != "Training for 6 hours" {
				t.Fatal("migration altered custom duration or descriptive fields", row)
			}
			if err = db.Transaction(migrateCatalog); err != nil {
				t.Fatal(err)
			}
			row = NativeItemsRow{}
			if err = db.First(&row, "native_items_key = ?", 34258).Error; err != nil || row.ValueDefinitionTimedMinutes != 60 {
				t.Fatal("unrelated ticket changed", err, row)
			}
		})
	}
}

func TestTrainingTicketSixHourUpgradeFromSchema69(t *testing.T) {
	for _, minutes := range []uint32{60, 90, 0} {
		db, err := assetdb.Open(filepath.Join(t.TempDir(), "assets.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer assetdb.Close(db)
		if err = db.AutoMigrate(&catalogSchema{}); err != nil {
			t.Fatal(err)
		}
		if err = db.Create(&catalogSchema{ID: 1, Version: 69}).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.Exec("CREATE TABLE catalog_native_items (native_items_key INTEGER PRIMARY KEY, value_definition_timed_kind INTEGER, value_definition_timed_minutes INTEGER, value_definition_exp_tenths INTEGER)").Error; err != nil {
			t.Fatal(err)
		}
		if err = db.Exec("INSERT INTO catalog_native_items VALUES (34253,3,?,0),(34258,3,60,0)", minutes).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.Transaction(migrateCatalog); err != nil {
			t.Fatal(err)
		}
		want := minutes
		if minutes == 60 {
			want = 360
		}
		for _, id := range []uint16{34253, 34258} {
			var row NativeItemsRow
			if err = db.First(&row, "native_items_key = ?", id).Error; err != nil {
				t.Fatal(err)
			}
			expected := want
			if id == 34258 {
				expected = 60
			}
			if row.ValueDefinitionTimedMinutes != expected {
				t.Fatalf("item %d: %d, want %d", id, row.ValueDefinitionTimedMinutes, expected)
			}
		}
		if err = db.Transaction(migrateCatalog); err != nil {
			t.Fatal(err)
		}
	}
}
