package world

import "wonderland-gonline/client/wlo/surface"

// PlayerShape is a cosmetic NPC sprite over the player's unchanged character.
// AC5:5 does not change equipment, attributes or the permanent body/head.
type PlayerShape struct {
	Template uint16
	Info     NPCTemplate
	Painter  NPCPainter
}

func (w *World) SetShapeshift(owner uint32, shape PlayerShape) {
	if shape.Template == 0 {
		delete(w.Shapeshifts, owner)
		return
	}
	if w.Shapeshifts == nil {
		w.Shapeshifts = map[uint32]PlayerShape{}
	}
	w.Shapeshifts[owner] = shape
}

func (w *World) drawShapeshift(dst *surface.Surface, owner uint32, x, y int, action int32) bool {
	shape, ok := w.Shapeshifts[owner]
	if !ok || shape.Painter == nil {
		return false
	}
	if lit, ok := shape.Painter.(interface{ SetLit(bool) }); ok {
		lit.SetLit(w.JoinTeamSelection && w.teamHover.owner == owner)
	}
	shape.Painter.Draw(dst, x, y+shape.Info.SpriteDrop(), int(action))
	return true
}
