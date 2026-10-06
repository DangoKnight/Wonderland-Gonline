package assetsql

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"

	"gorm.io/gorm"
	"wonderland-gonline/internal/assetdb"
)

func runtimeDatabaseFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "assets.db")
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err := db.AutoMigrate(&assetdb.ImportInfo{}, &assetdb.Document{}, &assetdb.Record{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&assetdb.ImportInfo{ID: 1, SchemaVersion: assetdb.ManifestSchemaVersion}).Error; err != nil {
		t.Fatal(err)
	}
	add := func(asset string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&assetdb.Document{Asset: asset, JSON: string(raw)}).Error; err != nil {
			t.Fatal(err)
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			t.Fatal(err)
		}
		for _, collection := range []string{"items", "records", "maps", "value", "rows"} {
			var rows []json.RawMessage
			if json.Unmarshal(object[collection], &rows) != nil {
				continue
			}
			for ordinal, row := range rows {
				if err := db.Create(&assetdb.Record{Asset: asset, Collection: collection, Ordinal: ordinal, JSON: string(row)}).Error; err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	add("item.dat", itemJSONFixture(t))
	native := func(asset, fields string) {
		add(asset, map[string]any{"schema_version": 1, "format": "native-records", "records": []any{map[string]any{"fields": json.RawMessage(fields), "name": map[string]any{"text": "SQL name"}, "text": map[string]any{"text": "SQL dialogue", "length": 12}, "record_index": 1, "file_offset": 292}}})
	}
	native("npc.dat", `{"id":42,"strength":11,"constitution":12,"intelligence":13,"wisdom":14,"agility":15,"skill_1":23,"drop_1":10002}`)
	native("skill.dat", `{"id":23,"sp":5,"power_per_level":1.25,"effect_refs":[]}`)
	native("talk.dat", `{"id":7}`)
	native("mark.dat", `{"id":9,"completion_flag":17}`)
	native("compound.dat", `{"result_id":10002,"material_1_id":10001,"material_2_id":10003}`)
	native("compound2.dat", `{"result_id":10002,"material_1_id":10004,"material_2_id":10005}`)
	header := make([]byte, 12)
	binary.LittleEndian.PutUint32(header[8:], 1)
	index := make([]byte, 10)
	binary.LittleEndian.PutUint16(index, 10017)
	binary.LittleEndian.PutUint16(index[2:], 10017)
	binary.LittleEndian.PutUint32(index[4:], 22)
	binary.LittleEndian.PutUint16(index[8:], 44)
	add("eve.emg", map[string]any{"schema_version": 1, "format": "plaintext-eve", "source_bytes": 66, "header_hex": hex.EncodeToString(header), "maps": []any{map[string]any{"file_offset": 22, "index_hex": hex.EncodeToString(index), "decoded_hex": hex.EncodeToString(make([]byte, 44))}}})
	animation := make([]byte, 27)
	animation[3] = 2 // Unsupported movement retains fallback timing.
	index = make([]byte, 10)
	binary.LittleEndian.PutUint16(index, 23)
	binary.LittleEndian.PutUint32(index[6:], 27)
	add("skilldata.mbtm", map[string]any{"schema_version": 1, "format": "plaintext-animation-archive", "footer_count_hex": "0100", "records": []any{map[string]any{"decoded_hex": hex.EncodeToString(animation), "index_hex": hex.EncodeToString(index)}}})
	for asset, value := range map[string]string{
		"starter_items.json": `[{"OrderIdx":1,"ItemID":10002,"Count":1}]`,
		"pet_vouchers.json":  `[{"item_id":10002,"pet_id":42}]`,
		"critical_hits.json": `{"damage_multiplier":2,"items":[{"item_id":10002,"chance_percent":5}]}`,
		"item_mall.json":     `[{"item_id":10002,"count":1,"point_cost":25}]`,
		"mall_forging.json":  `{"families":[[10002,10003]],"point_items":[]}`,
		"gacha_packs.json":   `[]`,
		"lucky_draw.json":    `[]`,
	} {
		add(asset, map[string]any{"schema_version": 1, "value": json.RawMessage(value)})
	}
	add("disabled_quest_events.csv", map[string]any{"schema_version": 1, "text": "invalid provenance", "rows": [][]string{{"map", "event", "reason"}, {"10017", "2", "SQL reason"}}})
	add("npc_sale_prices.csv", map[string]any{"schema_version": 1, "text": "invalid provenance", "rows": [][]string{{"item_id", "price", "flags"}, {"10002", "31", "0"}}})
	add("monster_drops.txt", map[string]any{"schema_version": 1, "text": "TID:42 | 10002,SQL drop,1,1,50"})
	add("alchemy_recipes.txt", map[string]any{"schema_version": 1, "text": "10006,10007 | 10002,SQL recipe,100%"})
	return path
}

func TestRuntimeCatalogUsesSQLWithoutSourceFiles(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	c, err := loadLegacyFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.AssetsDatabase != path || c.ItemCatalog != "" {
		t.Fatal("runtime selected file source", c.Summary())
	}
	npc := c.NPCs[42]
	if npc.Stats != [5]uint16{11, 12, 13, 14, 15} || npc.Skills[0] != 23 || npc.Drops[0] != 10002 || npc.Name != "SQL name" {
		t.Fatal("assets.NPC fields lost", npc)
	}
	if c.Skills[23].PowerPerLevel != 1.25 || c.Talks.ByID[7] != "SQL dialogue" || c.Marks[9] != 17 || c.Maps[10017].Scene != 10017 {
		t.Fatal("native SQL fields lost", c.Summary())
	}
	if c.SalePrices[10002].Price != 31 || c.DisabledEvents[assets.EventKey(10017, 2)] != "SQL reason" || len(c.Drops[42]) != 1 || c.Drops[42][0].Name != "SQL drop" {
		t.Fatal("SQL settings lost")
	}
	want := []assets.AlchemyRecipe{{Input1: 10006, Input2: 10007, Output: 10002}, {Input1: 10004, Input2: 10005, Output: 10002}, {Input1: 10001, Input2: 10003, Output: 10002}}
	if !reflect.DeepEqual(c.AlchemyRecipes, want) {
		t.Fatal("authored recipe order lost", c.AlchemyRecipes)
	}
	writer, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(writer)
	var record assetdb.Record
	if err := writer.Where("asset = ? AND collection = ?", "item.dat", "items").Take(&record).Error; err != nil {
		t.Fatal(err)
	}
	var item assets.NativeItemExportRecord
	if err := json.Unmarshal([]byte(record.JSON), &item); err != nil {
		t.Fatal(err)
	}
	item.Definition.Name = "Edited in SQL"
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Model(&assetdb.Record{}).Where("asset = ? AND collection = ?", "item.dat", "items").Update("json", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	updated, err := loadLegacyFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Items[10002].Name != "Edited in SQL" || c.Items[10002].Name == "Edited in SQL" {
		t.Fatal("SQL reload or immutable snapshot failed")
	}
	if err := writer.Where("asset = ?", "npc.dat").Delete(&assetdb.Document{}).Error; err != nil {
		t.Fatal(err)
	}
	if got, err := loadLegacyFixture(path); err == nil || got != nil {
		t.Fatal("missing required SQL asset accepted")
	}
}

func TestRuntimeCatalogRejectsMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if c, err := LoadDatabase(path); err == nil || c != nil {
		t.Fatal("missing database accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("missing database created", err)
	}
}

// Numeric projections and plaintext archives must match independent native
// decoders. Names use the export's readable decoded text instead of ASCII-only
// legacy projections.
func TestInstalledSQLCatalogMatchesClientData(t *testing.T) {
	path, source := os.Getenv("WONDERLAND_TEST_ASSETS_DB"), os.Getenv("WONDERLAND_TEST_CLIENT_DATA")
	if path == "" || source == "" {
		t.Skip("set WONDERLAND_TEST_ASSETS_DB and WONDERLAND_TEST_CLIENT_DATA")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join("..", "..", path)
	}
	if !filepath.IsAbs(source) {
		source = filepath.Join("..", "..", source)
	}
	catalog, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) []byte {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	npcs, err := assets.ParseNPCs(read("Npc.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if len(npcs) != len(catalog.NPCs) {
		t.Fatal("assets.NPC count differs", len(npcs), len(catalog.NPCs))
	}
	for id, want := range npcs {
		got := catalog.NPCs[id]
		got.Name = ""
		want.Name = ""
		if got != want {
			t.Fatalf("assets.NPC %d: SQL %+v native %+v", id, got, want)
		}
	}
	skills, err := assets.ParseSkills(read("Skill.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != len(catalog.Skills) {
		t.Fatal("skill count differs", len(skills), len(catalog.Skills))
	}
	for id, want := range skills {
		got := catalog.Skills[id]
		got.Name = ""
		want.Name = ""
		got.EffectRefs = nil // Export annotation has no native byte equivalent.
		if len(got.Effects) == 0 {
			got.Effects = nil
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("skill %d: SQL %+v native %+v", id, got, want)
		}
	}
	marks, err := assets.ParseMarks(read("Mark.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(marks, catalog.Marks) {
		t.Fatal("mark projection differs")
	}
	maps, err := assets.ParseEVE(read("eve.Emg"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(maps, catalog.Maps) {
		t.Fatal("SQL EVE maps differ from native archive")
	}
	timings, err := assets.ParseAnimationTiming(read("SkillData.MBTM"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(timings, catalog.AnimationTiming) {
		t.Fatal("SQL animation timings differ from native archive")
	}
}

func TestRuntimeCatalogRejectsInvalidDatabaseContent(t *testing.T) {
	for _, name := range []string{"schema", "empty native rows", "invalid row JSON", "invalid archive"} {
		t.Run(name, func(t *testing.T) {
			path := runtimeDatabaseFixture(t)
			db, err := assetdb.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer assetdb.Close(db)
			switch name {
			case "schema":
				err = db.Model(&assetdb.ImportInfo{}).Where("id = ?", assetdb.ImportMetadataID).Update("schema_version", assetdb.ManifestSchemaVersion+1).Error
			case "empty native rows":
				err = db.Where("asset = ?", "npc.dat").Delete(&assetdb.Record{}).Error
			case "invalid row JSON":
				err = db.Model(&assetdb.Record{}).Where("asset = ?", "skill.dat").Update("json", "invalid JSON").Error
			case "invalid archive":
				err = db.Model(&assetdb.Record{}).Where("asset = ?", "eve.emg").Update("json", `{"file_offset":99999,"decoded_hex":"00","index_hex":"00"}`).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			if c, err := loadLegacyFixture(path); err == nil || c != nil {
				t.Fatal("invalid SQL content accepted")
			}
		})
	}
}

// itemJSONFixture exports the golden Item.Dat record of the assets package.
func itemJSONFixture(t *testing.T) assets.NativeItemExport {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "assets", "testdata", "native_item_record.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Encrypted string `json:"encrypted_hex"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	encrypted, err := hex.DecodeString(fixture.Encrypted)
	if err != nil {
		t.Fatal(err)
	}
	// The same 451-byte file header as the assets package's golden helper.
	header := make([]byte, 451)
	header[114] = 9
	export, err := assets.ExportNativeItems(append(header, encrypted...), "unavailable-source/Item.dat")
	if err != nil {
		t.Fatal(err)
	}
	return export
}

func TestSQLSkillEffectDefinitions(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		bad          bool
	}{
		{"native class", `{"id":50000,"effect_layer":15,"unknown_u8_offset_51":3,"unknown_u8_offset_52":172}`, true},
		{"authored", `{"id":50000,"effects":[{"target":"self","rounds":5,"modifiers":[{"stat":"spd","flat":42}]}]}`, false},
		{"protection", `{"id":50000,"area_attack":true,"effects":[{"target":"ally","rounds":2,"miss_physical_attacks":true,"miss_area_attacks":true}]}`, false},
		{"invalid", `{"id":50000,"effects":[{"target":"self","rounds":5,"modifiers":[{"stat":"unknown"}]}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := runtimeDatabaseFixture(t)
			db, err := assetdb.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			row := `{"name":{"text":"unrelated name"},"fields":` + tc.fields + `}`
			err = db.Model(&assetdb.Record{}).Where("asset = ? AND collection = ?", "skill.dat", "records").Update("json", row).Error
			assetdb.Close(db)
			if err != nil {
				t.Fatal(err)
			}
			c, err := loadLegacyFixture(path)
			if tc.bad {
				if err == nil || c != nil {
					t.Fatal("invalid effect catalog published")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			effects := c.Skills[50000].Effects
			if len(effects) != 1 {
				t.Fatal("SQL effects not projected", effects)
			}
			if tc.name == "authored" && (effects[0].Rounds != 5 || effects[0].Modifiers[0].Flat != 42) {
				t.Fatal("authored effect projection", effects)
			}
			if tc.name == "protection" && (!effects[0].MissPhysicalAttacks || !effects[0].MissAreaAttacks || !c.Skills[50000].IsAreaAttack(1)) {
				t.Fatal("authored protection or area override lost", c.Skills[50000])
			}
		})
	}
}

func TestInstalledSQLSkillEffectsMatchNativeClient(t *testing.T) {
	path := os.Getenv("WONDERLAND_TEST_ASSETS_DB")
	if path == "" {
		t.Skip("set WONDERLAND_TEST_ASSETS_DB")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join("..", "..", path)
	}
	source := filepath.Join("..", "..", "..", "Wonderland-Client", "data", "Skill.dat")
	raw, err := os.ReadFile(source)
	if os.IsNotExist(err) {
		t.Skip("translated native client unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	native, err := assets.ParseSkills(raw)
	if err != nil {
		t.Fatal(err)
	}
	sql, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range native {
		got, ok := sql.Skills[id]
		if len(got.Effects) == 0 {
			got.Effects = nil
		}
		if !ok || got.NativeEffectCode52 != want.NativeEffectCode52 || got.NativeRounds51 != want.NativeRounds51 || !reflect.DeepEqual(got.Effects, want.Effects) {
			t.Fatalf("SQL/native effect difference for %d: %+v / %+v", id, got, want)
		}
	}
}

func TestSQLNamedEffectRowsAreAuthoritative(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	doc := `{"schema_version":1,"format":"skill-effect-definitions","records":[{"id":"guard","name":"Guard","effects":[{"target":"ally","rounds":4,"modifiers":[{"stat":"def","percent":10}]}]}]}`
	if err := db.Create(&assetdb.Document{Asset: "skill_effects.json", JSON: doc}).Error; err != nil {
		t.Fatal(err)
	}
	row := assetdb.Record{Asset: "skill_effects.json", Collection: "records", Ordinal: 0, JSON: `{"id":"guard","name":"Guard","effects":[{"target":"ally","rounds":4,"modifiers":[{"stat":"def","percent":25}]}]}`}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	var skill assetdb.Record
	if err := db.Where("asset = ?", "skill.dat").Take(&skill).Error; err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	var value map[string]json.RawMessage
	json.Unmarshal([]byte(skill.JSON), &value)
	json.Unmarshal(value["fields"], &fields)
	fields["effect_refs"] = json.RawMessage(`["guard"]`)
	value["fields"], _ = json.Marshal(fields)
	raw, _ := json.Marshal(value)
	if err := db.Model(&skill).Update("json", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	catalog, err := loadLegacyFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Skills[23].Effects[0].Modifiers[0].Percent != 25 {
		t.Fatal("retained document masked SQL effect edit")
	}
	if err := db.Delete(&row).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := loadLegacyFixture(path); err == nil {
		t.Fatal("dangling SQL reference silently fell back to native")
	}
}

func TestSQLGachaUsesIndexedPoolsAndDisablesMissingItems(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	writer, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(writer)
	rows := []string{
		`{"item_id":10002,"rewards":[{"item_id":10002,"quantity":2,"weight":10000}]}`,
		`{"item_id":1234,"rewards":[{"item_id":10002,"quantity":1,"weight":10000}]}`,
		`{"item_id":34171,"rewards":[{"item_id":9999,"quantity":1,"weight":10000}]}`,
	}
	for i, row := range rows {
		if err := writer.Create(&assetdb.Record{Asset: "gacha_packs.json", Collection: "value", Ordinal: i, JSON: row}).Error; err != nil {
			t.Fatal(err)
		}
	}
	c, err := loadLegacyFixture(path)
	if err != nil || len(c.GachaPacks) != 1 || len(c.UnavailableGachaPacks) != 2 {
		t.Fatal(c, err)
	}
	if c.GachaPacks[10002].Rewards[0].Quantity != 2 || len(c.Warnings) < 2 || c.Warnings[0] != "gacha_packs.json: gacha pack 1234 disabled: missing item definitions [1234]" || !c.IsGachaPack(1234) || c.GachaAvailable(1234) {
		t.Fatal(c.Summary())
	}
	// The retained document still contains []; indexed rows are authoritative.
	if err := writer.Model(&assetdb.Record{}).Where("asset = ? AND ordinal = ?", "gacha_packs.json", 0).Update("json", `{"item_id":10002,"rewards":[{"item_id":10002,"quantity":3,"weight":10000}]}`).Error; err != nil {
		t.Fatal(err)
	}
	updated, err := loadLegacyFixture(path)
	if err != nil || updated.GachaPacks[10002].Rewards[0].Quantity != 3 || c.GachaPacks[10002].Rewards[0].Quantity != 2 {
		t.Fatal("reload snapshot", err)
	}
	// A malformed missing pool is a configuration error, not compatibility filtering.
	if err := writer.Model(&assetdb.Record{}).Where("asset = ? AND ordinal = ?", "gacha_packs.json", 1).Update("json", `{"item_id":1234,"rewards":[{"item_id":10002,"quantity":1,"weight":9999}]}`).Error; err != nil {
		t.Fatal(err)
	}
	if catalog, err := loadLegacyFixture(path); err == nil || catalog != nil {
		t.Fatal("malformed table published", catalog, err)
	}
}

func TestInstalledSQLGachaCompatibility(t *testing.T) {
	path := os.Getenv("WONDERLAND_TEST_ASSETS_DB")
	if path == "" {
		t.Skip("set WONDERLAND_TEST_ASSETS_DB")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join("..", "..", path)
	}
	c, err := loadLegacyFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.GachaPacks) != 20 || len(c.UnavailableGachaPacks) != 4 {
		t.Fatal("translated client compatibility", c.Summary())
	}
	for _, id := range []uint16{34381, 34382, 34383, 34384} {
		if c.GachaAvailable(id) || len(c.UnavailableGachaPacks[id].MissingItems) != 8 {
			t.Fatal("incompatible pool", id)
		}
	}
	for _, id := range []uint16{34171, 34385, 34388} {
		if !c.GachaAvailable(id) {
			t.Fatal("compatible pool", id)
		}
	}
}

func TestSQLLuckyDrawRewardsAndReload(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	row := assetdb.Record{Asset: "lucky_draw.json", Collection: "value", Ordinal: 0, JSON: `{"item_id":10002,"quantity":2,"weight":7,"slot":1}`}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	c, err := loadLegacyFixture(path)
	if err != nil || c.LuckyDraw.TotalWeight != 7 || len(c.LuckyDraw.Rewards) != 1 || c.LuckyDraw.Rewards[0].Quantity != 2 {
		t.Fatal(c, err)
	}
	if err := db.Model(&assetdb.Record{}).Where("asset = ?", "lucky_draw.json").Update("json", `{"item_id":10002,"quantity":3,"weight":9,"slot":1}`).Error; err != nil {
		t.Fatal(err)
	}
	updated, err := loadLegacyFixture(path)
	if err != nil || updated.LuckyDraw.TotalWeight != 9 || updated.LuckyDraw.Rewards[0].Quantity != 3 || c.LuckyDraw.Rewards[0].Quantity != 2 {
		t.Fatal("SQL snapshot reload", err)
	}
	if err := db.Model(&assetdb.Record{}).Where("asset = ?", "lucky_draw.json").Update("json", `{"item_id":999,"quantity":3,"weight":9,"slot":1}`).Error; err != nil {
		t.Fatal(err)
	}
	if c, err := loadLegacyFixture(path); err == nil || c != nil {
		t.Fatal("unknown reward accepted", c, err)
	}
}

func TestInstalledSQLLuckyDrawEqualDefaults(t *testing.T) {
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
	if len(c.LuckyDraw.Rewards) != 9 || c.LuckyDraw.TotalWeight != 9 {
		t.Fatal(c.LuckyDraw)
	}
	if reward := c.LuckyDraw.Rewards[8]; reward.ID != 34008 || reward.Quantity != 3 {
		t.Fatal("missing Fun Token default", reward)
	}
	for roll := int64(0); roll < 9; roll++ {
		r, ok := c.LuckyDraw.RewardForRoll(roll)
		if !ok || r.Weight != 1 || int64(r.Slot) != roll+1 {
			t.Fatal(r)
		}
	}
}

// Legacy decoding remains covered independently of the structured runtime.
func loadLegacyFixture(path string) (*assets.Catalog, error) {
	db, err := assetdb.OpenReadOnly(path)
	if err != nil {
		return nil, err
	}
	defer assetdb.Close(db)
	var c *assets.Catalog
	err = db.Transaction(func(tx *gorm.DB) error { var err error; c, err = loadLegacyTransaction(tx); return err })
	if err != nil {
		return nil, err
	}
	c.AssetsDatabase = path
	return c, nil
}
