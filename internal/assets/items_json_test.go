package assets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func itemJSONFixture(t *testing.T) NativeItemExport {
	t.Helper()
	data, _ := nativeItemGolden(t)
	export, err := ExportNativeItems(data, "unavailable-source/Item.dat")
	if err != nil {
		t.Fatal(err)
	}
	return export
}

func TestItemJSONDefinitionsAreAuthoritativeAndBinaryFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	export := itemJSONFixture(t)
	export.Items[0].Definition.Name = "JSON name"
	export.Items[0].Definition.Values = [2]int32{12345, -100}
	data, err := json.Marshal(export)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "items.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	// Attempting to read either old path would fail. They are irrelevant to loading.
	for _, name := range []string{"Item.dat", "itemDat.wpdat"} {
		if err = os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	c, err := Load(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Items) != 1 || c.Items[10002] != export.Items[0].Definition || c.ItemCatalog != path || len(c.NativeItems) != 1 {
		t.Fatal("JSON not authoritative", c.Summary())
	}
	for _, warning := range c.Warnings {
		if strings.HasPrefix(warning, "Item.dat:") || strings.HasPrefix(warning, "itemDat.wpdat:") {
			t.Fatal("binary consulted", warning)
		}
	}
	// Removing JSON must fail even when legacy files are available.
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(dir, path); err == nil || c != nil {
		t.Fatal("missing JSON accepted")
	}
	dataNative, _ := nativeItemGolden(t)
	if err = os.Remove(filepath.Join(dir, "Item.dat")); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "Item.dat"), dataNative, 0600); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(dir, path); err == nil || c != nil {
		t.Fatal("binary fallback enabled")
	}
	if err = os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(dir, path); err == nil || c != nil {
		t.Fatal("corrupt JSON fell back")
	}
}

func TestItemJSONRejectsCorruptCatalogs(t *testing.T) {
	for _, name := range []string{"schema", "format", "native version", "record size", "empty items", "source size", "header", "hash", "position", "offset", "zero id", "duplicate id", "record length", "record id", "unknown field", "trailing data"} {
		t.Run(name, func(t *testing.T) {
			export := itemJSONFixture(t)
			switch name {
			case "schema":
				export.SchemaVersion++
			case "format":
				export.Format = "other"
			case "native version":
				export.NativeVersion++
			case "record size":
				export.RecordBytes--
			case "empty items":
				export.Items = nil
			case "source size":
				export.SourceBytes--
			case "header":
				export.HeaderHex = "00"
			case "hash":
				export.SourceSHA256 = "bad"
			case "position":
				export.Items[0].RecordIndex++
			case "offset":
				export.Items[0].FileOffset++
			case "zero id":
				export.Items[0].Definition.ID = 0
			case "duplicate id":
				row := export.Items[0]
				row.RecordIndex = 2
				row.FileOffset = 902
				export.Items = append(export.Items, row)
				export.SourceBytes += 451
			case "record length":
				export.Items[0].DecodedHex = "00"
			case "record id":
				export.Items[0].Definition.ID++
			}
			data, err := json.Marshal(export)
			if err != nil {
				t.Fatal(err)
			}
			if name == "unknown field" {
				data = append([]byte(`{"unexpected":true,`), data[1:]...)
			}
			if name == "trailing data" {
				data = append(data, []byte(` {}`)...)
			}
			if items, err := ParseItemCatalogJSON(data); err == nil || items != nil {
				t.Fatal("invalid JSON published items")
			}
		})
	}
}

func TestMaintainedItemJSONCensus(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("../..", "data/item_data.json"))
	if err != nil {
		t.Fatal(err)
	}
	items, err := ParseItemCatalogJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 7312 || items[10002].Definition.Name != "Plate" || items[27025].Definition.Type != 20 {
		t.Fatal("maintained JSON catalog changed")
	}
	for _, id := range []uint16{34381, 21303, 61370} {
		if _, ok := items[id]; ok {
			t.Fatal("non-JSON item imported", id)
		}
	}
}
