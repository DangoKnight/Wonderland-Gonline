package role

import (
	"image"
	"wonderland-gonline/client/wlo/sprites"
	"wonderland-gonline/client/wlo/surface"
)

// Tent is TRe_Camp's static 009-family exterior. Its anchor is supplied by
// the sprite, not by the inventory icon's dimensions (FUN_001d171c).
type Tent struct {
	Lib    *Library
	Sprite uint16
}

const tentSpriteFamily = "009"

func (t *Tent) frame() (*sprite, *sprites.Frame) {
	if t == nil || t.Lib == nil {
		return nil, nil
	}
	a, key := t.Lib.resolve(tentSpriteFamily, int(t.Sprite))
	if a == nil {
		return nil, nil
	}
	s := a.sprite(key)
	if s == nil {
		return nil, nil
	}
	return s, s.frame(0, 0)
}
func (t *Tent) Draw(dst *surface.Surface, x, y int) {
	s, f := t.frame()
	if f != nil {
		s.draw(dst, f, x, y)
	}
}
func (t *Tent) Hit(x, y, px, py int) bool {
	s, f := t.frame()
	if f == nil {
		return false
	}
	return (spritePick{s: s, f: f, at: image.Pt(x+f.OffsetX, y+f.OffsetY), scale: 1}).hit(image.Pt(px, py))
}
