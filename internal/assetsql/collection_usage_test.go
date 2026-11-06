package assetsql

import (
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func collectionUsageEvent(id uint16) assets.Event {
	return assets.Event{ClickID: id, Branches: []assets.Branch{
		{Index: 1, Condition: [21]byte{14, 12, 43, 1}, Operations: []assets.Operation{{Index: 1, Data: [21]byte{1, 2, 0, 141, 195, 1}}}},
		{Index: 2, Condition: [21]byte{14, 12, 43, 2}, Operations: []assets.Operation{
			{Index: 1, Data: [21]byte{1, 1, 0, 6, 0, 1, 0, 0, 1}},
			{Index: 2, Data: [21]byte{14, 12, 43, 1, 0, 0, 0, 0, 180}},
		}},
	}}
}

func TestCollectionUsageMigrationPreservesOverridesAndIncompleteEvents(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	c, err := readCatalog(db)
	if err != nil {
		t.Fatal(err)
	}
	c.EventGathering = assets.EventGatheringRules{Timers: []uint16{11020}, Pools: []assets.EventGatheringPool{{Type: 1, Variant: 1, Items: []uint16{10002}}}}
	c.Talks.ByID[50061] = "No more things to collect now."
	if c.DisabledEvents == nil {
		c.DisabledEvents = map[uint32]string{}
	}
	var events []assets.Event
	for id := uint16(1); id <= 5; id++ {
		event := collectionUsageEvent(id)
		if id == 3 {
			event.Branches[1].Operations[0].Data[5] = 99
		}
		events = append(events, event)
		reason := legacyCollectionDisableReason
		if id == 2 {
			reason = "operator disabled"
		}
		if id == 5 {
			reason += ";missing item"
		}
		if id != 4 {
			c.DisabledEvents[assets.EventKey(60000, id)] = reason
		}
	}
	c.Maps[60000] = assets.Map{ID: 60000, Events: events}
	if err := writeCatalog(db, c); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 28).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := MigrateDatabase(path); err != nil {
			t.Fatal(err)
		}
		got, err := readDisabledEvents(db)
		if err != nil {
			t.Fatal(err)
		}
		if _, disabled := got[assets.EventKey(60000, 1)]; disabled {
			t.Fatal("supported collection remains disabled")
		}
		if got[assets.EventKey(60000, 2)] != "operator disabled" || got[assets.EventKey(60000, 3)] != legacyCollectionDisableReason || got[assets.EventKey(60000, 5)] != legacyCollectionDisableReason+";missing item" {
			t.Fatal("custom or incomplete event enabled")
		}
		if _, disabled := got[assets.EventKey(60000, 4)]; disabled {
			t.Fatal("missing disable record recreated")
		}
	}
	// An intentional later disable must survive repeated initialization.
	if err := db.Create(&DisabledEventsRow{DisabledEventsKey: assets.EventKey(60000, 1), Value: legacyCollectionDisableReason}).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	got, _ := readDisabledEvents(db)
	if got[assets.EventKey(60000, 1)] != legacyCollectionDisableReason {
		t.Fatal("repeat initialization removed later disable")
	}
}
