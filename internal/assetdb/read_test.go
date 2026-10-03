package assetdb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeReadOnlyDatabaseAndAuthoritativeRows(t *testing.T) {
	root, data := fixture(t)
	path := filepath.Join(root, "assets.db")
	if _, err := Build(data, path, nil); err != nil {
		t.Fatal(err)
	}
	writer, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(writer)
	// Runtime readers must observe committed WAL changes, unlike immutable role probes.
	if err := writer.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(reader)
	if err := reader.Model(&Record{}).Where("asset = ?", "item.dat").Update("name", "forbidden").Error; err == nil {
		t.Fatal("runtime reader permitted writes")
	}
	edited := `{"definition":{"id":42,"name":"SQL edit"}}`
	if err := writer.Model(&Record{}).Where("asset = ? AND ordinal = ?", "item.dat", 0).Update("json", edited).Error; err != nil {
		t.Fatal(err)
	}
	if err := writer.Where("asset = ? AND ordinal = ?", "item.dat", 1).Delete(&Record{}).Error; err != nil {
		t.Fatal(err)
	}
	raw, err := ReadDocument(reader, "item.dat")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Items) != 1 || string(document.Items[0]) != edited {
		t.Fatalf("SQL rows not authoritative: %s", raw)
	}
	if err := writer.Where("asset = ?", "item.dat").Delete(&Record{}).Error; err != nil {
		t.Fatal(err)
	}
	raw, err = ReadDocument(reader, "item.dat")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Items) != 0 {
		t.Fatal("deleted SQL collection fell back to document")
	}
	if _, err := ReadDocument(reader, "absent.dat"); err == nil {
		t.Fatal("missing document accepted")
	}
}

func TestRuntimeMissingDatabaseIsNotCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.db")
	if db, err := OpenReadOnly(path); err == nil {
		Close(db)
		t.Fatal("missing database opened")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("database was created: %v", err)
	}
}
