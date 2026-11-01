package assetsql

import (
	"reflect"
	"testing"

	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
)

func TestNPCRoutesMigrationPreservesEdits(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	// Independent native NPC section: click 7, route slot 3, two points.
	route := make([]byte, 92)
	copy(route, []byte{3, 1, 0, 2})
	copy(route[12:], []byte{100, 0, 0, 0, 200, 0, 0, 0, 44, 1, 0, 0, 144, 1, 0, 0})
	section := protocol.Builder{}.U16(1).U16(7).Bytes(make([]byte, 20)).U8(1).U32(50).U32(60).U8(0).U8(0).U8(4).
		U32(17001).U8(1).U8(2).U8(0).U8(0).U8(5).Bytes(make([]byte, 12)).U8(1).Bytes(route).Bytes(make([]byte, 8))
	const mapID = 65000
	if err := db.Create(&MapsRow{MapsKey: mapID, ValueID: mapID, ValueSections0: section, ValueSections0Present: true, ValueNPCsPresent: true}).Error; err != nil {
		t.Fatal(err)
	}
	edited := []assets.MapNPC{{ClickID: 99, Name: "Added"}, {ClickID: 7, Name: "Edited", X: 900, WalkSteps: []assets.WalkStep{{X: 123, Y: 456, Delay: 789}}}}
	if err := writeMapsNPCs(db, edited, mapID); err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{&MapsNPCsRoutesValuesRow{}, &MapsNPCsRoutesRow{}} {
		if err := db.Migrator().DropTable(model); err != nil {
			t.Fatal(err)
		}
	}
	for _, column := range []string{"value_routes_present", "value_sign", "value_speed"} {
		if err := db.Exec("ALTER TABLE catalog_maps_n_p_cs DROP COLUMN " + column).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&catalogSchema{}).Where("id = ?", 1).Update("version", 13).Error; err != nil {
		t.Fatal(err)
	}
	// Upgrades are offline: reopen the reader after schema changes.
	if err := assetdb.Close(db); err != nil {
		t.Fatal(err)
	}
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err = assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	got, err := readMapsNPCs(db, mapID)
	if err != nil {
		t.Fatal(err)
	}
	edited[1].Routes = map[byte][]assets.RoutePoint{3: {{X: 100, Y: 200}, {X: 300, Y: 400}}}
	// Generated empty slices intentionally remain non-nil; compare only owned
	// fields here so the fixture also checks a reordered and an added NPC.
	if len(got) != 2 || got[0].Name != "Added" || len(got[0].Routes) != 0 || got[1].Name != "Edited" || got[1].X != 900 || got[1].Sign != 4 || got[1].Speed != 5 || !reflect.DeepEqual(got[1].WalkSteps, edited[1].WalkSteps) || !reflect.DeepEqual(got[1].Routes, edited[1].Routes) {
		t.Fatalf("route projection lost edits: %+v", got)
	}
	if err := db.Model(&MapsNPCsRoutesValuesRow{}).Where("maps_key = ?", mapID).Update("value_x", 777).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	got, err = readMapsNPCs(db, mapID)
	if err != nil || got[1].Routes[3][0].X != 777 {
		t.Fatal("repeat migration overwrote route edit", err)
	}
}
