package app

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestAgentContractValidation(t *testing.T) {
	c := &Client{Now: time.Now}
	for _, request := range []string{
		`null`, `{}`, `{"command":"missing"}`,
		`{"command":"walk.to","arguments":{"x":1.5,"y":2}}`,
		`{"command":"walk.to","arguments":{"x":null,"y":2}}`,
		`{"command":"walk.to","arguments":{"x":-1,"y":2}}`,
		`{"command":"walk.to","arguments":{"x":1,"y":65536}}`,
		`{"command":"walk.to","arguments":{"x":1,"y":2,"extra":3}}`,
		`{"command":"afk.set","arguments":{"enabled":null}}`,
		`{"command":"remote.stop","secret":"private"}`,
		`{"command":"remote.stop","timeoutMs":30001}`,
		`{"command":"remote.stop"} {}`,
		`{"command":"login.submit","arguments":{"account":"sample","password":"very-private-password"}}`,
	} {
		if _, err := c.AgentSubmit([]byte(request)); err == nil {
			t.Errorf("accepted invalid request: %s", request)
		}
	}
	if _, err := c.AgentSubmit(bytes.Repeat([]byte(" "), agentRequestBytes+1)); err == nil {
		t.Fatal("accepted oversized request")
	}
	if len(c.AgentCommandResults()) != 0 {
		t.Fatal("invalid requests entered history")
	}
	seen := map[string]bool{}
	for _, a := range agentActions {
		if seen[a.definition.Name] {
			t.Fatalf("duplicate command %s", a.definition.Name)
		}
		seen[a.definition.Name] = true
	}
}

func TestAgentQueueLifecycleAndPrivacy(t *testing.T) {
	now := time.Unix(100, 0)
	c := &Client{Now: func() time.Time { return now }}
	request := []byte(`{"command":"login.submit","arguments":{"account":"sample","password":"secret123456"}}`)
	accepted, err := c.AgentSubmit(request)
	if err != nil || accepted.Status != "accepted" {
		t.Fatalf("admission: %+v %v", accepted, err)
	}
	b, _ := json.Marshal(c.AgentCommandResults())
	if bytes.Contains(b, []byte("secret123456")) || bytes.Contains(b, []byte("sample")) {
		t.Fatal("credentials leaked into results")
	}
	other := &Client{Now: c.Now}
	if other.AgentCancel(accepted.ID) {
		t.Fatal("another client cancelled the ticket")
	}
	if !c.AgentCancel(accepted.ID) || c.AgentCancel(accepted.ID) {
		t.Fatal("cancellation not exactly once")
	}
	if len(c.agent.queue) != 0 || c.AgentCommandResults()[0].Status != "cancelled" {
		t.Fatal("cancelled command retained for execution")
	}
	if _, err := c.AgentSubmit([]byte(`{"command":"remote.stop","timeoutMs":1}`)); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Millisecond)
	c.agentTick()
	if result := c.AgentCommandResults()[1]; result.Status != "timed_out" || result.ServerConfirmed {
		t.Fatalf("timeout: %+v", result)
	}
	if _, err := c.AgentSubmit(request); err != nil {
		t.Fatal(err)
	}
	c.agentTick() // No login form: admission succeeds, local execution rejects.
	if result := c.AgentCommandResults()[2]; result.Status != "rejected" || result.ServerConfirmed {
		t.Fatalf("rejection: %+v", result)
	}
	if _, err := c.AgentSubmit(request); err != nil {
		t.Fatal(err)
	}
	c.agentReset()
	if result := c.AgentCommandResults()[3]; result.Status != "cancelled" {
		t.Fatalf("connection reset: %+v", result)
	}
}

func TestAgentConcurrentAdmissionAndBounds(t *testing.T) {
	c := &Client{Now: time.Now}
	var wg sync.WaitGroup
	for range agentQueueLimit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.AgentSubmit([]byte(`{"command":"login.submit","arguments":{"account":"sample","password":"1234"}}`)); err != nil {
				t.Error(err)
			}
			_ = c.AgentCommandResults()
		}()
	}
	wg.Wait()
	if _, err := c.AgentSubmit([]byte(`{"command":"remote.stop"}`)); err == nil {
		t.Fatal("queue limit not enforced")
	}
	ids := map[string]bool{}
	for _, r := range c.AgentCommandResults() {
		if ids[r.ID] {
			t.Fatal("duplicate ticket")
		}
		ids[r.ID] = true
	}
	c.agentTick()
	if len(c.agent.queue) != agentQueueLimit-agentCommandsPerFrame {
		t.Fatal("frame budget not enforced")
	}
	c.agentReset()
	for range agentHistoryLimit {
		if _, err := c.AgentSubmit([]byte(`{"command":"login.submit","arguments":{"account":"sample","password":"1234"}}`)); err != nil {
			t.Fatal(err)
		}
		c.agentTick()
	}
	if len(c.AgentCommandResults()) != agentHistoryLimit {
		t.Fatal("history is not bounded")
	}
}

func TestAgentBattleDirectExecution(t *testing.T) {
	c, _, sent := enterBattle(t)
	if _, err := c.AgentSubmit([]byte(`{"command":"battle.attack","arguments":{"targetX":2,"targetY":2}}`)); err != nil {
		t.Fatal(err)
	}
	if c.hotbar.submitted[[2]byte{4, 2}] {
		t.Fatal("action executed before owning frame")
	}
	c.agentTick()
	results := c.AgentCommandResults()
	if len(results) != 1 || results[0].Status != "executed" {
		t.Fatalf("execution: %+v", results)
	}
	found := false
	for _, packet := range sent() {
		// Independent native basic-attack packet, without input emulation.
		if bytes.Equal(packet, []byte{50, 1, 4, 2, 2, 2, 17, 39}) {
			found = true
		}
	}
	if !found {
		t.Fatalf("basic attack packet missing: %v", sent())
	}
}

func TestAgentBattleRejectsUnavailableActions(t *testing.T) {
	c, _, sent := enterBattle(t)
	c.remote.active = true
	c.remote.options.AutoFight = true
	for _, request := range []string{
		`{"command":"battle.attack","arguments":{"targetX":2,"targetY":2}}`,
		`{"command":"battle.flee"}`,
		`{"command":"battle.capture","arguments":{"targetX":2,"targetY":2}}`,
	} {
		if _, err := c.AgentSubmit([]byte(request)); err != nil {
			t.Fatal(err)
		}
	}
	c.agentTick()
	for _, result := range c.AgentCommandResults() {
		if result.Status != "rejected" {
			t.Fatalf("remote-controlled action falsely succeeded: %+v", result)
		}
	}
	if packets := sent(); len(packets) != 0 {
		t.Fatalf("rejected actions sent packets: %v", packets)
	}
}

func TestAgentNamedModuleWithoutInput(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	before := *c.Input
	for _, command := range []string{"menu.open", "menu.close"} {
		data, _ := json.Marshal(map[string]any{"command": command, "arguments": map[string]any{"module": "inventory"}})
		if _, err := c.AgentSubmit(data); err != nil {
			t.Fatal(err)
		}
		c.agentTick()
		results := c.AgentCommandResults()
		if results[len(results)-1].Status != "executed" {
			t.Fatalf("module action: %+v", results)
		}
		if c.Inventory.Visible != (command == "menu.open") {
			t.Fatalf("module visibility did not change: %s", command)
		}
	}
	if *c.Input != before {
		t.Fatal("named module action synthesized input")
	}
}
