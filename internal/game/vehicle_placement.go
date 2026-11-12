package game

// RaftPlacementCellPixels is the collision-cell spacing in Ground.MMG.
const RaftPlacementCellPixels = 20

// RaftPlacement finds adjacent navigable water, never the player's land cell.
// Terrain predicates must reject out-of-map and blocked coordinates. Requiring
// a cardinal water exit excludes isolated puddles; diagonal shore crossings
// must also have clear sides so placement cannot pass through a wall corner.
func RaftPlacement(x, y int, land, water func(int, int) bool) (int, int, bool) {
	clear := func(x, y int) bool { return land(x, y) || water(x, y) }
	if x < 0 || y < 0 || !clear(x, y) {
		return 0, 0, false
	}
	cx, cy := x/RaftPlacementCellPixels, y/RaftPlacementCellPixels
	point := func(cx, cy int) (int, int) {
		return cx*RaftPlacementCellPixels + RaftPlacementCellPixels/2, cy*RaftPlacementCellPixels + RaftPlacementCellPixels/2
	}
	best, bx, by := int(^uint(0)>>1), 0, 0
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			if dx == 0 && dy == 0 {
				continue
			}
			px, py := point(cx+dx, cy+dy)
			if !water(px, py) {
				continue
			}
			if dx != 0 && dy != 0 {
				ax, ay := point(cx+dx, cy)
				bx, by := point(cx, cy+dy)
				if !clear(ax, ay) || !clear(bx, by) {
					continue
				}
			}
			exit := false
			for _, step := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				if water(px+step[0]*RaftPlacementCellPixels, py+step[1]*RaftPlacementCellPixels) {
					exit = true
					break
				}
			}
			if !exit {
				continue
			}
			if distance := (px-x)*(px-x) + (py-y)*(py-y); distance < best {
				best, bx, by = distance, px, py
			}
		}
	}
	return bx, by, best != int(^uint(0)>>1)
}
