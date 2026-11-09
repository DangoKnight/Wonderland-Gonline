package housing

import (
	"image"
	"math"
)

// FUN_003cc158 initializes the native 60x30 isometric housing grid from
// 0x5c0 by 0x3c0. FUN_003c9ab4 projects its cells, including stacked height.
const (
	CellWidth   = 60
	CellHeight  = 30
	StackHeight = 30
	SceneWidth  = 1472
	SceneHeight = 960
	// FUN_0041b9b0 applies +0x45/-0xb2 after initializing the grid.
	projectionX = 750 + 69
	projectionY = -375 - 178
)

const (
	DefaultRoom                  = 1
	SmallRoomBackground   uint16 = 10005
	MediumRoomBackground  uint16 = 10004
	DefaultRoomBackground uint16 = 10000
)

func Background(kind byte) uint16 {
	if kind == 1 {
		return SmallRoomBackground
	}
	if kind == 2 {
		return MediumRoomBackground
	}
	return DefaultRoomBackground
}

// Room bounds are FUN_003e5ccc's authored floor rectangles.
func Room(kind byte) image.Rectangle {
	y := 38
	if kind == 2 {
		y = 32
	} else if kind == 3 {
		y = 27
	}
	return image.Rect(29, y, 45, 51)
}

func Pixel(x, y, z int) image.Point {
	return image.Pt(projectionX+(x-y)*CellWidth/2, projectionY+(x+y-2)*CellHeight/2-(z-1)*StackHeight)
}

func Cell(x, y int) image.Point {
	a := float64(x-projectionX) / (CellWidth / 2)
	b := float64(y-projectionY)/(CellHeight/2) + 2
	return image.Pt(int(math.Floor((a+b)/2+0.5)), int(math.Floor((b-a)/2+0.5)))
}

// All three native room images share this doorway. The compatibility anchor
// follows the visible landing in the templates and the Legacy screenshot.
func Exit(_ byte) image.Point { return image.Pt(388, 738) }

func Doorway() image.Rectangle { return image.Rect(355, 715, 430, 775) }
