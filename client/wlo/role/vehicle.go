package role

import (
	"image"

	"wonderland-gonline/client/wlo/sprites"
	"wonderland-gonline/client/wlo/surface"
)

// Vehicle is the unoccupied native 006-family sprite, using the same origin
// as a mounted vehicle. It remains independent of the player's body layers.
type Vehicle struct {
	Lib    *Library
	Sprite uint16
	Water  bool
}

const placedVehicleAction = 12

func (v *Vehicle) frame() (*sprite, *sprites.Frame) {
	if v == nil || v.Lib == nil {
		return nil, nil
	}
	arc, key := v.Lib.lookup(vehicleSpriteFamily, int(v.Sprite))
	if arc == nil {
		return nil, nil
	}
	s := arc.sprite(key)
	if s == nil {
		return nil, nil
	}
	return s, s.frame(placedVehicleAction, 0)
}

func (v *Vehicle) origin(x, y int) image.Point {
	if v.Water && seatedVehicleGroundCorrection(v.Sprite) {
		y += seatedVehicleGroundOffset
	}
	return image.Pt(x, y)
}

func (v *Vehicle) Draw(dst *surface.Surface, x, y int) {
	s, f := v.frame()
	if f != nil {
		at := v.origin(x, y)
		s.draw(dst, f, at.X, at.Y)
	}
}

func (v *Vehicle) Hit(x, y, px, py int) bool {
	s, f := v.frame()
	if f == nil {
		return false
	}
	at := v.origin(x, y).Add(image.Pt(f.OffsetX, f.OffsetY))
	return (spritePick{s: s, f: f, at: at, scale: 1}).hit(image.Pt(px, py))
}
