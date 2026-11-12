package app

import (
	"sync"
	"testing"
	"time"
)

func hostOperation(t *testing.T, c *Client, p AgentHostRequest) AgentCommandResult {
	t.Helper()
	result, err := c.AgentHostSubmit(p)
	if err != nil {
		t.Fatal(err)
	}
	c.agentTick()
	for _, r := range c.AgentCommandResults() {
		if r.ID == result.ID {
			return r
		}
	}
	t.Fatal("missing host ticket")
	return AgentCommandResult{}
}
func TestAgentHostOwnershipAndDisconnect(t *testing.T) {
	c := automationClient(t)
	c.AgentEnableSessionHost()
	hostOperation(t, c, AgentHostRequest{Operation: "attach", Viewer: "first"})
	if c.AgentHostState().Viewers != 1 {
		t.Fatal("readonly viewer not observable")
	}
	// Handoff is queued before this API action, so the latter must be cancelled.
	_, err := c.AgentHostSubmit(AgentHostRequest{Operation: "takeover", Viewer: "first", Generation: 1})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := c.AgentSubmit([]byte(`{"command":"remote.stop"}`))
	if err != nil {
		t.Fatal(err)
	}
	c.agentTick()
	if s := c.AgentHostState(); s.Owner != "viewer" || s.Generation != 2 {
		t.Fatal(s)
	}
	c.AgentEnableSessionHost()
	if c.AgentHostState().Generation != 2 {
		t.Fatal("enabling host reset the ownership fence")
	}
	for _, r := range c.AgentCommandResults() {
		if r.ID == queued.ID && r.Status != "cancelled" {
			t.Fatal(r)
		}
	}
	if _, err := c.AgentSubmit([]byte(`{"command":"remote.stop"}`)); err == nil {
		t.Fatal("API input accepted during viewer control")
	}
	for _, p := range []AgentHostRequest{
		{Operation: "takeover", Viewer: "second", Generation: 2},
		{Operation: "release", Viewer: "second", Generation: 2},
		{Operation: "input", Viewer: "first", Generation: 1, Sequence: 1, Kind: "reset"},
	} {
		if r := hostOperation(t, c, p); r.Status != "rejected" {
			t.Fatal(r)
		}
	}
	if r := hostOperation(t, c, AgentHostRequest{Operation: "detach", Viewer: "first"}); r.Status != "executed" {
		t.Fatal(r)
	}
	if s := c.AgentHostState(); s.Owner != "api" || s.Generation != 3 {
		t.Fatal(s)
	}
	if c.AgentHostState().Viewers != 0 {
		t.Fatal("disconnected viewer retained")
	}
	if _, err := c.AgentSubmit([]byte(`{"generation":1,"command":"remote.stop"}`)); err == nil {
		t.Fatal("stale API generation accepted")
	}
	if _, err := c.AgentSubmit([]byte(`{"generation":3,"command":"remote.stop"}`)); err != nil {
		t.Fatal(err)
	}
	// Reconnecting does not automatically acquire control. A detach from the
	// previous connection cannot revoke the new controller's lease.
	hostOperation(t, c, AgentHostRequest{Operation: "takeover", Viewer: "second", Generation: 3})
	hostOperation(t, c, AgentHostRequest{Operation: "detach", Viewer: "first"})
	if s := c.AgentHostState(); s.Viewer != "second" || s.Generation != 4 {
		t.Fatal(s)
	}
}
func TestAgentHostLeaseAndInputSequence(t *testing.T) {
	c := automationClient(t)
	now := time.Now()
	c.Now = func() time.Time { return now }
	c.AgentEnableSessionHost()
	hostOperation(t, c, AgentHostRequest{Operation: "takeover", Viewer: "first", Generation: 1})
	p := AgentHostRequest{Operation: "input", Viewer: "first", Generation: 2, Sequence: 1, Kind: "reset"}
	if r := hostOperation(t, c, p); r.Status != "executed" {
		t.Fatal(r)
	}
	if r := hostOperation(t, c, p); r.Status != "rejected" {
		t.Fatal("replayed input", r)
	}
	now = now.Add(agentViewerLease + time.Millisecond)
	c.agentTick()
	if s := c.AgentHostState(); s.Owner != "api" || s.Generation != 3 {
		t.Fatal(s)
	}
	if c.remote.active || c.battle.auto || c.agent.viewerHeld {
		t.Fatal("automation or held input revived")
	}
}
func TestAgentHostConcurrentAdmission(t *testing.T) {
	c := automationClient(t)
	c.AgentEnableSessionHost()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 32 {
				_, _ = c.AgentSubmit([]byte(`{"command":"remote.stop"}`))
				_ = c.AgentHostState()
				_ = c.AgentCommandResults()
			}
		})
	}
	wg.Wait()
	c.agentTick()
}

func TestAgentHostTakeoverWaitsForAFKResync(t *testing.T) {
	c := automationClient(t)
	c.AgentEnableSessionHost()
	c.afk.resync = true
	r := hostOperation(t, c, AgentHostRequest{Operation: "takeover", Viewer: "first", Generation: 1})
	if r.Status != "rejected" || c.AgentHostState().Owner != "api" {
		t.Fatal(r, c.AgentHostState())
	}
	c.afk.resync = false
	hostOperation(t, c, AgentHostRequest{Operation: "takeover", Viewer: "first", Generation: 1})
	c.afk.pending = true
	r = hostOperation(t, c, AgentHostRequest{Operation: "input", Viewer: "first", Generation: 2, Sequence: 1, Kind: "key", Key: 0x25, Down: true})
	if r.Status != "rejected" || len(c.agent.viewerKeys) != 0 {
		t.Fatal("input crossed scene resync", r)
	}
}
