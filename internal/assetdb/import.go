// Package assetdb builds a disposable SQL catalog from maintained data exports.
package assetdb

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	ManifestSchemaVersion = 1
	ImportMetadataID      = 1
	ImportBatchRows       = 50
)

// Indexed top-level arrays are shared by import and runtime materialization.
var indexedCollections = []string{"items", "records", "entries", "maps", "groups", "rows", "value", "coefficients", "level_gates"}

type Manifest struct {
	SchemaVersion int     `json:"schema_version"`
	Priority      string  `json:"priority"`
	Assets        []Entry `json:"assets"`
}
type Entry struct {
	Asset        string   `json:"asset"`
	Origin       string   `json:"origin"`
	Source       string   `json:"source"`
	SourceSHA256 string   `json:"source_sha256"`
	Output       string   `json:"output"`
	OutputBytes  int64    `json:"output_bytes"`
	OutputSHA256 string   `json:"output_sha256"`
	Payload      *Payload `json:"payload"`
}
type Payload struct {
	Output string `json:"output"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type Document struct {
	Asset        string `gorm:"primaryKey"`
	Origin       string
	Source       string
	SourceSHA256 string
	OutputSHA256 string
	JSON         string `gorm:"type:text;not null"`
}

func (Document) TableName() string { return "asset_documents" }

type Record struct {
	Asset      string `gorm:"primaryKey;index:idx_asset_game_id,priority:1"`
	Collection string `gorm:"primaryKey"`
	Ordinal    int    `gorm:"primaryKey;autoIncrement:false"`
	GameID     *int64 `gorm:"index:idx_asset_game_id,priority:2"`
	Name       string `gorm:"index"`
	JSON       string `gorm:"type:text;not null"`
}

func (Record) TableName() string { return "asset_records" }

type ImportInfo struct {
	ID             int `gorm:"primaryKey;autoIncrement:false"`
	SchemaVersion  int
	ManifestSHA256 string
	ManifestJSON   string `gorm:"type:text;not null"`
}

func (ImportInfo) TableName() string { return "asset_import" }

type Summary struct{ Assets, Records int }

func Open(path string) (*gorm.DB, error) {
	return openDatabase(path, false)
}

func openDatabase(path string, readOnly bool) (*gorm.DB, error) {
	return openDatabaseMode(path, readOnly, readOnly)
}

func openDatabaseMode(path string, readOnly, immutable bool) (*gorm.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	address := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	query := address.Query()
	if readOnly {
		query.Set("mode", "ro")
	}
	if immutable {
		query.Set("immutable", "1")
	}
	query.Set("_foreign_keys", "on")
	query.Set("_busy_timeout", "5000")
	address.RawQuery = query.Encode()
	db, err := gorm.Open(sqlite.Open(address.String()), &gorm.Config{SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	return db, nil
}

func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func assetPath(root, relative string) (string, error) {
	if filepath.IsAbs(relative) || relative == "" {
		return "", fmt.Errorf("asset path must be relative: %s", relative)
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return "", err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(resolvedRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("asset path escapes data directory: %s", relative)
	}
	return path, nil
}

func verifiedJSON(root string, entry Entry) ([]byte, error) {
	path, err := assetPath(root, entry.Output)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(data)
	if int64(len(data)) != entry.OutputBytes || hex.EncodeToString(hash[:]) != entry.OutputSHA256 {
		return nil, fmt.Errorf("export checksum mismatch: %s", entry.Asset)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("invalid exported JSON: %s", entry.Asset)
	}
	var provenance struct {
		SourceSHA256 string `json:"source_sha256"`
		Format       string `json:"format"`
	}
	if err = json.Unmarshal(data, &provenance); err != nil {
		return nil, err
	}
	if provenance.Format == "plaintext-sqlite-snapshot" {
		return nil, fmt.Errorf("original database snapshots are excluded from assets: %s", entry.Asset)
	}
	if provenance.SourceSHA256 != entry.SourceSHA256 {
		return nil, fmt.Errorf("source provenance mismatch: %s", entry.Asset)
	}
	return data, nil
}

// Build imports everything into a NEW database. Callers publish it only after
// success. Each original JSON document is retained byte for byte, and its main
// collections receive indexed rows. Nested fields remain queryable with JSON1.
func Build(dataDirectory, path string, progress func(string)) (summary Summary, result error) {
	manifestBytes, err := os.ReadFile(filepath.Join(dataDirectory, "asset_manifest.json"))
	if err != nil {
		return summary, err
	}
	var manifest Manifest
	if err = json.Unmarshal(manifestBytes, &manifest); err != nil {
		return summary, err
	}
	if manifest.SchemaVersion != ManifestSchemaVersion || manifest.Priority != "client-over-server" || len(manifest.Assets) == 0 {
		return summary, fmt.Errorf("unsupported or empty asset manifest")
	}
	seen := map[string]bool{}
	for _, entry := range manifest.Assets {
		if entry.Asset == "" || seen[entry.Asset] {
			return summary, fmt.Errorf("duplicate or empty manifest asset: %s", entry.Asset)
		}
		if strings.EqualFold(entry.Asset, "serverdatabase.db") || entry.Output == "server/database_data.json" {
			return summary, fmt.Errorf("original database snapshots are excluded; regenerate the asset manifest")
		}
		if strings.EqualFold(entry.Asset, "database.override.txt") || entry.Output == "server/database.override.json" {
			return summary, fmt.Errorf("original database overrides are excluded; regenerate the asset manifest")
		}
		seen[entry.Asset] = true
	}
	// Create privately before SQLite opens the file. Exclusive creation prevents
	// clobbering an existing catalog.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return summary, fmt.Errorf("build destination must be a new file: %w", err)
	}
	if err = file.Close(); err != nil {
		return summary, err
	}
	db, err := Open(path)
	if err != nil {
		return summary, err
	}
	defer func() {
		if err := Close(db); result == nil {
			result = err
		}
	}()
	err = db.Transaction(func(tx *gorm.DB) error {
		var probe int
		if err := tx.Raw(`SELECT json_extract('{"ok":1}', '$.ok')`).Scan(&probe).Error; err != nil || probe != 1 {
			return fmt.Errorf("SQLite JSON support is required: %v", err)
		}
		if err := tx.AutoMigrate(&Document{}, &Record{}, &ImportInfo{}); err != nil {
			return err
		}
		digest := sha256.Sum256(manifestBytes)
		if err := tx.Create(&ImportInfo{ID: ImportMetadataID, SchemaVersion: ManifestSchemaVersion, ManifestSHA256: hex.EncodeToString(digest[:]), ManifestJSON: string(manifestBytes)}).Error; err != nil {
			return err
		}
		for _, entry := range manifest.Assets {
			data, err := verifiedJSON(dataDirectory, entry)
			if err != nil {
				return err
			}
			document := Document{Asset: entry.Asset, Origin: entry.Origin, Source: entry.Source, SourceSHA256: entry.SourceSHA256, OutputSHA256: entry.OutputSHA256, JSON: string(data)}
			if err := tx.Create(&document).Error; err != nil {
				return err
			}
			count, err := importRecords(tx, entry.Asset, data)
			if err != nil {
				return err
			}
			summary.Records += count
			summary.Assets++
			if progress != nil {
				progress(entry.Asset)
			}
		}
		for _, ddl := range views {
			if err := tx.Exec(ddl).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return summary, err
	}
	var integrity string
	if err := db.Raw("PRAGMA integrity_check").Scan(&integrity).Error; err != nil {
		return summary, err
	}
	if integrity != "ok" {
		return summary, fmt.Errorf("asset database integrity check: %s", integrity)
	}
	return summary, nil
}

func importRecords(tx *gorm.DB, asset string, data []byte) (int, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return 0, err
	}
	total := 0
	for _, collection := range indexedCollections {
		raw, exists := document[collection]
		if !exists {
			continue
		}
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 || raw[0] != '[' {
			continue
		}
		count, err := importArray(tx, asset, collection, raw)
		if err != nil {
			return total, err
		}
		total += count
	}

	return total, nil
}

func importArray(tx *gorm.DB, asset, collection string, raw []byte) (int, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if _, err := decoder.Token(); err != nil {
		return 0, err
	}
	batch := make([]Record, 0, ImportBatchRows)
	count := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := tx.Create(&batch).Error
		batch = batch[:0]
		return err
	}
	for decoder.More() {
		var row json.RawMessage
		if err := decoder.Decode(&row); err != nil {
			return count, err
		}
		gameID, name := recordIdentity(row)
		batch = append(batch, Record{Asset: asset, Collection: collection, Ordinal: count, GameID: gameID, Name: name, JSON: string(row)})
		count++
		if len(batch) == ImportBatchRows {
			if err := flush(); err != nil {
				return count, err
			}
		}
	}
	return count, flush()
}

func recordIdentity(raw []byte) (*int64, string) {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return nil, ""
	}
	for _, nested := range []string{"definition", "fields"} {
		if value, exists := object[nested]; exists {
			var fields map[string]json.RawMessage
			if json.Unmarshal(value, &fields) == nil {
				for key, value := range fields {
					if _, exists := object[key]; !exists {
						object[key] = value
					}
				}
			}
		}
	}
	var identifier *int64
	if raw, exists := object["id"]; exists {
		if value, err := strconv.ParseInt(string(raw), 10, 64); err == nil {
			identifier = &value
		}
	}
	name := ""
	if raw, exists := object["name"]; exists {
		if json.Unmarshal(raw, &name) != nil {
			var text struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(raw, &text) == nil {
				name = text.Text
			}
		}
	}
	return identifier, name
}

// Views are schema DDL, keeping import writes in GORM. All unresolved fields
// remain in record JSON; use json_extract/json_each for additional SQL columns.
var views = []string{
	`CREATE VIEW asset_lucky_draw_rewards AS SELECT ordinal AS reward_order,json_extract(json,'$.item_id') AS item_id,name,json_extract(json,'$.quantity') AS quantity,json_extract(json,'$.weight') AS weight,json_extract(json,'$.slot') AS slot,json FROM asset_records WHERE asset='lucky_draw.json' AND collection='value'`,
	`CREATE VIEW asset_gacha_packs AS SELECT json_extract(json,'$.item_id') AS item_id,ordinal AS pack_order,name,json FROM asset_records WHERE asset='gacha_packs.json' AND collection='value'`,
	`CREATE VIEW asset_gacha_rewards AS SELECT json_extract(p.json,'$.item_id') AS pack_id,p.ordinal AS pack_order,CAST(r.key AS INTEGER) AS reward_order,json_extract(r.value,'$.item_id') AS item_id,json_extract(r.value,'$.quantity') AS quantity,json_extract(r.value,'$.weight') AS weight FROM asset_records p,json_each(p.json,'$.rewards') r WHERE p.asset='gacha_packs.json' AND p.collection='value'`,
	`CREATE VIEW asset_skill_effects AS SELECT json_extract(json,'$.id') AS id,name,json FROM asset_records WHERE asset='skill_effects.json' AND collection='records'`,
	`CREATE VIEW asset_items AS SELECT game_id AS id,name,json_extract(json,'$.definition.type') AS type,json_extract(json,'$.definition.equip_slot') AS equip_slot,json_extract(json,'$.definition.level') AS level,json FROM asset_records WHERE asset='item.dat' AND collection='items'`,
	`CREATE VIEW asset_npcs AS SELECT game_id AS id,name,json_extract(json,'$.fields.level') AS level,json_extract(json,'$.fields.hp') AS hp,json_extract(json,'$.fields.sp') AS sp,json_extract(json,'$.fields.element') AS element,json FROM asset_records WHERE asset='npc.dat' AND collection='records'`,
	`CREATE VIEW asset_skills AS SELECT game_id AS id,name,json_extract(json,'$.fields.sp') AS sp,json_extract(json,'$.fields.element') AS element,json FROM asset_records WHERE asset='skill.dat' AND collection='records'`,
	`CREATE VIEW asset_dialogues AS SELECT game_id AS id,json_extract(json,'$.text.text') AS text,json FROM asset_records WHERE asset='talk.dat' AND collection='records'`,
	`CREATE VIEW asset_scenes AS SELECT game_id AS id,name,json FROM asset_records WHERE asset='scenedata.dat' AND collection='records'`,
	`CREATE VIEW asset_quest_marks AS SELECT game_id AS id,name,json_extract(json,'$.fields.completion_flag') AS completion_flag,json FROM asset_records WHERE asset='mark.dat' AND collection='records'`,
	`CREATE VIEW asset_audio AS SELECT asset,ordinal,name,json_extract(json,'$.file_offset') AS file_offset,json_extract(json,'$.bytes') AS bytes,json FROM asset_records WHERE asset IN ('odd.dat','odd_d01.dat') AND collection='entries'`,
}
