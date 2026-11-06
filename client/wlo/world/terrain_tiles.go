package world

import (
	"fmt"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientruntime"
)

const (
	terrainTileStepX           = 180
	terrainTileStepY           = 90
	terrainTileOriginStep      = 181
	terrainFloorTop            = 1
	terrainGridIsometric       = 1
	terrainWallStyleFirst      = 1
	terrainWallStyleLast       = 4
	terrainWallTop             = -143
	terrainWallLowVariant      = 17
	terrainWallLowTop          = 7
	terrainWallSpecialResource = 10100
	terrainWallSpecialLift     = 3
	terrainGridMaxSide         = 38 // Native row stride 0x2d2 / 0x13-byte runtime cells.
)

type TerrainTile struct {
	X, Y, Depth int
	Sorted      bool
	Image       *surface.Surface
}

// FUN_003fb2b4 draws walls independently of their authored signed depth line.
// Background grid cells use FUN_003faa10's one-pixel lower baseline.
func (s *Scene) loadTerrainTiles(a login.Assets, grid *clientruntime.TileGrid) error {
	if grid == nil || grid.Mode != terrainGridIsometric {
		return nil
	}
	if grid.Rows <= 0 || grid.Rows > terrainGridMaxSide || grid.Columns <= 0 || grid.Columns > terrainGridMaxSide || len(grid.Cells) != grid.Rows*grid.Columns {
		return fmt.Errorf("map %d: invalid terrain tile grid", s.MapID)
	}
	images := map[string]*surface.Surface{}
	load := func(name string) (*surface.Surface, error) {
		if im := images[name]; im != nil {
			return im, nil
		}
		im, err := a.LoadPicture("images2", name)
		if err != nil {
			return nil, fmt.Errorf("map %d terrain tile %s: %w", s.MapID, name, err)
		}
		images[name] = keyed(im)
		return images[name], nil
	}
	for i, cell := range grid.Cells {
		row, col := i/grid.Columns, i%grid.Columns
		x, y := (grid.Columns-1)*terrainTileOriginStep+(row-col)*terrainTileStepX, (row+col)*terrainTileStepY
		if cell.Floor != 0 && cell.FloorVariant != 0 {
			name := fmt.Sprintf("%d%c", cell.Floor, 'A'+cell.FloorVariant-1)
			im, err := load(name)
			if err != nil {
				return err
			}
			s.Tiles = append(s.Tiles, TerrainTile{X: x, Y: y + terrainFloorTop, Image: im})
		}
		if cell.Wall != 0 {
			name := fmt.Sprintf("%d", uint32(cell.Wall)+uint32(cell.WallVariant))
			if cell.WallStyle >= terrainWallStyleFirst && cell.WallStyle <= terrainWallStyleLast {
				name += string(rune('A' + cell.WallStyle - 1))
			}
			im, err := load(name)
			if err != nil {
				return err
			}
			top := terrainWallTop
			if cell.WallVariant == terrainWallLowVariant {
				top = terrainWallLowTop
			}
			if cell.Wall == terrainWallSpecialResource {
				top -= terrainWallSpecialLift
			}
			s.Tiles = append(s.Tiles, TerrainTile{X: x, Y: y + top, Depth: int(int32(cell.Depth)), Sorted: true, Image: im})
		}
	}
	return nil
}

func (t *TerrainTile) Draw(dst *surface.Surface, x, y int) { dst.Draw(t.X-x, t.Y-y, t.Image, true) }
