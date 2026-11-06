package assets

import (
	_ "embed"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
)

// ItemNamePatch restores a verified full display name without replacing edits.
type ItemNamePatch struct{ OriginalName, Name string }

//go:embed item_names.csv
var itemNamePatchCSV string

// ItemNamePatches supplies offline SQL initialization and client asset compilation.
// Runtime server definitions remain owned by the assets database.
func ItemNamePatches() (map[uint16]ItemNamePatch, error) {
	rows, err := csv.NewReader(strings.NewReader(itemNamePatchCSV)).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || strings.Join(rows[0], ",") != "id,original_name,name,source_ids" {
		return nil, fmt.Errorf("invalid item name patch header")
	}
	patches := make(map[uint16]ItemNamePatch, len(rows)-1)
	for _, row := range rows[1:] {
		id, err := strconv.ParseUint(row[0], 10, 16)
		if err != nil || id == 0 || len(row[1]) < nativeItemNameBytes-1 || len(row[2]) <= len(row[1]) || !strings.HasPrefix(row[2], row[1]) || row[3] == "" {
			return nil, fmt.Errorf("invalid item name patch %q", row)
		}
		if _, exists := patches[uint16(id)]; exists {
			return nil, fmt.Errorf("duplicate item name patch %d", id)
		}
		patches[uint16(id)] = ItemNamePatch{OriginalName: row[1], Name: row[2]}
	}
	return patches, nil
}

// ApplyItemNamePatches is used by offline server conversion.
// Native records and custom/full definition names remain untouched.
func ApplyItemNamePatches(items map[uint16]NativeItem) error {
	patches, err := ItemNamePatches()
	if err != nil {
		return err
	}
	for id, patch := range patches {
		if item, ok := items[id]; ok && item.Definition.Name == patch.OriginalName {
			item.Definition.Name = patch.Name
			items[id] = item
		}
	}
	return nil
}
