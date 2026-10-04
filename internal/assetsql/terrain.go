package assetsql

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"wonderland-go/internal/assetdb"
	"wonderland-go/internal/assets"
)

// Offline-only projection: read individual imported Ground.MMG rows. Runtime
// reads the generated typed terrain table and never materializes this document.
func importedTerrains(tx *gorm.DB) (map[uint16]assets.Terrain, error) {
	out := map[uint16]assets.Terrain{}
	if !tx.Migrator().HasTable(&assetdb.Record{}) {
		return out, nil
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
			Name    string `json:"name"`
			Terrain struct {
				Width, Height uint32
				GridWidth     uint16 `json:"grid_width"`
				GridHeight    uint16 `json:"grid_height"`
				CellsHex      string `json:"cells_hex"`
			}
		}
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			return nil, fmt.Errorf("ground.mmg: %w", err)
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name), ".map") {
			continue
		}
		id, err := strconv.ParseUint(entry.Name[:len(entry.Name)-4], 10, 16)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("ground.mmg: invalid map name %q", entry.Name)
		}
		if _, ok := out[uint16(id)]; ok {
			return nil, fmt.Errorf("ground.mmg: duplicate map %d", id)
		}
		cells, err := hex.DecodeString(entry.Terrain.CellsHex)
		if err != nil {
			return nil, fmt.Errorf("terrain %d: %w", id, err)
		}
		out[uint16(id)] = assets.Terrain{Width: entry.Terrain.Width, Height: entry.Terrain.Height, GridWidth: entry.Terrain.GridWidth, GridHeight: entry.Terrain.GridHeight, Cells: cells}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, assets.ValidateTerrains(out)
}
