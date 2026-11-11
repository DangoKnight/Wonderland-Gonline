package app

import (
	"testing"
	"wonderland-gonline/internal/protocol"
)

func TestAFKAggregationAuthorizedFanoutAndReplay(t *testing.T) {
	first, _, _ := enteredClient(t)
	_ = wire(t, first)
	second, _, _ := enteredClient(t)
	_ = wire(t, second)
	w := NewWorkspace(first)
	defer w.Close()
	w.Sessions = append(w.Sessions, &Session{ID: 2, Client: second})
	second.afkWorkspace = w
	for i, c := range []*Client{first, second} {
		c.World.Player.ID = uint32(i + 1)
		c.TeamState.Members = []uint32{1, 2}
		c.connection = connectionRuntime{enabled: true, features: 96}
		c.afk.active = true
		c.afk.groupRequested = true
		c.afk.privateState = protocol.AFKState{Map: 1, X: uint16(i + 5), Units: []protocol.AFKUnit{{Owner: uint32(i + 1), ID: 100, Kind: 1}}}
	}
	bind, _ := protocol.EncodeAFKBinding(protocol.AFKStreamBinding{Stream: 10, Carrier: 1, Members: []uint32{1, 2}})
	first.bindAFKStream(bind)
	shared := protocol.AFKSharedState{Stream: 10, Sequence: 1, State: protocol.AFKState{Units: []protocol.AFKUnit{{Owner: 1, ID: 1, HP: 10}, {Owner: 2, ID: 2, HP: 20}}}}
	packet, _ := protocol.EncodeAFKShared(shared)
	first.afkPacket(packet)
	w.shareAFKState()
	if second.afk.sequence != 0 {
		t.Fatal("unbound subscriber received data")
	}
	second.bindAFKStream(bind)
	for i, c := range []*Client{first, second} {
		c.afk.privateState = protocol.AFKState{Map: 1, X: uint16(i + 5), Units: []protocol.AFKUnit{{Owner: uint32(i + 1), ID: 100, Kind: 1}}}
	}
	w.shareAFKState()
	if second.afk.sequence != 1 || len(second.afk.state.Units) != 3 || second.afk.state.X != 6 || second.afk.state.Units[0].Owner != 2 {
		t.Fatal("private/common merge wrong", second.afk.state)
	}
	shared.State.Units[0].HP = 99
	if second.acceptSharedAFK(shared) {
		t.Fatal("replay accepted")
	}
	shared.Sequence = 2
	shared.Stream = 11
	if second.acceptSharedAFK(shared) {
		t.Fatal("obsolete stream accepted")
	}
	second.TeamState.Members = nil
	shared.Stream = 10
	if second.acceptSharedAFK(shared) {
		t.Fatal("departed party accepted")
	}
	second.TeamState.Members = []uint32{1, 2}
	second.afk.groupRequested = false
	first.afk.latest.Sequence = 2
	w.shareAFKState()
	if second.afk.sequence != 1 {
		t.Fatal("revoked subscriber updated")
	}
	clear, _ := protocol.EncodeAFKBinding(protocol.AFKStreamBinding{})
	first.bindAFKStream(clear)
	if first.afk.latest != nil || first.afk.binding.Stream != 0 {
		t.Fatal("clear retained shared state")
	}
}
