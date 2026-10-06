package assets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"golang.org/x/text/encoding/traditionalchinese"
	"sort"
	"strings"
	"unicode/utf8"
	"wonderland-gonline/internal/game"
)

const (
	convertedItemCellHeightOffset = 33
	convertedItemCellWidthOffset  = 34
	convertedItemRecordBytes      = 47
	convertedItemNameOffset       = 1
	convertedItemNameBytes        = 20
	convertedItemTypeOffset       = 21
	convertedItemIDOffset         = 22
	convertedItemEquipSlotOffset  = 28
	convertedItemLevelOffset      = 30
	convertedItemStatusOffset     = 35
	convertedItemValueOffset      = 39
)

// ParseConvertedItems reads the packed PhxItemInfo records used by
// .codex-verify/Inspect-InventoryData.ps1. ParseNativeItems handles encrypted Item.dat.
func ParseConvertedItems(data []byte) (map[uint16]game.ItemDefinition, error) {
	if len(data) == 0 || len(data)%convertedItemRecordBytes != 0 {
		return nil, fmt.Errorf("converted item catalog: invalid record length")
	}
	out := map[uint16]game.ItemDefinition{}
	for offset := 0; offset < len(data); offset += convertedItemRecordBytes {
		b := data[offset : offset+convertedItemRecordBytes]
		id := le.Uint16(b[convertedItemIDOffset:])
		if id == 0 {
			continue
		}
		name, err := decodeItemText(b[convertedItemNameOffset : convertedItemNameOffset+convertedItemNameBytes])
		if err != nil {
			return nil, err
		}
		v := game.ItemDefinition{CellWidth: b[convertedItemCellWidthOffset], CellHeight: b[convertedItemCellHeightOffset], ID: id, Name: name, Type: b[convertedItemTypeOffset], EquipSlot: le.Uint16(b[convertedItemEquipSlotOffset:]), Level: le.Uint16(b[convertedItemLevelOffset:]), Status: [2]uint16{le.Uint16(b[convertedItemStatusOffset:]), le.Uint16(b[convertedItemStatusOffset+2:])}, Values: [2]int32{int32(le.Uint32(b[convertedItemValueOffset:])), int32(le.Uint32(b[convertedItemValueOffset+4:]))}}
		// The C# lookup uses the first matching record.
		if _, exists := out[id]; !exists {
			out[id] = v
		}
	}
	return out, nil
}

func ParseStarterItems(data []byte) ([]game.StarterGrant, error) {
	var records []struct {
		Order int    `json:"OrderIdx"`
		ID    uint16 `json:"ItemID"`
		Count int    `json:"Count"`
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	if len(records) == 0 || len(records) > 50 {
		return nil, fmt.Errorf("invalid starter item count")
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].Order < records[j].Order })
	grants := make([]game.StarterGrant, 0, len(records))
	for _, r := range records {
		if r.ID == 0 || r.Count < 1 || r.Count > 2500 {
			return nil, fmt.Errorf("invalid starter item %d", r.ID)
		}
		grants = append(grants, game.StarterGrant{ID: r.ID, Count: r.Count})
	}
	return grants, nil
}

// Item tables contain ASCII/UTF-8 or legacy Big5 text. Keep the existing
// converted-catalog policy for both formats.
func decodeItemText(data []byte) (string, error) {
	data = bytes.TrimRight(data, "\x00 ")
	if !utf8.Valid(data) {
		decoded, err := traditionalchinese.Big5.NewDecoder().Bytes(data)
		if err != nil {
			return "", err
		}
		data = decoded
	}
	return strings.TrimSpace(string(data)), nil
}
