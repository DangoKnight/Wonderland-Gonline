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

// NearestWalkable finds the nearest interior land point to a water position.
// Exhaustive cell search is used only for exceptional recovery transitions.
func (t Terrain) NearestWalkable(x, y int) (uint16, uint16, bool) {
	if t.Walkable(x, y) && t.Cells[(x/TerrainCellSize)*int(t.GridHeight)+y/TerrainCellSize] != terrainAlternateWater {
		return uint16(x), uint16(y), true
	}
	best := int64(^uint64(0) >> 1)
	bx, by, found := 0, 0, false
	for cx := 0; cx < int(t.GridWidth); cx++ {
		for cy := 0; cy < int(t.GridHeight); cy++ {
			at := cx*int(t.GridHeight) + cy
			if at >= len(t.Cells) || t.Cells[at]&TerrainBlockedMask != 0 || t.Cells[at] == terrainAlternateWater {
				continue
			}
			px := max(cx*TerrainCellSize+1, min(x, min((cx+1)*TerrainCellSize-1, int(t.Width)-1)))
			py := max(cy*TerrainCellSize+1, min(y, min((cy+1)*TerrainCellSize-1, int(t.Height)-1)))
			if !t.Walkable(px, py) {
				continue
			}
			dx, dy := int64(px-x), int64(py-y)
			if distance := dx*dx + dy*dy; distance < best {
				best, bx, by, found = distance, px, py, true
			}
		}
	}
	return uint16(bx), uint16(by), found
}

const terrainAlternateWater = 8
