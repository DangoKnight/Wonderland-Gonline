package game

import (
	"bytes"
	"testing"

	"wonderland-gonline/internal/protocol"
)

func TestTentPositionPackets(t *testing.T) {
	c := Character{ID: 0x01020304, Map: TentMapID, X: 460, Y: 700}

	world := c.PositionPacket()
	want := protocol.Builder{protocol.CommandPosition}.U32(c.ID).U16(TentMapID).U16(460).U16(700)
	if !bytes.Equal(world, want) {
		t.Fatalf("world position %v", world)
	}

	ground := c.PositionPacketFloor(0)
	if len(ground) != len(world)+1 || ground[len(ground)-1] != 0 {
		t.Fatalf("ground level position %v", ground)
	}
	upper := c.PositionPacketFloor(1)
	if len(upper) != len(world)+1 || upper[len(upper)-1] != 1 {
		t.Fatalf("upper level position %v", upper)
	}

	onMap := c.PositionPacketTent(TentUpperMapID, 1)
	wantMap := protocol.Builder{protocol.CommandPosition}.U32(c.ID).U16(TentUpperMapID).U16(460).U16(700).U8(1)
	if !bytes.Equal(onMap, wantMap) {
		t.Fatalf("tent map position %v", onMap)
	}
}
