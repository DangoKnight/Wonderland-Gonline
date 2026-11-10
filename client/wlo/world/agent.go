package world

import "image"

// AgentCellSize is the collision grid's cell size in world pixels.
const AgentCellSize = cellSize

// AgentWalkable reports whether a collision cell is walkable.
func (s *Scene) AgentWalkable(cx, cy int) bool {
	if s == nil {
		return false
	}
	return s.free(cx, cy)
}

// AgentGrid returns a walkability grid centred on a world point. Each byte is
// 1 for walkable and 0 for blocked. The origin is the top-left world pixel of
// the first cell.
func (s *Scene) AgentGrid(centerX, centerY, radius int) (originX, originY, cols, rows int, cells []byte) {
	if s == nil {
		return 0, 0, 0, 0, nil
	}
	pc := cellOf(centerX, centerY)
	cols, rows = 2*radius+1, 2*radius+1
	originX, originY = (pc.X-radius)*cellSize, (pc.Y-radius)*cellSize
	cells = make([]byte, cols*rows)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			if s.free(pc.X-radius+x, pc.Y-radius+y) {
				cells[y*cols+x] = 1
			}
		}
	}
	return originX, originY, cols, rows, cells
}

// AgentMapGrid returns a coarse walkability overview of the whole scene.
// step is the number of collision cells per overview cell; a cell is marked
// walkable when any of its collision cells is walkable.
func (s *Scene) AgentMapGrid(step int) (cols, rows int, cells []byte) {
	if s == nil {
		return 0, 0, nil
	}
	if step < 1 {
		step = 1
	}
	cw, ch := s.Width/cellSize, s.Height/cellSize
	cols, rows = (cw+step-1)/step, (ch+step-1)/step
	cells = make([]byte, cols*rows)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			found := false
			for sy := 0; sy < step && !found; sy++ {
				for sx := 0; sx < step; sx++ {
					cx, cy := x*step+sx, y*step+sy
					if cx >= cw || cy >= ch {
						break
					}
					if s.free(cx, cy) {
						found = true
						break
					}
				}
			}
			if found {
				cells[y*cols+x] = 1
			}
		}
	}
	return cols, rows, cells
}

// AgentCompanion is one following companion/pet.
type AgentCompanion struct {
	Owner uint32
	ID    uint32
	Name  string
	X, Y  int
}

// AgentCompanions returns the map's following companions.
func (w *World) AgentCompanions() []AgentCompanion {
	if w == nil {
		return nil
	}
	out := make([]AgentCompanion, 0, len(w.Companions))
	for owner, c := range w.Companions {
		out = append(out, AgentCompanion{
			Owner: owner, ID: c.ID, Name: string(c.Name),
			X: c.position.X, Y: c.position.Y,
		})
	}
	return out
}

// AgentWater reports whether a world point is water.
func (s *Scene) AgentWater(x, y int) bool { return s.Water(x, y) }

// AgentLand reports whether a world point is clear land.
func (s *Scene) AgentLand(x, y int) bool { return s.Land(x, y) }

// AgentWaterEdge reports whether the point is land next to clear water.
func (s *Scene) AgentWaterEdge(x, y int) bool { return s.WaterEdge(x, y) }

// AgentDirectWaterEntry checks the native direct water-entry ray.
func (s *Scene) AgentDirectWaterEntry(fromX, fromY, toX, toY int) (image.Point, bool) {
	return s.DirectWaterEntry(fromX, fromY, toX, toY)
}

// AgentShorePoint finds a shore point near a world position.
func (s *Scene) AgentShorePoint(x, y int, water bool) (image.Point, bool) {
	return s.ShorePoint(x, y, water)
}

// AgentPlan returns the client's own short-range path waypoints.
func (s *Scene) AgentPlan(fromX, fromY, toX, toY int) []image.Point {
	if s == nil {
		return nil
	}
	return s.Plan(fromX, fromY, toX, toY)
}
