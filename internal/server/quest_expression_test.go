package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/world"
)

func TestNativeQuestPlayerExpressionKeepsHeldPose(t *testing.T) {
	s, c, wire := eventFixture(t)
	c.character.ID = 0x01020304
	c.emote = 26
	ev := assets.Event{Branches: []assets.Branch{{Index: 1}}}
	es := &eventSession{mapID: 10017, ev: &ev, branch: 0}
	wire.Reset()
	ok, err := s.execute(context.Background(), c, es, world.DecodeOp(evOp(1, 1, 7, 14, 1, 0)))
	if !ok || err != nil || c.emote != 26 {
		t.Fatal("expression replaced pose", err)
	}
	if packets := wire.packets(t); len(packets) != 2 || !contains(packets, []byte{32, 1, 4, 3, 2, 1, 114}) {
		t.Fatal("native quest expression missing", packets)
	}
}
