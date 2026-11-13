package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
)

func automationClient(t *testing.T) *Client {
	t.Helper()
	env := &seui.Env{Pics: picdb.New()}
	ui := seui.NewManager(env, &seui.Input{})
	return &Client{UI: ui, Env: env, Input: ui.Input, Now: time.Now}
}
func agentUICommand(t *testing.T, c *Client, node AgentControlState, action string, value any) AgentCommandResult {
	t.Helper()
	data, err := json.Marshal(map[string]any{"command": "ui.invoke", "arguments": map[string]any{"id": node.ID, "revision": node.Revision, "action": action, "value": value}})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := c.AgentSubmit(data)
	if err != nil {
		t.Fatal(err)
	}
	c.agentTick()
	for _, r := range c.AgentCommandResults() {
		if r.ID == accepted.ID {
			return r
		}
	}
	t.Fatal("ticket disappeared")
	return AgentCommandResult{}
}
func TestAgentUIOwnershipRevisionAndRetirement(t *testing.T) {
	c := automationClient(t)
	f := seui.NewForm(c.Env)
	c.UI.Add(f)
	f.Show()
	e := seui.NewEditor(c.Env, f)
	e.SetText([]byte("old"))
	first := c.agentDescribe(e)
	if result := agentUICommand(t, c, first, "set", "new"); result.Status != "executed" {
		t.Fatalf("set: %+v", result)
	}
	if result := agentUICommand(t, c, first, "set", "wrong"); result.Status != "rejected" || string(e.Text) != "new" {
		t.Fatal("stale control revision executed")
	}
	other := automationClient(t)
	if err := other.agentInvokeControl(first.ID, first.Revision, "set", json.RawMessage(`"cross-client"`)); err == nil {
		t.Fatal("cross-client control accepted")
	}
	latest := c.agentDescribe(e)
	f.Hide()
	c.AgentForms()
	f.Show()
	if err := c.agentInvokeControl(latest.ID, latest.Revision, "set", json.RawMessage(`"revived"`)); err == nil {
		t.Fatal("hidden control handle revived")
	}
	c.agentReset()
	if len(c.agentUI.controls) != 0 {
		t.Fatal("connection reset retained control ownership")
	}
}
func TestAgentContextAndSessionFences(t *testing.T) {
	c := &Client{Now: time.Now}
	c.agent.session = 2
	for _, raw := range []string{`{"session":1,"command":"remote.stop"}`, `{"session":-1,"command":"remote.stop"}`} {
		if _, err := c.AgentSubmit([]byte(raw)); err == nil {
			t.Fatal("wrong session admitted")
		}
	}
	if _, err := c.AgentSubmit([]byte(`{"session":2,"context":{"character":123,"map":1,"turn":0},"command":"remote.stop"}`)); err != nil {
		t.Fatal(err)
	}
	c.agentTick()
	if result := c.AgentCommandResults()[0]; result.Status != "rejected" || !strings.Contains(result.Error, "context") {
		t.Fatalf("context fence: %+v", result)
	}
}
func TestAgentEventHistoryIsBoundedAndPayloadFree(t *testing.T) {
	c := &Client{Now: time.Now}
	c.AgentEvents()
	for range agentHistoryLimit + 1 {
		c.agentObservePacket(append([]byte{23, 2}, []byte("private-credential")...))
	}
	events := c.AgentEvents()
	if len(events) != agentHistoryLimit || events[0].Sequence != 2 || events[len(events)-1].Sequence != agentHistoryLimit+1 {
		t.Fatal("event history is not bounded/monotonic")
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), "private-credential") {
		t.Fatal("packet payload leaked")
	}
}

func TestAgentExecutingTicketRemainsObservable(t *testing.T) {
	c := automationClient(t)
	before := agentActions
	defer func() { agentActions = before }()
	agentActions = append(append([]agentAction{}, before...), agentActionDef("test.yielding", "test only", nil, func(c *Client, _ map[string]any) error {
		results := c.AgentCommandResults()
		if len(results) != 1 || results[0].Status != "executing" {
			t.Fatalf("in-flight ticket disappeared: %+v", results)
		}
		if c.AgentCancel(results[0].ID) {
			t.Fatal("cancelled an action already executing")
		}
		return nil
	}))
	if _, err := c.AgentSubmit([]byte(`{"command":"test.yielding"}`)); err != nil {
		t.Fatal(err)
	}
	c.agentTick()
	if results := c.AgentCommandResults(); len(results) != 1 || results[0].Status != "executed" {
		t.Fatal(results)
	}
}

func TestAgentOperationSchemasAndWhitespaceNulls(t *testing.T) {
	c := automationClient(t)
	for _, raw := range []string{
		`{"command":"workspace.action","arguments":{"operation":"title","parameters":{"ID":1}}}`,
		`{"command":"workspace.action","arguments":{"operation":"add","parameters":{}}}`,
		`{"command":"afk.set","arguments":{"enabled": null }}`,
		`{"command":"walk.to","arguments":{"x": null ,"y":3}}`,
	} {
		if _, err := c.AgentSubmit([]byte(raw)); err == nil {
			t.Fatalf("admitted %s", raw)
		}
	}
}

func TestAgentSessionDefaultFollowsActiveAndSurvivesFirstRemoval(t *testing.T) {
	first, second := automationClient(t), automationClient(t)
	workspace := &Workspace{Sessions: []*Session{{ID: 1, Client: first}, {ID: 2, Client: second}}, Active: 1}
	first.afkWorkspace = workspace
	second.afkWorkspace = workspace
	if first.AgentSession(0) != second || first.AgentSession(1) != first {
		t.Fatal("session routing ignored active/explicit identity")
	}
	workspace.Sessions = workspace.Sessions[1:]
	workspace.Active = 0
	if first.AgentSession(0) != second || first.AgentSession(1) != nil {
		t.Fatal("removed first session remained an API route")
	}
}

func TestAgentNPCInteractionRespectsVisibility(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	const actor = 65000
	c.World.NPCs = map[uint16]*world.NPC{actor: {ClickID: actor, X: c.World.Player.X, Y: c.World.Player.Y}}
	if c.AgentInteractNPC(actor) || len(sent()) != 0 {
		t.Fatal("hidden actor was clickable through API")
	}
	c.World.NPCs[actor].Shown = true
	state := c.AgentState()
	npcs := state["npcs"].([]map[string]any)
	if len(npcs) != 1 || npcs[0]["clickID"] != uint16(actor) || npcs[0]["inInteractionRange"] != true || npcs[0]["interactable"] != true {
		t.Fatal("NPC ID/range state is incomplete", npcs)
	}
	if !c.AgentInteractNPC(actor) || len(sent()) != 1 {
		t.Fatal("visible nearby actor not clickable")
	}
}
