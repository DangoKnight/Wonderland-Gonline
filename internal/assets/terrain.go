package assets

import "fmt"

// Terrain contains only server-relevant Ground.MMG geometry. Cells are X-major,
// with 20-pixel spacing (aLogin FUN_004121a8 and FUN_0041a9b8).
const TerrainCellSize = 20
const TerrainGridLimit = 900
const TerrainBlockedMask byte = 0x17

type Terrain struct {
	Width, Height         uint32
	GridWidth, GridHeight uint16
	Cells                 []byte // Raw collision values, persisted as a SQL BLOB, not image assets.
}

func ValidateTerrains(values map[uint16]Terrain) error {
	for id, t := range values {
		if id == 0 || t.Width == 0 || t.Height == 0 || t.Width > TerrainGridLimit*TerrainCellSize || t.Height > TerrainGridLimit*TerrainCellSize || t.GridWidth == 0 || t.GridHeight == 0 || t.GridWidth > TerrainGridLimit || t.GridHeight > TerrainGridLimit || t.Width > uint32(t.GridWidth)*TerrainCellSize || t.Height > uint32(t.GridHeight)*TerrainCellSize || len(t.Cells) != int(t.GridWidth)*int(t.GridHeight) {
			return fmt.Errorf("terrain %d: invalid scene/grid dimensions or cell count", id)
		}
	}
	return nil
}

func (t Terrain) Contains(x, y int) bool {
	return x >= 0 && y >= 0 && uint32(x) < t.Width && uint32(y) < t.Height
}
func (t Terrain) Walkable(x, y int) bool {
	if !t.Contains(x, y) {
		return false
	}
	cx, cy := x/TerrainCellSize, y/TerrainCellSize
	if cx >= int(t.GridWidth) || cy >= int(t.GridHeight) {
		return false
	}
	at := cx*int(t.GridHeight) + cy
	return at < len(t.Cells) && t.Cells[at]&TerrainBlockedMask == 0
}
