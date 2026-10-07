package assetsql

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

const (
	exportSchemaVersion        = 1
	maxRuntimeAssetBytes       = 64 << 20
	eveExportHeaderBytes       = 12
	eveExportIndexBytes        = 10
	animationExportIndexBytes  = 10
	animationExportFooterBytes = 2
	talkExportTextEndOffset    = 257
	talkExportMaxTextBytes     = 254
)

type exportRow struct {
	RecordIndex int `json:"record_index"`
	FileOffset  int `json:"file_offset"`
	Name        struct {
		Text string `json:"text"`
	} `json:"name"`
	Text struct {
		Text   string `json:"text"`
		Length int    `json:"length"`
	} `json:"text"`
	Fields     json.RawMessage `json:"fields"`
	DecodedHex string          `json:"decoded_hex"`
	IndexHex   string          `json:"index_hex"`
}
type databaseExport struct {
	SchemaVersion  int             `json:"schema_version"`
	Format         string          `json:"format"`
	SourceBytes    int             `json:"source_bytes"`
	RecordBytes    int             `json:"record_bytes"`
	HeaderHex      string          `json:"header_hex"`
	FooterCountHex string          `json:"footer_count_hex"`
	Records        []exportRow     `json:"records"`
	Maps           []exportRow     `json:"maps"`
	Gaps           []exportRow     `json:"gaps"`
	Value          json.RawMessage `json:"value"`
	Text           string          `json:"text"`
	Rows           [][]string      `json:"rows"`
}

// LoadDatabase is the server's exclusive runtime asset source. A single read
// transaction creates a consistent catalog snapshot. No source/export files
// are opened, and SQL failures never fall back to native data.
func LoadDatabase(path string) (*assets.Catalog, error) {
	db, err := assetdb.OpenReadOnly(path)
	if err != nil {
		return nil, fmt.Errorf("assets database %s: %w", path, err)
	}
	defer assetdb.Close(db)
	var catalog *assets.Catalog
	err = db.Transaction(func(tx *gorm.DB) error { var err error; catalog, err = LoadTransaction(tx); return err })
	if err != nil {
		return nil, fmt.Errorf("assets database %s: %w", path, err)
	}
	catalog.AssetsDatabase = path
	return catalog, nil
}

// loadLegacyTransaction is used only by offline conversion and reference tests.
func loadLegacyTransaction(tx *gorm.DB) (*assets.Catalog, error) {
	c := &assets.Catalog{Items: map[uint16]game.ItemDefinition{}, Warnings: []string{}}
	var err error
	err = func() error {
		var info assetdb.ImportInfo
		if err := tx.Select("schema_version").Where("id = ?", assetdb.ImportMetadataID).Take(&info).Error; err != nil {
			return fmt.Errorf("asset import metadata: %w", err)
		}
		if info.SchemaVersion != assetdb.ManifestSchemaVersion {
			return fmt.Errorf("unsupported asset database schema %d", info.SchemaVersion)
		}
		read := func(asset string) (databaseExport, error) {
			raw, err := assetdb.ReadDocument(tx, asset)
			if err != nil {
				return databaseExport{}, err
			}
			var doc databaseExport
			if err = json.Unmarshal(raw, &doc); err != nil {
				return doc, fmt.Errorf("asset %s: %w", asset, err)
			}
			if doc.SchemaVersion != exportSchemaVersion {
				return doc, fmt.Errorf("asset %s: unsupported export schema", asset)
			}
			return doc, nil
		}
		raw, err := assetdb.ReadDocument(tx, "item.dat")
		if err != nil {
			return err
		}
		c.Instances, err = importedInstances(tx)
		if err != nil {
			return err
		}
		c.NativeItems, err = assets.ParseItemCatalogJSON(raw)
		if err != nil {
			return fmt.Errorf("item.dat SQL catalog: %w", err)
		}
		for id, item := range c.NativeItems {
			c.Items[id] = item.Definition
		}
		var effectDocuments int64
		if err := tx.Model(&assetdb.Document{}).Where("asset = ?", assets.EffectCatalogAsset).Count(&effectDocuments).Error; err != nil {
			return err
		}
		if effectDocuments > 0 {
			raw, err := assetdb.ReadDocument(tx, assets.EffectCatalogAsset)
			if err != nil {
				return err
			}
			c.SkillEffects, err = assets.ParseEffectCatalog(raw)
			if err != nil {
				return fmt.Errorf("SQL effect catalog: %w", err)
			}
		}
		for _, asset := range []string{"npc.dat", "skill.dat", "talk.dat", "mark.dat", "compound2.dat", "compound.dat"} {
			doc, err := read(asset)
			if err != nil {
				return err
			}
			if doc.Format != "native-records" || len(doc.Records) == 0 {
				return fmt.Errorf("asset %s: missing native records", asset)
			}
			if err = loadNativeSQL(c, asset, doc); err != nil {
				return fmt.Errorf("asset %s: %w", asset, err)
			}
		}
		doc, err := read("eve.emg")
		if err != nil {
			return err
		}
		data, err := reconstructPlainExport(doc)
		if err != nil {
			return fmt.Errorf("eve.emg: %w", err)
		}
		c.Maps, err = assets.ParseEVE(data)
		if err != nil {
			return err
		}
		if len(c.Maps) == 0 {
			return fmt.Errorf("eve.emg: no maps")
		}
		doc, err = read("skilldata.mbtm")
		if err != nil {
			return err
		}
		data, err = reconstructPlainExport(doc)
		if err != nil {
			return fmt.Errorf("skilldata.mbtm: %w", err)
		}
		c.AnimationTiming, err = assets.ParseAnimationTiming(data)
		if err != nil {
			return err
		}
		for _, asset := range []string{"alchemy_recipes.txt", "starter_items.json", "pet_vouchers.json", "disabled_quest_events.csv", "monster_drops.txt", "npc_sale_prices.csv", "critical_hits.json", "item_mall.json", "gacha_packs.json", "lucky_draw.json", "mall_forging.json"} {
			doc, err := read(asset)
			if err != nil {
				return err
			}
			if err = loadSettingsSQL(c, asset, doc); err != nil {
				return fmt.Errorf("asset %s: %w", asset, err)
			}
		}

		for _, asset := range []string{assets.AdminMapsAsset, assets.AdminChestAsset} {
			var count int64
			if err := tx.Model(&assetdb.Document{}).Where(map[string]any{"asset": asset}).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				continue
			}
			doc, err := read(asset)
			if err != nil {
				return err
			}
			if asset == assets.AdminMapsAsset {
				var overrides []assets.Map
				if err := json.Unmarshal(doc.Value, &overrides); err != nil {
					return err
				}
				seen := map[uint16]bool{}
				for _, m := range overrides {
					if m.ID == 0 || seen[m.ID] {
						return fmt.Errorf("invalid map override %d", m.ID)
					}
					seen[m.ID] = true
					if _, ok := c.Maps[m.ID]; !ok {
						return fmt.Errorf("unknown override map %d", m.ID)
					}
					for _, w := range m.Warps {
						if w.MapID != 0 {
							if _, ok := c.Maps[w.MapID]; !ok {
								return fmt.Errorf("unknown warp map %d", w.MapID)
							}
						}
					}
					c.Maps[m.ID] = m
				}
			} else {
				c.ChestPools, err = assets.ParseChestPools(doc.Value, c)
				if err != nil {
					return err
				}
			}
		}
		var trialDocuments int64
		if err := tx.Model(&assetdb.Document{}).Where("asset = ?", assets.CombatTrialsAsset).Count(&trialDocuments).Error; err != nil {
			return err
		}
		if trialDocuments > 0 {
			doc, err := read(assets.CombatTrialsAsset)
			if err != nil {
				return err
			}
			c.CombatTrials, err = assets.ParseCombatTrials(doc.Value, c)
			if err != nil {
				return fmt.Errorf("combat trials: %w", err)
			}
		}
		var visibilityDocuments int64
		if err := tx.Model(&assetdb.Document{}).Where("asset = ?", assets.QuestVisibilityAsset).Count(&visibilityDocuments).Error; err != nil {
			return err
		}
		if visibilityDocuments > 0 {
			doc, err := read(assets.QuestVisibilityAsset)
			if err != nil {
				return err
			}
			c.QuestVisibility, err = assets.ParseQuestVisibility(doc.Value, c)
			if err != nil {
				return fmt.Errorf("quest visibility: %w", err)
			}
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}
	return c, nil
}

func loadNativeSQL(c *assets.Catalog, asset string, doc databaseExport) error {
	switch asset {
	case "npc.dat":
		c.NPCs = map[uint16]assets.NPC{}
		for _, row := range doc.Records {
			var fields struct {
				assets.NPC
				Strength, Constitution, Intelligence, Wisdom, Agility uint16
				Skills1                                               uint16 `json:"skill_1"`
				Skills2                                               uint16 `json:"skill_2"`
				Skills3                                               uint16 `json:"skill_3"`
				Drop1                                                 uint16 `json:"drop_1"`
				Drop2                                                 uint16 `json:"drop_2"`
				Drop3                                                 uint16 `json:"drop_3"`
				Drop4                                                 uint16 `json:"drop_4"`
				Drop5                                                 uint16 `json:"drop_5"`
			}
			if err := json.Unmarshal(row.Fields, &fields); err != nil {
				return err
			}
			n := fields.NPC
			if n.ID == 0 {
				continue
			}
			n.Name = row.Name.Text
			n.Stats = [5]uint16{fields.Strength, fields.Constitution, fields.Intelligence, fields.Wisdom, fields.Agility}
			n.Skills = [3]uint16{fields.Skills1, fields.Skills2, fields.Skills3}
			n.Drops = [5]uint16{fields.Drop1, fields.Drop2, fields.Drop3, fields.Drop4, fields.Drop5}
			c.NPCs[n.ID] = n
		}
	case "skill.dat":
		c.Skills = map[uint16]assets.Skill{}
		for _, row := range doc.Records {
			var skill assets.Skill
			if err := json.Unmarshal(row.Fields, &skill); err != nil {
				return err
			}
			skill.Name = row.Name.Text
			if err := assets.ResolveSkillEffects(&skill, c.SkillEffects); err != nil {
				return fmt.Errorf("skill %d effects: %w", skill.ID, err)
			}
			if skill.ID != 0 && skill.Name != "" {
				c.Skills[skill.ID] = skill
			}
		}
	case "mark.dat":
		c.Marks = map[uint16]uint16{}
		for _, row := range doc.Records {
			var fields struct {
				ID   uint16
				Flag uint16 `json:"completion_flag"`
			}
			if err := json.Unmarshal(row.Fields, &fields); err != nil {
				return err
			}
			if fields.ID != 0 {
				c.Marks[fields.ID] = fields.Flag
			}
		}
	case "talk.dat":
		c.Talks = assets.Talks{ByID: map[uint32]string{}, ByIndex: map[uint32]string{}, ByOffset: map[uint32]string{}}
		for _, row := range doc.Records {
			var fields struct{ ID uint32 }
			if err := json.Unmarshal(row.Fields, &fields); err != nil {
				return err
			}
			text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(row.Text.Text), "fffff"))
			length := row.Text.Length
			if text == "" || length < 1 || length > talkExportMaxTextBytes {
				continue
			}
			if row.FileOffset < 0 || row.RecordIndex < 0 {
				return fmt.Errorf("invalid dialogue position")
			}
			if fields.ID > 0 {
				c.Talks.ByID[fields.ID] = text
			}
			c.Talks.ByIndex[uint32(row.RecordIndex)] = text
			c.Talks.ByOffset[uint32(row.FileOffset)] = text
			c.Talks.ByOffset[uint32(row.FileOffset+talkExportTextEndOffset-length)] = text
		}
	case "compound.dat", "compound2.dat":
		for _, row := range doc.Records {
			var fields struct {
				Output uint16 `json:"result_id"`
				First  uint16 `json:"material_1_id"`
				Second uint16 `json:"material_2_id"`
			}
			if err := json.Unmarshal(row.Fields, &fields); err != nil {
				return err
			}
			if fields.Output != 0 && fields.First != 0 && fields.Second != 0 {
				c.AlchemyRecipes = append(c.AlchemyRecipes, assets.AlchemyRecipe{Input1: fields.First, Input2: fields.Second, Output: fields.Output})
			}
		}
	}
	return nil
}

func loadSettingsSQL(c *assets.Catalog, asset string, doc databaseExport) (err error) {
	// CSV rows come from asset_records; retained document text is provenance only.
	if asset == "disabled_quest_events.csv" || asset == "npc_sale_prices.csv" {
		var buffer bytes.Buffer
		writer := csv.NewWriter(&buffer)
		if err := writer.WriteAll(doc.Rows); err != nil {
			return err
		}
		doc.Text = buffer.String()
	}

	switch asset {
	case "mall_forging.json":
		c.Forging, err = assets.ParseForging(doc.Value, c.Items)
		if err != nil {
			c.Warnings = append(c.Warnings, asset+": "+err.Error())
			err = nil
		}
	case "starter_items.json":
		c.StarterItems, err = assets.ParseStarterItems(doc.Value)
	case "pet_vouchers.json":
		c.PetVouchers, err = assets.ParsePetVouchers(doc.Value)
	case "disabled_quest_events.csv":
		c.DisabledEvents, err = assets.ParseDisabledEvents([]byte(doc.Text))
	case "monster_drops.txt":
		c.Drops, err = assets.ParseDrops([]byte(doc.Text))
	case "npc_sale_prices.csv":
		c.SalePrices, err = assets.ParseSalePrices([]byte(doc.Text))
	case "critical_hits.json":
		c.Critical, err = game.ParseCriticalHits(doc.Value)
	case "lucky_draw.json":
		c.LuckyDraw, err = assets.ParseLuckyDrawPool(doc.Value, c.Items)
	case "gacha_packs.json":
		var warnings []assets.GachaPackIssue
		c.GachaPacks, warnings, err = assets.ParseCompatibleGachaPacks(doc.Value, c.Items)
		c.UnavailableGachaPacks = map[uint16]assets.GachaPackIssue{}
		for _, warning := range warnings {
			c.UnavailableGachaPacks[warning.ID] = warning
			c.Warnings = append(c.Warnings, asset+": "+warning.String())
		}
	case "item_mall.json":
		err = json.Unmarshal(doc.Value, &c.Mall)
		if err != nil {
			return err
		}
		for _, item := range c.Mall {
			if item.ID == 0 || item.Count == 0 || item.Cost < 0 {
				return fmt.Errorf("invalid mall item %d", item.ID)
			}
		}
	case "alchemy_recipes.txt":
		var recipes []assets.AlchemyRecipe
		for _, line := range strings.Split(doc.Text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			halves := strings.SplitN(line, "|", 2)
			if len(halves) != 2 {
				return fmt.Errorf("invalid recipe line")
			}
			inputs := strings.Split(strings.TrimSpace(halves[0]), ",")
			outputs := strings.Split(strings.TrimSpace(halves[1]), ",")
			if len(inputs) != 2 || len(outputs) < 1 {
				return fmt.Errorf("invalid recipe columns")
			}
			values := [3]uint16{}
			for i, s := range []string{inputs[0], inputs[1], outputs[0]} {
				n, e := strconv.ParseUint(strings.TrimSpace(s), 10, 16)
				if e != nil || n == 0 {
					return fmt.Errorf("invalid recipe ID %q", s)
				}
				values[i] = uint16(n)
			}
			recipes = append(recipes, assets.AlchemyRecipe{Input1: values[0], Input2: values[1], Output: values[2]})
		}
		// Authored defaults precede Compound2 and Compound, as in AlchemyManager.
		c.AlchemyRecipes = append(recipes, c.AlchemyRecipes...)
	}
	return err
}

// EVE and animation exports are already plaintext. Reassemble only their SQL
// bytes in memory to reuse the independently tested native layout parsers.
func reconstructPlainExport(doc databaseExport) ([]byte, error) {
	decode := func(value string) ([]byte, error) { return hex.DecodeString(value) }
	switch doc.Format {
	case "plaintext-eve":
		if doc.SourceBytes < eveExportHeaderBytes || doc.SourceBytes > maxRuntimeAssetBytes {
			return nil, fmt.Errorf("invalid EVE size")
		}
		data := make([]byte, doc.SourceBytes)
		put := func(offset int, value string) error {
			b, err := decode(value)
			if err != nil {
				return err
			}
			if offset < 0 || offset > len(data)-len(b) {
				return fmt.Errorf("EVE block outside source bounds")
			}
			copy(data[offset:], b)
			return nil
		}
		header, err := decode(doc.HeaderHex)
		if err != nil || len(header) != eveExportHeaderBytes {
			return nil, fmt.Errorf("invalid EVE header")
		}
		copy(data, header)
		if int(binary.LittleEndian.Uint32(header[8:])) != len(doc.Maps) {
			return nil, fmt.Errorf("EVE map count differs from SQL rows")
		}
		for i, row := range doc.Maps {
			index, err := decode(row.IndexHex)
			if err != nil || len(index) != eveExportIndexBytes {
				return nil, fmt.Errorf("invalid EVE index")
			}
			if err = put(eveExportHeaderBytes+i*eveExportIndexBytes, row.IndexHex); err != nil {
				return nil, err
			}
			if err = put(row.FileOffset, row.DecodedHex); err != nil {
				return nil, err
			}
		}
		for _, row := range doc.Gaps {
			if err := put(row.FileOffset, row.DecodedHex); err != nil {
				return nil, err
			}
		}
		return data, nil
	case "plaintext-animation-archive":
		var data, index bytes.Buffer
		for _, row := range doc.Records {
			block, err := decode(row.DecodedHex)
			if err != nil {
				return nil, err
			}
			entry, err := decode(row.IndexHex)
			if err != nil || len(entry) != animationExportIndexBytes {
				return nil, fmt.Errorf("invalid animation index")
			}
			data.Write(block)
			index.Write(entry)
			if data.Len()+index.Len() > maxRuntimeAssetBytes {
				return nil, fmt.Errorf("animation data too large")
			}
		}
		footer, err := decode(doc.FooterCountHex)
		if err != nil || len(footer) != animationExportFooterBytes {
			return nil, fmt.Errorf("invalid animation footer")
		}
		data.Write(index.Bytes())
		data.Write(footer)
		return data.Bytes(), nil
	default:
		return nil, fmt.Errorf("unsupported plaintext format %q", doc.Format)
	}
}
