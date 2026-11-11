package app

import (
	"testing"
	"wonderland-gonline/internal/protocol"
)

func TestAFKDeltaReconstructionAndLateSubscriber(t *testing.T) {
	carrier, _, _ := enteredClient(t)
	_ = wire(t, carrier)
	follower, _, _ := enteredClient(t)
	_ = wire(t, follower)
	w := NewWorkspace(carrier)
	defer w.Close()
	follower.afkWorkspace = w
	w.Sessions = append(w.Sessions, &Session{ID: 2, Client: follower})
	binding, _ := protocol.EncodeAFKBinding(protocol.AFKStreamBinding{Stream: 1, Carrier: 1, Members: []uint32{1, 2}})
	for i, c := range []*Client{carrier, follower} {
		c.World.Player.ID = uint32(i + 1)
		c.TeamState.Members = []uint32{1, 2}
		c.connection = connectionRuntime{enabled: true, features: 224}
		c.afk.active = true
		c.afk.groupRequested = true
		if i == 0 {
			c.bindAFKStream(binding)
		}
	}
	base := protocol.AFKState{Units: []protocol.AFKUnit{{Owner: 1, ID: 1, HP: 30}, {Owner: 2, ID: 2, HP: 40}}}
	initial, _ := protocol.EncodeAFKShared(protocol.AFKSharedState{Stream: 1, Sequence: 1, State: base})
	carrier.afkPacket(initial)
	next := base
	next.Units = append([]protocol.AFKUnit(nil), base.Units...)
	next.Units[1].HP = 25
	delta, _ := protocol.EncodeAFKDelta(base, next)
	packet, _ := protocol.EncodeAFKSharedUpdate(1, 2, delta)
	carrier.afkPacket(packet)
	// The follower missed the first update. Fan-out must carry reconstructed full
	// state, rather than a delta referencing a base that this socket never saw.
	follower.bindAFKStream(binding)
	private, _ := protocol.EncodeAFKState(protocol.AFKState{Map: 1, X: 10, Y: 20})
	follower.afkPacket(private)
	w.shareAFKState()
	if follower.afk.sequence != 2 || len(follower.afk.state.Units) != 2 || follower.afk.state.Units[1].HP != 25 {
		t.Fatal(follower.afk.state)
	}
	carrier.afkPacket(packet)
	if carrier.afk.sequence != 2 {
		t.Fatal("duplicate delta reapplied")
	}
	// Independent AFK streams also reconstruct incremental stats.
	lone, _, _ := enteredClient(t)
	_ = wire(t, lone)
	lone.connection = connectionRuntime{enabled: true, features: 160}
	lone.afk.active = true
	base.Map = 1
	full, _ := protocol.EncodeAFKState(base)
	lone.afkPacket(full)
	next.Map = 1
	delta, _ = protocol.EncodeAFKDelta(base, next)
	lone.afkPacket(delta)
	if lone.afk.state.Units[1].HP != 25 || lone.afk.state.Map != 1 {
		t.Fatal(lone.afk.state)
	}
	if _, err := carrier.decodeAFKUpdate(protocol.AFKState{}, delta); err == nil {
		t.Fatal("delta without base accepted")
	}
	lone.connection.features = 32
	if _, err := lone.decodeAFKUpdate(base, delta); err == nil {
		t.Fatal("unnegotiated delta accepted")
	}
}
