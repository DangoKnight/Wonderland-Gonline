package world

import "wonderland-go/internal/game"

import "math"

type Destination struct{ Map, X, Y uint16 }

// Carnie (11094) exits to its saved entry point; without one the C# fallback is the starter beach.
var carnieFallback = Destination{11016, 1181, 243}

// Portal resolves a regular warp from EVE data. Reference: GameMap.LookupPortal and the
// map 11094 branch of GameMap.Teleport. Tent exits and the database portal table (empty
// by default) are not ported.
func (w *World) Portal(mapID, portal, x, y uint16) (Destination, bool) {
	if mapID == game.MapID11094 {
		return carnieFallback, true
	}
	if m, ok := w.maps[mapID]; ok && len(m.data.Warps) > 0 {
		warps := m.data.Warps
		// Geometric reverse match: the destination whose return warp is nearest the player.
		if x > 0 && y > 0 {
			best, bestDist := -1, math.MaxFloat64
			for i, warp := range warps {
				dst, ok := w.maps[warp.MapID]
				if !ok {
					continue
				}
				for _, back := range dst.data.Warps {
					if back.MapID != mapID {
						continue
					}
					if d := math.Hypot(float64(int64(x)-int64(back.X)), float64(int64(y)-int64(back.Y))); d < bestDist {
						best, bestDist = i, d
					}
				}
			}
			if best >= 0 && bestDist < 600 && warps[best].MapID > 0 {
				return destination(warps[best].MapID, warps[best].X, warps[best].Y), true
			}
		}
		for _, warp := range warps {
			if warp.ClickID == portal && warp.MapID > 0 {
				return destination(warp.MapID, warp.X, warp.Y), true
			}
		}
		if portal >= 1 && int(portal) <= len(warps) && warps[portal-1].MapID > 0 {
			warp := warps[portal-1]
			return destination(warp.MapID, warp.X, warp.Y), true
		}
		if gray := grayDecode(portal); gray != portal {
			for _, warp := range warps {
				if warp.ClickID == gray && warp.MapID > 0 {
					return destination(warp.MapID, warp.X, warp.Y), true
				}
			}
		}
		valid := -1
		for i, warp := range warps {
			if warp.MapID > 0 {
				if valid >= 0 {
					valid = -2
					break
				}
				valid = i
			}
		}
		if valid >= 0 {
			return destination(warps[valid].MapID, warps[valid].X, warps[valid].Y), true
		}
	}
	// Emergency recovery from invalid or test maps.
	if mapID < 1000 {
		return Destination{12000, 892, 734}, true
	}
	return Destination{}, false
}

// WarpEntry is the map's warp record with this click ID (a scene transition target).
func (w *World) WarpEntry(mapID, click uint16) (Destination, bool) {
	if m, ok := w.maps[mapID]; ok {
		for _, warp := range m.data.Warps {
			if warp.ClickID == click && warp.MapID > 0 {
				return destination(warp.MapID, warp.X, warp.Y), true
			}
		}
	}
	return Destination{}, false
}

func destination(m uint16, x, y uint32) Destination { return Destination{m, uint16(x), uint16(y)} }

func grayDecode(n uint16) uint16 {
	for mask := n >> 1; mask > 0; mask >>= 1 {
		n ^= mask
	}
	return n
}
