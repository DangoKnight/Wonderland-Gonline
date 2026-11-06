package world

import "slices"

// HideOtherPlayers makes SQL-authored introductory scenes personal. NPCs,
// authored events and terrain retain their ordinary map behavior.
func (w *World) HideOtherPlayers(mapID uint16) bool {
	m, ok := w.Map(mapID)
	return ok && slices.Contains(w.catalog.Story.PrivateScenes, m.data.Scene)
}
