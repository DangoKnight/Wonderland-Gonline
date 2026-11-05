package assetsql

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

// Offline migration reads retained native bytes, never the operator's current
// filesystem exports. Runtime reads only the typed catalog projection.
func importedBattleBackdrops(tx *gorm.DB) (map[uint16]assets.BattleBackdrop, error) {
	values := map[uint16]assets.BattleBackdrop{}
	if !tx.Migrator().HasTable(&assetdb.Record{}) {
		return values, nil
	}
	rows, err := tx.Model(&assetdb.Record{}).Select("json").Where("asset = ? AND collection = ?", "ground.mmg", "entries").Order("ordinal").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var entry struct {
			Name       string
			DecodedHex string `json:"decoded_hex"`
		}
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			return nil, err
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name), ".map") {
			continue
		}
		id, err := strconv.ParseUint(strings.TrimSuffix(strings.ToLower(entry.Name), ".map"), 10, 16)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid backdrop map %q", entry.Name)
		}
		if _, exists := values[uint16(id)]; exists {
			return nil, fmt.Errorf("duplicate backdrop map %d", id)
		}
		data, err := hex.DecodeString(entry.DecodedHex)
		if err != nil {
			return nil, err
		}
		b, err := assets.DecodeBattleBackdrop(data)
		if err != nil {
			return nil, fmt.Errorf("map %d backdrop: %w", id, err)
		}
		values[uint16(id)] = b
	}
	return values, rows.Err()
}

func migrateBattleBackdrops(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_battle_backdrops", "battle_backdrops", func() error {
		values, err := importedBattleBackdrops(tx)
		if err != nil {
			return err
		}
		return writeBattleBackdrops(tx, values)
	})
}
