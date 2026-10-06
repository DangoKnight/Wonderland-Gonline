package assets

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"wonderland-gonline/internal/game"
)

// ParseItemCatalogJSON reads the maintained export without consulting its source
// path or either binary catalog. Readable definitions are the gameplay values;
// the decoded records preserve source data for further migration work.
func ParseItemCatalogJSON(data []byte) (map[uint16]NativeItem, error) {
	var export NativeItemExport
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&export); err != nil {
		return nil, err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("item JSON: trailing data")
	}
	if export.SchemaVersion != nativeItemExportSchemaVersion || export.Format != nativeItemExportFormat || export.NativeVersion != nativeItemEncryptedVersion || export.RecordBytes != nativeItemRecordBytes {
		return nil, fmt.Errorf("item JSON: unsupported schema or native format")
	}
	if len(export.Items) == 0 || export.SourceBytes != (len(export.Items)+nativeItemHeaderRecords)*nativeItemRecordBytes {
		return nil, fmt.Errorf("item JSON: invalid record count or source size")
	}
	header, err := hex.DecodeString(export.HeaderHex)
	if err != nil || len(header) != nativeItemRecordBytes || le.Uint32(header[nativeItemVersionOffset:]) != nativeItemEncryptedVersion {
		return nil, fmt.Errorf("item JSON: invalid header")
	}
	digest, err := hex.DecodeString(export.SourceSHA256)
	if err != nil || len(digest) != nativeItemSourceHashBytes {
		return nil, fmt.Errorf("item JSON: invalid source hash")
	}
	items := make(map[uint16]NativeItem, len(export.Items))
	for i, row := range export.Items {
		index := i + nativeItemHeaderRecords
		if row.RecordIndex != index || row.FileOffset != index*nativeItemRecordBytes {
			return nil, fmt.Errorf("item JSON: invalid position at record %d", index)
		}
		id := row.Definition.ID
		if id == 0 {
			return nil, fmt.Errorf("item JSON: zero item ID at record %d", index)
		}
		if _, exists := items[id]; exists {
			return nil, fmt.Errorf("item JSON: duplicate item ID %d", id)
		}
		decoded, err := hex.DecodeString(row.DecodedHex)
		if err != nil || len(decoded) != nativeItemRecordBytes {
			return nil, fmt.Errorf("item JSON: invalid decoded record %d", index)
		}
		if le.Uint16(decoded[nativeItemIDOffset:]) != id {
			return nil, fmt.Errorf("item JSON: mismatched record ID %d", id)
		}
		item := NativeItem{Definition: row.Definition, Description: row.Description, Icon: row.Icon, LargeIcon: row.LargeIcon, Sprites: row.Sprites}
		copy(item.Record[:], decoded)
		item.InitializeInventoryDimensions()
		items[id] = item
	}
	return items, nil
}

func (c *Catalog) loadItems(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("item catalog %s: %w", path, err)
	}
	items, err := ParseItemCatalogJSON(data)
	if err != nil {
		return fmt.Errorf("item catalog %s: %w", path, err)
	}
	c.Items = make(map[uint16]game.ItemDefinition, len(items))
	c.NativeItems = items
	c.ItemCatalog = path
	for id, item := range items {
		c.Items[id] = item.Definition
	}
	return nil
}
