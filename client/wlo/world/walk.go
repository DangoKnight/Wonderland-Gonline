package world

import (
	"image"
	"math"
	"time"
)

// Walking. A click on the ground (FUN_0043bc70) plans a path on the scene's
// 20-pixel walk grid (FUN_0041a9b8): within 41 cells of the player, a
// straight line when its cells are free, otherwise a search of the 82 × 82
// cells around the player, traced back into waypoints (cell × 20 + 2,
// cell × 20 + 15). A blocked target backs off toward the player first,
// along the line between them (FUN_0041a348). The player then walks the
// waypoints at 0.16 pixels per millisecond (+0x1ee0, FUN_004126b8), and
// each leg is announced with 6/1 (FUN_0041897c → the send case at
// 0x2c33cc). The original's search order and its other maps' pathfinder
// (FUN_003cbb2c) are not ported.
const (
	cellSize              = 0x14
	waypointX             = 2
	waypointY             = 0xf
	walkReach             = 0x29 // cells from the player
	terrainWater          = 2
	terrainWaterAlternate = 8
	shoreScanMinimum      = -2
	shoreScanRadius       = 3
	walkBlocked           = 0x13 + 4
	walkSpeed             = 0.16 // pixels per millisecond
	walkingAction         = 0    // walking actions are 0..7, standing 8..15
	ratioFlat             = 0.25
	ratioSteep            = 2.0
	ratioEpsilon          = 0.001
	directionsCount       = 8
)

// Facing directions (FUN_0041218c): 0 up, then anticlockwise.
const (
	FaceUp = iota
	FaceUpLeft
	FaceLeft
	FaceDownLeft
	FaceDown
	FaceDownRight
	FaceRight
	FaceUpRight
)

// Facing is FUN_0041218c: the direction from (x, y) to (tx, ty), keeping
// current for no movement. Diagonals cover slopes between 0.25 and 2.
func Facing(x, y, tx, ty, current int) int {
	dx, dy := tx-x, ty-y
	f := math.Abs(float64(dy)) / (ratioEpsilon + math.Abs(float64(dx)))
	diagonal := f > ratioFlat && f < ratioSteep
	switch {
	case dx < 0 && dy > 0 && diagonal:
		return FaceDownLeft
	case dx < 0 && dy < 0 && diagonal:
		return FaceUpLeft
	case dx > 0 && dy > 0 && diagonal:
		return FaceDownRight
	case dx > 0 && dy < 0 && diagonal:
		return FaceUpRight
	case dx > 0 && f <= ratioFlat:
		return FaceRight
	case dx < 0 && f <= ratioFlat:
		return FaceLeft
	case dy > 0:
		return FaceDown
	case dy < 0:
		return FaceUp
	}
	return current
}

// walkState is the player's walk: the waypoints, the current one, and the
// exact position between frames.
type walkState struct {
	path   []image.Point
	leg    int
	fx, fy float64
	at     time.Time
	facing int
}

// Walkable reports whether a scene point's grid cell is walkable.
func (s *Scene) Walkable(x, y int) bool { c := cellOf(x, y); return s.free(c.X, c.Y) }

// free reports a walkable grid cell.
func (s *Scene) free(cx, cy int) bool {
	v, ok := s.Ground.Cell(cx, cy)
	if !ok {
		return false
	}
	if s.WaterTravel {
		return v == terrainWater || v == terrainWaterAlternate
	}
	return v&walkBlocked == 0 && v != terrainWaterAlternate
}

func cellOf(x, y int) image.Point { return image.Pt(x/cellSize, y/cellSize) }

func waypoint(c image.Point) image.Point {
	return image.Pt(c.X*cellSize+waypointX, c.Y*cellSize+waypointY)
}

// lineFree reports whether the cells under the segment are walkable.
func (s *Scene) lineFree(a, b image.Point) bool {
	n := max(abs(b.X-a.X), abs(b.Y-a.Y))
	for i := 0; i <= n; i++ {
		x := a.X + (b.X-a.X)*i/max(n, 1)
		y := a.Y + (b.Y-a.Y)*i/max(n, 1)
		if !s.free(x, y) {
			return false
		}
	}
	return true
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Plan is FUN_0041a9b8: the waypoints from the player's cell to the
// clicked point, or nil when it cannot be reached.
func (s *Scene) Plan(fromX, fromY, toX, toY int) []image.Point {
	start, goal := cellOf(fromX, fromY), cellOf(toX, toY)
	if abs(goal.X-start.X) >= walkReach || abs(goal.Y-start.Y) >= walkReach {
		return nil
	}
	// A blocked target backs off toward the player along the straight
	// line between them (FUN_0041a348): the first walkable cell from the
	// target's end, so a click past the walkable ground lands on its edge.
	if !s.free(goal.X, goal.Y) {
		goal = s.backOff(start, goal)
	}
	if goal == start {
		return nil
	}
	if s.lineFree(start, goal) {
		return []image.Point{waypoint(goal)}
	}
	cells := s.search(start, goal)
	if cells == nil {
		return nil
	}
	// Keep the turning points.
	var out []image.Point
	for i := 1; i < len(cells); i++ {
		if i == len(cells)-1 || cells[i].Sub(cells[i-1]) != cells[i+1].Sub(cells[i]) {
			out = append(out, waypoint(cells[i]))
		}
	}
	return out
}

// backOff is FUN_0041a348: the cells on the line from goal to start,
// stepping along the longer axis with the other rounded, the first free one
// (start when none is).
func (s *Scene) backOff(start, goal image.Point) image.Point {
	dx, dy := start.X-goal.X, start.Y-goal.Y
	n := max(abs(dx), abs(dy))
	for i := 0; i <= n; i++ {
		c := image.Pt(goal.X+roundDiv(dx*i, n), goal.Y+roundDiv(dy*i, n))
		if s.free(c.X, c.Y) {
			return c
		}
	}
	return start
}

// roundDiv is a / b rounded to the nearest integer (b > 0).
func roundDiv(a, b int) int {
	if a < 0 {
		return -((-a + b/2) / b)
	}
	return (a + b/2) / b
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// search is a breadth-first search over the cells within reach of start,
// eight-way without cutting blocked corners; it returns the cells from
// start to goal.
func (s *Scene) search(start, goal image.Point) []image.Point {
	from := map[image.Point]image.Point{start: start}
	queue := []image.Point{start}
	steps := []image.Point{{0, -1}, {-1, 0}, {1, 0}, {0, 1}, {-1, -1}, {1, -1}, {-1, 1}, {1, 1}}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == goal {
			var path []image.Point
			for p := goal; p != start; p = from[p] {
				path = append(path, p)
			}
			path = append(path, start)
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path
		}
		for _, d := range steps {
			n := c.Add(d)
			if _, seen := from[n]; seen || abs(n.X-start.X) >= walkReach || abs(n.Y-start.Y) >= walkReach || !s.free(n.X, n.Y) {
				continue
			}
			if d.X != 0 && d.Y != 0 && (!s.free(c.X+d.X, c.Y) || !s.free(c.X, c.Y+d.Y)) {
				continue
			}
			from[n] = c
			queue = append(queue, n)
		}
	}
	return nil
}

// Walker moves one character along a planned path: the player, or a peer
// following the server's 6/1 (both use FUN_004126b8's speed and facing).
type Walker struct {
	walk  *walkState
	OnLeg func(facing, x, y int)
}

// Start begins a walk of p along path.
func (k *Walker) Start(p *Player, path []image.Point, now time.Time) {
	k.walk = &walkState{path: path, fx: float64(p.X), fy: float64(p.Y), at: now, facing: int(p.Direction) % directionsCount}
	k.startLeg(p)
}

// startLeg turns toward the current waypoint, walks, and announces it.
func (k *Walker) startLeg(p *Player) {
	wp := k.walk.path[k.walk.leg]
	k.walk.facing = Facing(p.X, p.Y, wp.X, wp.Y, k.walk.facing)
	p.Direction = int32(walkingAction + k.walk.facing)
	if k.OnLeg != nil {
		k.OnLeg(k.walk.facing, wp.X, wp.Y)
	}
}

// Walking reports whether the character is moving.
func (k *Walker) Walking() bool { return k.walk != nil }

// destination is the walk's last waypoint.
func (k *Walker) destination() (image.Point, bool) {
	if k.walk == nil || len(k.walk.path) == 0 {
		return image.Point{}, false
	}
	return k.walk.path[len(k.walk.path)-1], true
}

// Stop ends a walk where the character stands.
func (k *Walker) Stop(p *Player) {
	if k.walk != nil {
		p.Direction = int32(standingAction + k.walk.facing)
		k.walk = nil
	}
}

// Step is FUN_004126b8: it moves 0.16 pixels per millisecond toward the
// waypoint, takes the next one on arrival, and stands facing the last
// direction at the end.
func (k *Walker) Step(p *Player, now time.Time) {
	ws := k.walk
	if ws == nil {
		return
	}
	budget := walkSpeed * float64(now.Sub(ws.at).Milliseconds())
	if budget <= 0 {
		return
	}
	ws.at = now
	for budget > 0 {
		wp := ws.path[ws.leg]
		dx, dy := float64(wp.X)-ws.fx, float64(wp.Y)-ws.fy
		dist := math.Hypot(dx, dy)
		if dist > budget {
			ws.fx += dx / dist * budget
			ws.fy += dy / dist * budget
			break
		}
		ws.fx, ws.fy = float64(wp.X), float64(wp.Y)
		budget -= dist
		p.X, p.Y = wp.X, wp.Y
		ws.leg++
		if ws.leg == len(ws.path) {
			k.Stop(p)
			return
		}
		k.startLeg(p)
	}
	p.X, p.Y = int(math.Round(ws.fx)), int(math.Round(ws.fy))
}

// WalkTo starts the player walking toward a point on the scene (scene
// coordinates). It reports whether a path was found.
func (w *World) WalkTo(x, y int, now time.Time) bool {
	path := w.Scene.Plan(w.Player.X, w.Player.Y, x, y)
	if path == nil {
		return false
	}
	w.walker.OnLeg = w.OnLeg
	w.walker.Start(&w.Player, path, now)
	return true
}

// StopWalk stops the player where it stands.
func (w *World) StopWalk() { w.walker.Stop(&w.Player) }

// Walking reports whether the player is moving.
func (w *World) Walking() bool { return w.walker.Walking() }

// Step moves the player and the peers.
func (w *World) Step(now time.Time) {
	w.walker.Step(&w.Player, now)
	for _, p := range w.Peers {
		p.Walker.Step(&p.Player, now)
	}
}

// Water follows the native shore transition checks in FUN_0041897c.
func (s *Scene) Water(x, y int) bool {
	if x < 0 || y < 0 {
		return false
	}
	v, ok := s.Ground.Cell(x/cellSize, y/cellSize)
	return ok && (v == terrainWater || v == terrainWaterAlternate)
}

// Land reports the clear terrain used by native boarding/landing checks.
func (s *Scene) Land(x, y int) bool {
	if x < 0 || y < 0 {
		return false
	}
	v, ok := s.Ground.Cell(x/cellSize, y/cellSize)
	return ok && v == 0
}

// ShorePoint follows FUN_0041897c's -2..+3 cell scan. Require a clear
// crossing so a nearby water cell behind a wall cannot enable boarding.
func (s *Scene) ShorePoint(x, y int, water bool) (image.Point, bool) {
	origin := cellOf(x, y)
	for dx := shoreScanMinimum; dx <= shoreScanRadius; dx++ {
		for dy := shoreScanMinimum; dy <= shoreScanRadius; dy++ {
			cell := origin.Add(image.Pt(dx, dy))
			point := image.Pt(cell.X*cellSize, cell.Y*cellSize)
			if (water && !s.Water(point.X, point.Y)) || (!water && !s.Land(point.X, point.Y)) {
				continue
			}
			clear := true
			count := max(abs(dx), abs(dy))
			for i := 0; i <= count; i++ {
				step := image.Pt((origin.X+dx*i/max(count, 1))*cellSize, (origin.Y+dy*i/max(count, 1))*cellSize)
				if !s.Land(step.X, step.Y) && !s.Water(step.X, step.Y) {
					clear = false
					break
				}
			}
			if clear {
				return point, true
			}
		}
	}
	return image.Point{}, false
}
func (s *Scene) WaterEdge(x, y int) bool {
	if !s.Land(x, y) {
		return false
	}
	_, ok := s.ShorePoint(x, y, true)
	return ok
}
func (s *Scene) PlanWater(fromX, fromY, toX, toY int) []image.Point {
	water := *s
	water.WaterTravel = true
	return water.Plan(fromX, fromY, toX, toY)
}

// Relocate resets interpolated walking before a native shore transition.
func (w *World) Relocate(point image.Point) {
	w.StopWalk()
	w.Player.X, w.Player.Y = point.X, point.Y
}

// SameCell compares points on the native walk grid.
func (s *Scene) SameCell(a, b image.Point) bool { return cellOf(a.X, a.Y) == cellOf(b.X, b.Y) }
