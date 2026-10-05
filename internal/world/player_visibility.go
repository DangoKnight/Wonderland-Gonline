package world

// Native EVE scene IDs group all map variants of the introductory locations.
// These references come from the SQL map catalog, rather than translated names.
const (
	starterShipDeckScene uint16 = 10001
	starterCabinScene    uint16 = 10002
	robinsonIslandScene  uint16 = 10003
)

// HideOtherPlayers makes introductory scenes personal for player presentation.
// NPCs, authored events and terrain retain their ordinary map behavior.
func (w *World) HideOtherPlayers(mapID uint16) bool {
	m, ok := w.Map(mapID)
	if !ok {
		return false
	}
	switch m.data.Scene {
	case starterShipDeckScene, starterCabinScene, robinsonIslandScene:
		return true
	default:
		return false
	}
}
