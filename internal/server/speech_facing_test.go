package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/world"
)

func TestInstalledBoundXaolanTalkPreservesFacing(t *testing.T) {
	const northIslandMap, boundXaolanActor, boundXaolanEvent = 60001, 62, 46
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, c, wire := eventFixture(t)
	s.Assets, s.World = catalog, world.New(catalog)
	c.character.Map = northIslandMap
	ev, ok := s.World.Event(northIslandMap, boundXaolanEvent)
	if !ok {
		t.Fatal("native bound Xaolan event missing")
	}
	if err := s.startEvent(context.Background(), c, boundXaolanActor, ev, 0, true); err != nil {
		t.Fatal(err)
	}
	// Independent raw golden, including native no-turn mode and 24-bit talk ID.
	want := []byte{20, 1, 0, 0, 0, 1, 1, 3, 62, 0, 2, 0, 0, 0, 0, 110, 117, 0}
	if !contains(wire.packets(t), want) {
		t.Fatal("native no-turn speech mode was discarded", wire.packets(t))
	}
}
