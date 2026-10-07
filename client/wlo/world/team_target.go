package world

import "image"

const (
	// Compatibility bounds for RoleView implementations without sprite picking.
	teamTargetHalfWidth = 24
	teamTargetHeight    = 90
	teamTargetBelowFeet = 8
)

type teamTarget struct {
	owner     uint32
	companion bool
	depth     int
}

// FUN_004106b0 separates player and companion hits. Join Team resolves a
// following companion to its owner; the frontmost opaque sprite wins.
func (w *World) pickTeamTarget(x, y int) teamTarget {
	cx, cy := w.Camera()
	var hit teamTarget
	choose := func(owner uint32, companion bool, depth int) {
		if hit.owner == 0 || depth > hit.depth || depth == hit.depth && (companion && !hit.companion || companion == hit.companion && owner < hit.owner) {
			hit = teamTarget{owner: owner, companion: companion, depth: depth}
		}
	}
	for id, p := range w.Peers {
		inside := false
		if picker, ok := p.Role.(interface {
			HitBody(int, int, int32, int, int) bool
		}); ok {
			inside = picker.HitBody(p.X-cx, p.Y-cy, p.Direction, x, y)
		} else {
			inside = image.Pt(x, y).In(image.Rect(p.X-cx-teamTargetHalfWidth, p.Y-cy-teamTargetHeight, p.X-cx+teamTargetHalfWidth, p.Y-cy+teamTargetBelowFeet))
		}
		if inside {
			choose(id, false, p.Y)
		}
	}
	for owner, c := range w.Companions {
		if owner == w.Player.ID || w.Peers[owner] == nil || c.Painter == nil {
			continue
		}
		px, py, action, ok := w.companionPosition(owner)
		if !ok {
			continue
		}
		drawY := py - cy + c.Info.SpriteDrop()
		inside := false
		if picker, ok := c.Painter.(interface {
			HitSprite(int, int, int, int, int) bool
		}); ok {
			inside = picker.HitSprite(px-cx, drawY, action, x, y)
		} else {
			inside = image.Pt(x, y).In(c.Painter.Bounds(px-cx, drawY, action))
		}
		if inside {
			choose(owner, true, py)
		}
	}
	return hit
}
func (w *World) TeamTargetAt(x, y int) uint32 { return w.pickTeamTarget(x, y).owner }
