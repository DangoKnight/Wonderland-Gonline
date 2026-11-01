package world

import (
	"encoding/binary"
	"image"
	"math"
	"slices"

	"wonderland-gonline/client/wlo/login"
	native "wonderland-gonline/internal/assets"
)

// Minimap data. The map frame's minimap (FUN_0030e1b0, painted by
// FUN_0030e978) shows on scenes whose SceneData byte +0x1d is 1. It marks
// map NPCs, doors and mines that carry a sign with the skin picture
// Icon_Sign_<n> (FUN_0030f27c), and the player with a small face
// (FUN_0030eb60). A sign starts as the record's +0x3c and AC22:12
// (FUN_00459d88) replaces it: the server marks NPCs with unfinished quest
// branches with 7, the "?".
const (
	// sceneMiniMap is the SceneData byte +0x1d value that shows the
	// minimap; the export calls the byte by its file offset, 23.
	sceneMiniMap = 1
	// SignPicture prefixes a sign's number (the string at 0x30f654).
	SignPicture = "Icon_Sign_"
	// areaSign is the area and mine tails' first byte (+0x3c).
	areaSign = 0
	// Mine tails: the sign, then x2 and y2 (FUN_003090f4 → +0x5b8c).
	mineTailBytes = 9
	mineX2Offset  = 1
	mineY2Offset  = 5
	// miniMapShift moves every mark three pixels left (FUN_0030f008).
	miniMapShift = 3
	// AC22:12: kind, ID u16, sign; kinds 1 NPC, 2 door, 3 mine.
	signBytes = 4
	signNPC   = 1
	signDoor  = 2
	signMine  = 3
)

// miniMapBands are FUN_0030f008's vertical corrections: below each map Y
// limit a mark rises k - round(y / height × k) pixels (constants at
// 0x30f264); beyond the last limit the last k applies.
var miniMapBands = []struct{ limit, k int }{
	{1000, 6}, {2000, 8}, {3000, 10}, {4000, 16}, {5000, 32}, {math.MaxInt, 36},
}

// Sign is one minimap mark: its map point and sign number.
type Sign struct {
	At   image.Point
	Sign byte
}

// SceneMiniMaps lists the scenes whose minimap is shown.
func SceneMiniMaps(a login.Assets) (map[uint16]bool, error) {
	var doc struct {
		Records []struct {
			Fields struct {
				ID      uint16 `json:"id"`
				MiniMap byte   `json:"unknown_u8_offset_23"`
			} `json:"fields"`
		} `json:"records"`
	}
	if err := readJSON(a.DataPath(sceneExport), &doc); err != nil {
		return nil, err
	}
	out := map[uint16]bool{}
	for _, r := range doc.Records {
		if r.Fields.MiniMap == sceneMiniMap {
			out[r.Fields.ID] = true
		}
	}
	return out, nil
}

// Mine is a map mine (EVE category 2, +0x5b8c) as the minimap knows it:
// its ID, its rectangle's centre and its sign.
type Mine struct {
	ID   uint16
	At   image.Point
	Sign byte
}

// MapMines reads the map's mines (FUN_003090f4).
func MapMines(rec native.Map) []Mine {
	var out []Mine
	for _, e := range rec.Mining {
		t := e.Tail
		if len(t) < mineTailBytes {
			continue
		}
		x1, y1 := int(int32(e.X)), int(int32(e.Y))
		x2 := int(int32(binary.LittleEndian.Uint32(t[mineX2Offset:])))
		y2 := int(int32(binary.LittleEndian.Uint32(t[mineY2Offset:])))
		w, h := (abs(x2-x1)+1)*cellSize, (abs(y2-y1)+1)*cellSize
		out = append(out, Mine{e.ClickID, image.Pt((x1-1)*cellSize+w/2, (y1-1)*cellSize+h/2), t[areaSign]})
	}
	return out
}

// MiniMapSigns is FUN_0030f27c's list, in its order: shown map NPCs with
// a sign at their feet, doors with a sign at their light's point, then
// mines.
func (w *World) MiniMapSigns() []Sign {
	var out []Sign
	ids := make([]uint16, 0, len(w.NPCs))
	for id, n := range w.NPCs {
		if n.Shown && n.Sign != 0 {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids) // the role table's order, by click ID
	for _, id := range ids {
		n := w.NPCs[id]
		out = append(out, Sign{image.Pt(n.X, n.Y), n.Sign})
	}
	for _, a := range w.Areas {
		if a.Sign != 0 {
			out = append(out, Sign{a.LightAt, a.Sign})
		}
	}
	for _, m := range w.Mines {
		if m.Sign != 0 {
			out = append(out, Sign{m.At, m.Sign})
		}
	}
	return out
}

// ApplySign is AC22:12 (FUN_00459d88): p is the packet after its command
// and subcommand bytes. Unknown targets are ignored.
func (w *World) ApplySign(p []byte) bool {
	if len(p) < signBytes {
		return false
	}
	id, sign := binary.LittleEndian.Uint16(p[1:]), p[3]
	switch p[0] {
	case signNPC:
		if n := w.NPCs[id]; n != nil {
			n.Sign = sign
		}
	case signDoor:
		for i := range w.Areas {
			if w.Areas[i].ID == id {
				w.Areas[i].Sign = sign
			}
		}
	case signMine:
		for i := range w.Mines {
			if w.Mines[i].ID == id {
				w.Mines[i].Sign = sign
			}
		}
	}
	return true
}

// MiniMapPoint is FUN_0030f008: a map point on a minimap at (left, top)
// that shows the map at 1/scale; height is the map's height.
func MiniMapPoint(x, y, scale, left, top, height int) (int, int) {
	k := miniMapBands[len(miniMapBands)-1].k
	for _, b := range miniMapBands {
		if y <= b.limit {
			k = b.k
			break
		}
	}
	rise := k - int(math.RoundToEven(float64(y)/float64(height)*float64(k)))
	return x/scale + left - miniMapShift, y/scale + top - rise
}
