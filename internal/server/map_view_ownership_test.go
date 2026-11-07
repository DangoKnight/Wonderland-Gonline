package server

import "testing"

func TestMapAcknowledgmentPreservesSnapshotHidesAndClearsStaleMap(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	s.World.MapInfo(c.character, c.view, nil)
	c.view.Hidden[119] = true // A scripted hide applied in the destination snapshot.
	if err := s.acknowledgeWorld(c); err != nil {
		t.Fatal(err)
	}
	if !c.view.Hidden[119] || c.view.HiddenMap != c.character.Map {
		t.Fatal("acknowledgment discarded destination visibility")
	}
	c.character.Map = 20000
	c.ready = false // A real warp clears readiness before the next map acknowledgment.
	if err := s.acknowledgeWorld(c); err != nil {
		t.Fatal(err)
	}
	if c.view.Hidden[119] || c.view.HiddenMap != 20000 {
		t.Fatal("old map click IDs survived map change")
	}
}
