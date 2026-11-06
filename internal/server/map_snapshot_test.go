package server

import (
	"bytes"
	"testing"

	"wonderland-gonline/internal/protocol"
)

func TestMapSnapshotCompletionNegotiatedOrder(t *testing.T) {
	for _, complete := range []bool{false, true} {
		s, players, wires := worldFixture(t)
		c := players[0]
		if complete {
			c.connectionFeatures = protocol.ConnectionCompleteMapSnapshot
		}
		if err := s.acknowledgeWorld(players[1]); err != nil {
			t.Fatal(err)
		}
		wires[0].Reset()
		if err := s.acknowledgeWorld(c); err != nil {
			t.Fatal(err)
		}
		packets := wires[0].packets(t)
		ready, lastSnapshot, count := -1, -1, 0
		for i, p := range packets {
			if bytes.Equal(p, []byte{5, 4}) {
				ready = i
				count++
			}
			// This fixture sends the peer appearance/position and a stall snapshot.
			if p[0] == 4 || p[0] == 7 || bytes.Equal(p, []byte{56, 30, 0}) {
				lastSnapshot = i
			}
		}
		if count != 1 || lastSnapshot < 0 {
			t.Fatal("snapshot/ready missing", packets)
		}
		if complete && ready <= lastSnapshot || !complete && ready >= lastSnapshot {
			t.Fatal("wrong negotiated map-ready ordering", complete, packets)
		}
		wires[0].Reset()
		if err := s.acknowledgeWorld(c); err != nil || wires[0].Len() != 0 {
			t.Fatal("duplicate map ACK replayed snapshot", err)
		}
	}
}

func TestCompleteMapSnapshotIncludesTentFurniture(t *testing.T) {
	s, players, wires := tentFixture(t)
	owner, guest := players[0], players[1]
	guest.connectionFeatures = protocol.ConnectionCompleteMapSnapshot
	tradeDo(t, s, owner, []byte{23, 96, 1})
	enterTestTent(t, s, owner, owner.character.ID)
	tradeDo(t, s, guest, protocol.Builder{65, 1}.U32(owner.character.ID))
	wires[1].Reset()
	tradeDo(t, s, owner, protocol.Builder{62, 3}.U16(1).U32(39).U32(44).U32(1).U8(3))
	tradeDo(t, s, guest, []byte{12, 1})
	furniture := tentFurnitureGolden(owner.character.ID, 38027, 39, 44, 3)
	ready, updated := -1, -1
	for i, p := range wires[1].packets(t) {
		if bytes.Equal(p, []byte{5, 4}) {
			ready = i
		}
		if bytes.Equal(p, furniture) {
			updated = i
		}
	}
	if updated < 0 || ready <= updated {
		t.Fatal("map readiness preceded updated tent furniture", wires[1].packets(t))
	}
}
