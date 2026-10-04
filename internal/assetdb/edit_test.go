package assetdb

import (
	"bytes"
	"context"
	"errors"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
)

func TestAdminAssetEditConflictValidationRollbackAndProvenance(t *testing.T) {
	root, data := fixture(t)
	path := filepath.Join(root, "assets.db")
	if _, err := Build(data, path, nil); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(db)
	ctx := context.Background()
	before, err := ReadDocument(db, "item.dat")
	if err != nil {
		t.Fatal(err)
	}
	var row Record
	if err := db.Where(map[string]any{"asset": "item.dat", "ordinal": 0}).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	edited := []byte(`{"definition":{"id":43,"name":"Edited"},"unknown":123}`)
	reject := errors.New("invalid candidate")
	if err := ReplaceRecord(ctx, db, "item.dat", "items", 0, DocumentVersion([]byte(row.JSON)), edited, func(*gorm.DB) error { return reject }); !errors.Is(err, reject) {
		t.Fatal(err)
	}
	after, err := ReadDocument(db, "item.dat")
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed edit changed catalog", err)
	}
	if err := ReplaceRecord(ctx, db, "item.dat", "items", 0, "stale", edited, func(*gorm.DB) error { return nil }); !errors.Is(err, ErrEditConflict) {
		t.Fatal(err)
	}
	if err := ReplaceRecord(ctx, db, "item.dat", "items", 0, DocumentVersion([]byte(row.JSON)), edited, func(*gorm.DB) error { return nil }); err != nil {
		t.Fatal(err)
	}
	after, err = ReadDocument(db, "item.dat")
	if err != nil || !bytes.Contains(after, []byte("Edited")) || !bytes.Contains(after, []byte("unknown")) {
		t.Fatal(string(after), err)
	}
	if err := EnsureDocument(ctx, db, "item.dat"); err == nil {
		t.Fatal("initialized immutable imported document")
	}
	if err := EnsureDocument(ctx, db, "chest_drops.json"); err != nil {
		t.Fatal(err)
	}
	chest, err := ReadDocument(db, "chest_drops.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := ReplaceDocument(ctx, db, "chest_drops.json", DocumentVersion(chest), []byte(`{"schema_version":1,"value":[{"map_id":42}]}`), func(*gorm.DB) error { return reject }); !errors.Is(err, reject) {
		t.Fatal(err)
	}
	after, err = ReadDocument(db, "chest_drops.json")
	if err != nil || !bytes.Equal(chest, after) {
		t.Fatal("document rollback", err)
	}
}
