package game

import "strings"

// Tent placement rules, ported from the desktop server's Tent class
// (Game/PlayerRelated/Tent/Tent.cs) and the AC 62 / AC 64 handlers.

// tentWallpaperIDs lists the wall items the desktop server recognizes by ID.
var tentWallpaperIDs = map[uint16]bool{
	39064: true, 39128: true, 39130: true, 39136: true, 39138: true,
	39140: true, 39142: true, 39144: true, 39169: true, 39171: true,
	39174: true, 39176: true, 39181: true, 39193: true, 39195: true,
	39197: true, 39199: true, 39213: true, 39297: true, 39311: true,
	39312: true, 39326: true, 39327: true, 39336: true, 39361: true,
	39370: true, 39441: true, 39451: true, 39484: true, 39486: true,
	39507: true, 39524: true, 39562: true, 39595: true, 39690: true,
}

// tentFloorIDs lists the floor items the desktop server recognizes by ID.
var tentFloorIDs = map[uint16]bool{
	39062: true, 39067: true, 39068: true, 39131: true, 39177: true,
	39214: true, 39310: true, 39335: true, 39362: true, 39372: true,
	39462: true, 39506: true, 39561: true, 39584: true,
}

// IsTentWallpaper reports whether a tent item is applied to a wall instead of
// being placed as furniture.
func IsTentWallpaper(id uint16, name string) bool {
	if (id >= 39041 && id <= 39044) || tentWallpaperIDs[id] {
		return true
	}
	n := strings.ToLower(name)
	return strings.Contains(n, "wall") && !strings.Contains(n, "lamp") && !strings.Contains(n, "light") &&
		!strings.Contains(n, "furnace") && !strings.Contains(n, "painting") && !strings.Contains(n, "clock")
}

// IsTentFloor reports whether a tent item is applied to a floor instead of
// being placed as furniture.
func IsTentFloor(id uint16, name string) bool {
	if (id >= 39033 && id <= 39036) || (id >= 39189 && id <= 39191) || tentFloorIDs[id] {
		return true
	}
	n := strings.ToLower(name)
	return (strings.Contains(n, "floor") || strings.Contains(n, "tile")) && !strings.Contains(n, "lamp") &&
		!strings.Contains(n, "light") && !strings.Contains(n, "gift")
}

// TentFootprint returns the cells an item covers at a rotation. Quarter turns
// swap width and height; missing sizes count as one cell.
func TentFootprint(def ItemDefinition, rotation byte) (w, h int) {
	w, h = int(def.CellWidth), int(def.CellHeight)
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	if rotation == 1 || rotation == 3 {
		w, h = h, w
	}
	return w, h
}

// TentInFloorBounds checks the anchor cell against the tent floor. The desktop
// server validates only the anchor (aLogin.exe FUN_003e67c0), and the floor is
// taller for enlarged tents and tent types 2 and up.
func TentInFloorBounds(x, y int, tentType byte, enlarged bool) bool {
	if tentType == 0 {
		tentType = 1
	}
	minX, maxX, minY, maxY := 29, 44, 38, 50
	if enlarged || tentType >= 2 {
		minY = 32
		if tentType >= 3 {
			minY = 27
		}
	}
	return x >= minX && x <= maxX && y >= minY && y <= maxY
}

// TentRect is an occupied rectangle of cells on one floor.
type TentRect struct {
	X, Y, W, H int
	Floor      byte
}

// TentOverlaps reports whether two rectangles share a cell.
func TentOverlaps(a, b TentRect) bool {
	return a.Floor == b.Floor &&
		a.X < b.X+b.W && a.X+a.W > b.X &&
		a.Y < b.Y+b.H && a.Y+a.H > b.Y
}

// TentFreeSpot returns the anchor for a new piece. The desktop server tries
// (42,42) first; if that is taken it scans the floor on a two-cell grid. When
// no cell is free it still uses (42,42), which keeps its behavior.
func TentFreeSpot(occupied []TentRect, w, h int, tentType byte, enlarged bool) (x, y int) {
	free := func(px, py int) bool {
		if !TentInFloorBounds(px, py, tentType, enlarged) {
			return false
		}
		for _, o := range occupied {
			if TentOverlaps(o, TentRect{X: px, Y: py, W: w, H: h}) {
				return false
			}
		}
		return true
	}
	if free(42, 42) {
		return 42, 42
	}
	for ty := 38; ty <= 50; ty += 2 {
		for tx := 29; tx <= 44; tx += 2 {
			if free(tx, ty) {
				return tx, ty
			}
		}
	}
	return 42, 42
}
