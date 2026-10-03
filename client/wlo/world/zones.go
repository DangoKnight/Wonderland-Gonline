package world

// Sound zones. After its walk grid a Ground.MMG record lists 6-byte zones
// (the loader FUN_003f830c → +0xc6ab4, +0xc64b4): a centre cell (x, y), a
// sound number (wav####.wav) and a radius in cells, packed in the export's
// third word (low byte the sound, high byte the radius). Every 500 ms the
// frame (FUN_00498aec) finds the first zone whose square of cells holds the
// player (FUN_003f925c) and plays its sound around the centre.
type SoundZone struct {
	X, Y   int // the centre cell
	Sound  int
	Radius int // cells
}

// groundZone is an export record of the zone list.
type groundZone struct {
	X      uint16 `json:"unknown_u16_0"`
	Y      uint16 `json:"unknown_u16_1"`
	Packed uint16 `json:"unknown_u16_2"`
}

const (
	zoneSoundMask   = 0xff
	zoneRadiusShift = 8
)

func zonesOf(list []groundZone) []SoundZone {
	out := make([]SoundZone, 0, len(list))
	for _, z := range list {
		out = append(out, SoundZone{X: int(z.X), Y: int(z.Y), Sound: int(z.Packed & zoneSoundMask), Radius: int(z.Packed >> zoneRadiusShift)})
	}
	return out
}

// SoundZoneAt is FUN_003f925c: the first zone around the cell holding the
// map point (x, y), false for none.
func (s *Scene) SoundZoneAt(x, y int) (SoundZone, bool) {
	cx, cy := x/cellSize, y/cellSize
	for _, z := range s.Zones {
		if z.X-z.Radius <= cx && cx <= z.X+z.Radius && z.Y-z.Radius <= cy && cy <= z.Y+z.Radius {
			return z, true
		}
	}
	return SoundZone{}, false
}
