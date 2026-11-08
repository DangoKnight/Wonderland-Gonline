package world

import "image"

// Fighting actors are drawn as dust rather than their ordinary body. Reuse
// the field actor's compatibility hit envelope for that replacement model.
func (w *World) BattleTargetAt(x, y int) uint32 {
	cx, cy := w.Camera()
	var id uint32
	depth := -1
	for key, peer := range w.Peers {
		if !w.fighting(key) {
			continue
		}
		bounds := image.Rect(peer.X-cx-teamTargetHalfWidth, peer.Y-cy-teamTargetHeight, peer.X-cx+teamTargetHalfWidth, peer.Y-cy+teamTargetBelowFeet)
		if image.Pt(x, y).In(bounds) && (peer.Y > depth || peer.Y == depth && key < id) {
			id, depth = key, peer.Y
		}
	}
	return id
}
