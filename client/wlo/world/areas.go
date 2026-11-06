package world

import (
	"encoding/binary"
	"image"
	"time"

	native "wonderland-gonline/internal/assets"
)

// Map areas (EVE category 1, the scene loader FUN_003090f4 → +0x338). An
// area is a rectangle of 20-pixel cells: from cell (x1, y1), counted from
// 1, to (x2, y2). Walking into one that has events asks the server to run
// it (20/8, FUN_0030410c); doors among them glow (DoorLight,
// SmallDoorLight, added by FUN_003030ec and drawn by FUN_00403b8c).
const (
	// Record tail after the events and conditions: a byte, x2 and y2,
	// the door kind (+0x48), and the light's offset from the area's
	// corner (+0x4c, +0x50).
	areaTailBytes = 18
	areaX2Offset  = 1
	areaY2Offset  = 5
	areaDoor      = 9
	areaLightX    = 10
	areaLightY    = 14
	// Door kinds (+0x48), mapped to the light (+0x27): 1 none, 2 small,
	// 3 large.
	doorKindSmall = 2
	doorKindLarge = 3
)

// Legacy reference metadata from pic\images.BMg (FUN_003030ec's arguments).
// Runtime gateway and exit rendering no longer loads these sprite sheets.
const (
	DoorLightPicture      = "DoorLight"
	SmallDoorLightPicture = "SmallDoorLight"
	doorLightFrames       = 13
	doorLightInterval     = 300 * time.Millisecond
	doorLightLevel        = 0x18
)

// LightKind is an area's door light.
type LightKind byte

const (
	NoLight LightKind = iota
	LargeLight
	SmallLight
)

// Area is one map area.
type Area struct {
	ID     uint16
	Rect   image.Rectangle // in map pixels
	Events int             // +6: areas without events do not trigger
	Light  LightKind
	// LightAt is the light's point: its picture is centred on it and its
	// bottom sits a quarter of a frame below it.
	LightAt image.Point
	// Sign is the record's minimap icon (+0x3c, minimap.go).
	Sign byte
}

// MapAreas reads the map's areas from its event record (FUN_003090f4).
func MapAreas(rec native.Map) []Area {
	var out []Area
	for _, e := range rec.Entries {
		if len(e.Tail) < areaTailBytes {
			continue
		}
		t := e.Tail
		x1, y1 := int(int32(e.X)), int(int32(e.Y))
		x2 := int(int32(binary.LittleEndian.Uint32(t[areaX2Offset:])))
		y2 := int(int32(binary.LittleEndian.Uint32(t[areaY2Offset:])))
		x, y := (x1-1)*cellSize, (y1-1)*cellSize
		a := Area{ID: e.ClickID, Events: len(e.Events), Sign: t[areaSign],
			Rect: image.Rect(x, y, x+(abs(x2-x1)+1)*cellSize, y+(abs(y2-y1)+1)*cellSize)}
		switch t[areaDoor] {
		case doorKindSmall:
			a.Light = SmallLight
		case doorKindLarge:
			a.Light = LargeLight
		}
		// The point is kept in 16 bits (+0x28, +0x2a).
		a.LightAt = image.Pt(int(uint16(int(binary.LittleEndian.Uint16(t[areaLightX:]))+x)),
			int(uint16(int(binary.LittleEndian.Uint16(t[areaLightY:]))+y)))
		out = append(out, a)
	}
	return out
}

// drawLights preserves EVE positions, drawing both gateway and small exit
// indicators procedurally. Each animation is generated once per world frame.
func (w *World) drawLights(cx, cy int, now time.Time) {
	var pointerDrawn, gatewayDrawn bool
	for _, a := range w.Areas {
		if a.Light == NoLight {
			continue
		}
		if a.LightAt.X == 0 && a.LightAt.Y == 0 {
			continue
		}
		sx, sy := a.LightAt.X-cx, a.LightAt.Y-cy
		if sx < 0 || sx > w.Env.Screen.W || sy < 0 || sy > w.Env.Screen.H {
			continue
		}
		if a.Light == SmallLight {
			if w.exitIndicator == nil {
				w.exitIndicator = newExitIndicator()
			}
			if !pointerDrawn {
				w.exitIndicator.render(now.Sub(w.lightsFrom))
				pointerDrawn = true
			}
			w.exitIndicator.draw(w.Env.Screen, sx, sy)
			continue
		}
		if w.gatewayLight == nil {
			w.gatewayLight = newGatewayLight()
		}
		if !gatewayDrawn {
			w.gatewayLight.render(now.Sub(w.lightsFrom))
			gatewayDrawn = true
		}
		w.gatewayLight.draw(w.Env.Screen, sx, sy)
	}
}
