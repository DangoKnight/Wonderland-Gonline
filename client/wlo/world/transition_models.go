package world

import "image"

// transitionModelHits uses the same opaque sprite pixels as field picking.
// It runs after actor drawing, so layered humans refer to this frame's artwork.
// No framebuffer readback or rectangular fade over transparent sprite gaps.
func (w *World) transitionModelHits(cx, cy int) []func(int, int) bool {
	var hits []func(int, int) bool
	addNPC := func(p NPCPainter, x, y, action int) {
		if p == nil {
			return
		}
		bounds := p.Bounds(x, y, action)
		picker, ok := p.(interface {
			HitSprite(int, int, int, int, int) bool
		})
		if !ok {
			return // No opaque-pixel information: retain the ordinary effect.
		}
		hits = append(hits, func(px, py int) bool {
			return image.Pt(px, py).In(bounds) && picker.HitSprite(x, y, action, px, py)
		})
	}
	addPlayer := func(p Player, body any) {
		if w.fighting(p.ID) {
			return
		}
		if shape, ok := w.Shapeshifts[p.ID]; ok && shape.Painter != nil {
			addNPC(shape.Painter, p.X-cx, p.Y-cy+shape.Info.SpriteDrop(), int(p.Direction))
			return
		}
		if picker, ok := body.(interface {
			HitBody(int, int, int32, int, int) bool
		}); ok {
			hits = append(hits, func(x, y int) bool {
				return picker.HitBody(p.X-cx, p.Y-cy, p.Direction, x, y)
			})
		}
	}
	if w.Body != nil && !w.HidePlayer {
		addPlayer(w.Player, w.Body)
	}
	for _, peer := range w.Peers {
		if peer.Role != nil {
			addPlayer(peer.Player, peer.Role)
		}
	}
	for _, n := range w.NPCs {
		if n.Visible() {
			addNPC(n.Painter, n.X-cx, n.Y-cy+n.Info.SpriteDrop(), n.Action)
		}
	}
	for owner, pet := range w.Companions {
		if x, y, action, ok := w.companionPosition(owner); ok {
			addNPC(pet.Painter, x-cx, y-cy+pet.Info.SpriteDrop(), action)
		}
	}
	return hits
}
