package role

import (
	"image"
	"wonderland-gonline/client/wlo/sprites"
)

// FUN_002fe8e8 reads the source pixel at the pointer and ignores transparent
// pixels before registering the actor with FUN_004106b0. These references are
// to immutable source frames, never a readback of the GPU framebuffer.
type spritePick struct {
	s     *sprite
	f     *sprites.Frame
	at    image.Point
	scale int
}

func (p spritePick) hit(at image.Point) bool {
	at = at.Sub(p.at)
	if at.X < 0 || at.Y < 0 || at.X >= p.f.Width*p.scale || at.Y >= p.f.Height*p.scale {
		return false
	}
	im, err := p.f.Image(p.s.m)
	if err != nil || im == nil {
		return false
	}
	return im.NRGBAAt(im.Bounds().Min.X+at.X/p.scale, im.Bounds().Min.Y+at.Y/p.scale).A != 0
}

func (h *Human) SetLit(on bool) { h.Lit = on }

// HitBody uses the same layered frames and placement as the last DrawBody.
func (h *Human) HitBody(x, y int, direction int32, px, py int) bool {
	at := image.Pt(px-x, py-y)
	for _, p := range h.picks[:h.pickCount] {
		if p.hit(at) {
			return true
		}
	}
	if h.petMount != nil {
		return h.petMount.HitSprite(x, y+petSpriteDrop(h.petHeightScale, h.petHeightPreset), int(direction), px, py)
	}
	return false
}

func (n *NPC) HitSprite(x, y, action, px, py int) bool {
	arc, key := n.Lib.lookup(npcFamily, n.sprite)
	if arc == nil {
		return false
	}
	s := arc.sprite(key)
	if s == nil {
		return false
	}
	count := s.frameCount(action)
	if count == 0 {
		return false
	}
	f := s.frame(action, n.shown(count))
	if f == nil {
		return false
	}
	scale := n.rasterScale()
	return (spritePick{s: s, f: f, at: image.Pt(x+f.OffsetX*scale, y+f.OffsetY*scale), scale: scale}).hit(image.Pt(px, py))
}

// Keep selection geometry owned by the role when a preview copies a Human.
func (h *Human) recordPick(p spritePick) {
	if h.pickCount < len(h.picks) {
		h.picks[h.pickCount] = p
		h.pickCount++
	}
}
