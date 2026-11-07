package assetsql

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

const repositorySeedFormat = "wonderland-server-catalog-sqlite-gzip"
const repositorySeedVersion = 1

type repositorySeedManifest struct {
	Format          string         `json:"format"`
	Version         int            `json:"version"`
	SchemaVersion   int            `json:"schema_version"`
	DatabaseBytes   int64          `json:"database_bytes"`
	DatabaseSHA256  string         `json:"database_sha256"`
	CompressedBytes int64          `json:"compressed_bytes"`
	Tables          int            `json:"tables"`
	Rows            int64          `json:"rows"`
	Definitions     assets.Summary `json:"definitions"`
}

// ExportRepositorySeed snapshots only typed shared definitions. It never
// exports gameplay state or client media, and strips original import documents.
// Call against an offline, current-schema asset database.
func ExportRepositorySeed(source, output string) error {
	catalog, err := LoadDatabase(source)
	if err != nil {
		return err
	}
	db, err := assetdb.OpenReadOnly(source)
	if err != nil {
		return err
	}
	defer assetdb.Close(db)
	if db.Migrator().HasTable("accounts") || db.Migrator().HasTable("characters") {
		return fmt.Errorf("refusing to export a database containing gameplay state")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(output), ".server-catalog-*.db")
	if err != nil {
		return err
	}
	path := temporary.Name()
	defer os.Remove(path)
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := db.Exec("VACUUM INTO ?", path).Error; err != nil {
		return err
	}
	manifest, err := stripRepositorySeed(path)
	if err != nil {
		return err
	}
	got, err := LoadDatabase(path)
	if err != nil {
		return err
	}
	catalog.AssetsDatabase, got.AssetsDatabase = "", ""
	if !reflect.DeepEqual(catalog, got) {
		return fmt.Errorf("repository seed changed shared definitions")
	}
	manifest.Definitions = got.Summary()
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	defer input.Close()
	zipped, err := os.CreateTemp(filepath.Dir(output), ".server-catalog-*.gz")
	if err != nil {
		return err
	}
	zipPath := zipped.Name()
	defer os.Remove(zipPath)
	writer, err := gzip.NewWriterLevel(zipped, gzip.BestCompression)
	if err != nil {
		zipped.Close()
		return err
	}
	digest := sha256.New()
	manifest.DatabaseBytes, err = io.Copy(writer, io.TeeReader(input, digest))
	if err != nil {
		writer.Close()
		zipped.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		zipped.Close()
		return err
	}
	if err := zipped.Close(); err != nil {
		return err
	}
	info, err := os.Stat(zipPath)
	if err != nil {
		return err
	}
	manifest.CompressedBytes = info.Size()
	manifest.DatabaseSHA256 = hex.EncodeToString(digest.Sum(nil))
	if err := os.Rename(zipPath, output); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(output+".json", append(data, '\n'), 0644)
}

func stripRepositorySeed(path string) (repositorySeedManifest, error) {
	m := repositorySeedManifest{Format: repositorySeedFormat, Version: repositorySeedVersion, SchemaVersion: catalogSchemaVersion}
	db, err := assetdb.Open(path)
	if err != nil {
		return m, err
	}
	defer assetdb.Close(db)
	allowed := map[string]bool{(catalogSchema{}).TableName(): true}
	for _, model := range catalogTables() {
		allowed[model.(interface{ TableName() string }).TableName()] = true
	}
	var objects []struct{ Type, Name string }
	if err := db.Raw("SELECT type,name FROM sqlite_master WHERE type IN ('view','trigger') ORDER BY type,name").Scan(&objects).Error; err != nil {
		return m, err
	}
	quote := func(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }
	for _, object := range objects {
		if err := db.Exec("DROP " + strings.ToUpper(object.Type) + " IF EXISTS " + quote(object.Name)).Error; err != nil {
			return m, err
		}
	}
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return m, err
	}
	for _, table := range tables {
		if strings.HasPrefix(table, "sqlite_") {
			continue
		}
		if !allowed[table] {
			if err := db.Exec("DROP TABLE " + quote(table)).Error; err != nil {
				return m, err
			}
			continue
		}
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			return m, err
		}
		m.Tables++
		m.Rows += count
	}
	// Empty import tables retain the assets database's existing role checks;
	// original client files, paths and archive payloads are not shipped.
	if err := db.AutoMigrate(&assetdb.Document{}, &assetdb.Record{}, &assetdb.ImportInfo{}); err != nil {
		return m, err
	}
	if err := db.Create(&assetdb.ImportInfo{ID: assetdb.ImportMetadataID, SchemaVersion: assetdb.ManifestSchemaVersion, ManifestJSON: `{"format":"repository-server-seed"}`}).Error; err != nil {
		return m, err
	}
	if err := db.Exec("VACUUM").Error; err != nil {
		return m, err
	}
	return m, nil
}
