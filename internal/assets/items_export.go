package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"wonderland-go/internal/game"
)

const (
	nativeItemExportSchemaVersion = 1
	nativeItemExportFormat        = "wonderland-native-item-dat"
	nativeItemSourceHashBytes     = sha256.Size
)

// NativeItemExport preserves file order and the plaintext header. DecodedHex
// retains every record byte, including unresolved fields and text padding.
type NativeItemExport struct {
	SchemaVersion int                      `json:"schema_version"`
	Format        string                   `json:"format"`
	Source        string                   `json:"source"`
	SourceSHA256  string                   `json:"source_sha256"`
	SourceBytes   int                      `json:"source_bytes"`
	NativeVersion uint32                   `json:"native_version"`
	RecordBytes   int                      `json:"record_bytes"`
	HeaderHex     string                   `json:"header_hex"`
	Items         []NativeItemExportRecord `json:"items"`
}

type NativeItemExportRecord struct {
	RecordIndex int                           `json:"record_index"`
	FileOffset  int                           `json:"file_offset"`
	Definition  game.ItemDefinition           `json:"definition"`
	Description string                        `json:"description"`
	Icon        uint16                        `json:"icon"`
	LargeIcon   uint16                        `json:"large_icon"`
	Sprites     [nativeItemSpriteCount]uint16 `json:"sprites"`
	DecodedHex  string                        `json:"decoded_hex"`
}

// ExportNativeItems keeps all bytes of catalogs with unique nonzero item IDs.
// Refuse files the lookup parser would collapse instead of silently dropping
// records. The translated client catalog satisfies this constraint.
func ExportNativeItems(data []byte, source string) (NativeItemExport, error) {
	items, err := ParseNativeItems(data)
	if err != nil {
		return NativeItemExport{}, err
	}
	recordCount := len(data)/nativeItemRecordBytes - nativeItemHeaderRecords
	if len(items) != recordCount {
		return NativeItemExport{}, fmt.Errorf("native item export: duplicate or zero IDs would omit records")
	}
	digest := sha256.Sum256(data)
	out := NativeItemExport{
		SchemaVersion: nativeItemExportSchemaVersion, Format: nativeItemExportFormat,
		Source: source, SourceSHA256: hex.EncodeToString(digest[:]), SourceBytes: len(data),
		NativeVersion: le.Uint32(data[nativeItemVersionOffset:]), RecordBytes: nativeItemRecordBytes,
		HeaderHex: hex.EncodeToString(data[:nativeItemRecordBytes]), Items: make([]NativeItemExportRecord, 0, recordCount),
	}
	for offset := nativeItemHeaderRecords * nativeItemRecordBytes; offset < len(data); offset += nativeItemRecordBytes {
		id := (le.Uint16(data[offset+nativeItemIDOffset:]) ^ nativeItemWordMask) - nativeItemValueBias
		item := items[id]
		out.Items = append(out.Items, NativeItemExportRecord{
			RecordIndex: offset / nativeItemRecordBytes, FileOffset: offset, Definition: item.Definition,
			Description: item.Description, Icon: item.Icon, LargeIcon: item.LargeIcon, Sprites: item.Sprites,
			DecodedHex: hex.EncodeToString(item.Record[:]),
		})
	}
	return out, nil
}
