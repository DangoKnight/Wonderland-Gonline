package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/login"
)

func TestNativeReplicaWorldActionsAndBattle(t *testing.T) {
	fixture := newClientFixture(t)
	host := testClient(t, fixture)
	replica := testClient(t, fixture)
	now := time.Unix(100, 0)
	host.Now = func() time.Time { return now }
	host.enableNativeStream()
	replica.replica = &nativeReplica{}
	replica.Net.SetPresentationOnly()
	packet := func(p []byte) { host.recordNativeNet(login.Event{Kind: login.Packet, Data: p}); host.dispatch(p) }
	sync := func() {
		t.Helper()
		host.Frame()
		batch, err := host.AgentNativeStream(replica.replica.applied)
		if err != nil {
			t.Fatal(err)
		}
		if err = replica.queueNativeBatch(batch); err != nil {
			t.Fatal(err)
		}
		r := replica.replica
		for _, next := range r.pending {
			if next.sequence <= r.applied {
				continue
			}
			r.clock = r.clock.Add(time.Duration(next.frame.Delta))
			r.renderTime = r.clock
			replica.replaying = true
			for _, event := range next.frame.Events {
				if err := replica.applyNativeEvent(event); err != nil {
					t.Fatal(err)
				}
			}
			replica.frame(true)
			replica.replaying = false
			r.applied = next.sequence
		}
		r.pending = nil
	}
	packet(selfPacket(10001, 2, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Dango"))
	packet([]byte{5, 4})
	sync()
	if replica.World == nil || replica.World.Player.X != host.World.Player.X {
		t.Fatal("native map not reconstructed")
	}
	if _, err := host.AgentSubmit([]byte(`{"command":"menu.open","arguments":{"module":"inventory"}}`)); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second / 30)
	sync()
	if !host.Inventory.Visible || !replica.Inventory.Visible {
		t.Fatal("agent menu action was not rendered locally", host.AgentCommandResults(), replica.AgentCommandResults())
	}
	if _, err := host.AgentSubmit([]byte(`{"command":"menu.close","arguments":{"module":"inventory"}}`)); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second / 30)
	sync()
	if replica.Inventory.Visible {
		t.Fatal("replicated menu did not close")
	}
	x, y := host.World.Player.X, host.World.Player.Y
	if _, err := host.AgentSubmit([]byte(fmt.Sprintf(`{"command":"walk.to","arguments":{"x":%d,"y":%d}}`, x+40, y))); err != nil {
		t.Fatal(err)
	}
	for range 8 {
		now = now.Add(100 * time.Millisecond)
		sync()
	}
	if host.World.Player.X == x || host.World.Player.X != replica.World.Player.X || host.World.Player.Y != replica.World.Player.Y {
		t.Fatal("replicated movement differs", host.World.Player, replica.World.Player)
	}
	for _, p := range [][]byte{remoteRecord(250, 2, 4, 2, 10001, 0), remoteRecord(5, 7, 2, 2, 20001, 0), {50, 6, 4, 2, 0}, {52, 1}} {
		packet(p)
	}
	now = now.Add(time.Second)
	sync()
	if !host.battle.state.Active || !replica.battle.state.Active {
		t.Fatal("native battle not reconstructed")
	}
	hostState, _ := json.Marshal(host.battle.state)
	replicaState, _ := json.Marshal(replica.battle.state)
	if !bytes.Equal(hostState, replicaState) {
		t.Fatal("replicated battle differs", string(hostState), string(replicaState))
	}
	now = now.Add(2 * time.Second)
	sync()
	if replica.Screen == nil || !replica.battleFrame(true) {
		t.Fatal("spectator did not create local rendering resources")
	}
}

func TestNativeStreamRedactsCredentials(t *testing.T) {
	c := testClient(t)
	c.enableNativeStream()
	for _, a := range agentActions {
		if a.definition.Name == "login.submit" {
			c.recordNativeCommand(a, map[string]any{"account": "privateaccount", "password": "privatepassword"})
		}
	}
	c.recordNativeControl(c.Login.Account, "set", json.RawMessage(`"privateaccount"`))
	c.Input.Focused = c.Login.Password
	c.recordNativeInput(AgentHostRequest{Kind: "text", Text: "privatepassword"})
	raw, _ := json.Marshal(c.native.events)
	if bytes.Contains(raw, []byte("privateaccount")) || bytes.Contains(raw, []byte("privatepassword")) {
		t.Fatal("credentials leaked into presentation history")
	}
}
