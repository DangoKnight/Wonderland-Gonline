package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeRPC(t *testing.T, handle func(string, map[string]any) any) *rpcClient {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	go func() {
		defer server.Close()
		scanner := bufio.NewScanner(server)
		for scanner.Scan() {
			var request struct {
				ID     int            `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if json.Unmarshal(scanner.Bytes(), &request) != nil {
				return
			}
			if json.NewEncoder(server).Encode(map[string]any{"id": request.ID, "result": handle(request.Method, request.Params)}) != nil {
				return
			}
		}
	}()
	return &rpcClient{conn: client, reader: bufio.NewReader(client)}
}

func groupState(id int, walking bool) map[string]any {
	return map[string]any{"inWorld": true, "walking": walking, "player": map[string]any{"id": float64(10000 + id)}, "details": map[string]any{"context": map[string]any{"character": float64(10000 + id), "map": float64(1), "turn": float64(1)}, "control": map[string]any{"owner": "api", "generation": float64(1)}}}
}

func TestGroupSnapshotRoutesToReadyCharacterAndHidesCredentials(t *testing.T) {
	rpc := fakeRPC(t, func(method string, params map[string]any) any {
		id := number(params["session"])
		return groupState(id, id == 1)
	})
	g := &sessionGroup{rpc: rpc, accounts: []credentials{{account: "private-account", password: "private-password", key: "private-model-key"}, {account: "second-account", password: "second-password"}}, sessions: map[int]managedSession{1: {Account: 1, Character: 1}, 7: {Account: 2, Character: 1}}}
	state, err := g.snapshot()
	if err != nil || rpc.sessionID() != 7 || len(g.states) != 2 {
		t.Fatal("group did not select the ready member", err)
	}
	if strings.Contains(jsonText(state), "private-") || strings.Contains(jsonText(state), "second-password") {
		t.Fatal("credentials leaked into group observations")
	}
	rows := state["controlledSessions"].([]any)
	if object(rows[1])["session"] != 7 || !g.hasReadySession() {
		t.Fatal("missing stable session routing")
	}
}

func TestGroupDecisionTargetsChosenSession(t *testing.T) {
	var submissions []int
	rpc := fakeRPC(t, func(method string, params map[string]any) any {
		if method == "agent.execute" {
			submissions = append(submissions, number(object(params["request"])["session"]))
			return map[string]any{"status": "accepted"}
		}
		return groupState(number(params["session"]), false)
	})
	p := newPlayer(rpc, credentials{key: "test-key"}, "test-model", t.TempDir(), map[string]any{"commands": []any{map[string]any{"name": "walk.to"}}})
	p.group = &sessionGroup{rpc: rpc, sessions: map[int]managedSession{1: {Account: 1, Character: 1}, 7: {Account: 2, Character: 1}}, states: map[int]map[string]any{1: groupState(1, false), 7: groupState(7, false)}}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := decision{Kind: "command", Session: 7, Name: "walk.to", ArgumentsJSON: `{"x":20,"y":30}`, Memory: "Coordinate the party.", Reason: "Move the second member.", WaitSeconds: 1}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": jsonText(d)}}}}})
	}))
	defer model.Close()
	p.endpoint = model.URL
	if err := p.step(context.Background(), groupState(1, false)); err != nil {
		t.Fatal(err)
	}
	if len(submissions) != 1 || submissions[0] != 7 {
		t.Fatal("command was routed to the wrong character", submissions)
	}
}

func TestGroupRejectsDuplicateAccountAndForeignTargets(t *testing.T) {
	rpc := fakeRPC(t, func(string, map[string]any) any { return groupState(7, false) })
	g := &sessionGroup{rpc: rpc, accounts: []credentials{{}}, sessions: map[int]managedSession{7: {Account: 1, Character: 1}}, states: map[int]map[string]any{7: groupState(7, false)}}
	if _, err := g.open(context.Background(), 1, 2); err == nil {
		t.Fatal("two simultaneous characters from one account accepted")
	}
	result, err := g.open(context.Background(), 1, 1)
	if err != nil || result["alreadyOpen"] != true {
		t.Fatal("repeated account open was not idempotent")
	}
	p := &player{group: g}
	if !p.groupTargetAllowed(map[string]any{"operation": "joinTeam", "id": float64(10007)}) {
		t.Fatal("controlled teammate was rejected")
	}
	if p.groupTargetAllowed(map[string]any{"operation": "joinTeam", "id": float64(99999)}) {
		t.Fatal("unowned player accepted")
	}
	if validateGroupArguments(openAccountCommand, map[string]any{"account": 1.5, "character": float64(1)}) == nil {
		t.Fatal("fractional account ID accepted")
	}
}

func TestGroupOpenResumesExistingStartupWithoutAllocatingSession(t *testing.T) {
	state := startupState("SelectCharacter", startupControl("Enter[0]"))
	state["details"] = groupState(7, false)["details"]
	submissions := 0
	rpc := fakeRPC(t, func(method string, params map[string]any) any {
		switch method {
		case "agent.state":
			if number(params["session"]) != 7 {
				t.Error("startup resumed on another session")
			}
			return state
		case "agent.execute":
			request := object(params["request"])
			if request["command"] != "character.select" || number(object(request["arguments"])["slot"]) != 1 {
				t.Error("unexpected startup action", request["command"])
			}
			submissions++
			state["inWorld"] = true
			return map[string]any{"id": "resume", "status": "accepted"}
		default:
			t.Error("resume allocated or modified workspace", method)
			return nil
		}
	})
	started := 0
	g := &sessionGroup{rpc: rpc, root: t.TempDir(), accounts: []credentials{{}}, sessions: map[int]managedSession{7: {Account: 1, Character: 1}}, startBudget: func() { started++ }}
	result, err := g.open(context.Background(), 1, 1)
	if err != nil || result["session"] != 7 || submissions != 1 || started != 1 || len(g.sessions) != 1 {
		t.Fatal("incomplete startup was not resumed", result, err, submissions, started)
	}
}

func TestGroupBlocksModelLoginThroughGenericUI(t *testing.T) {
	state := groupState(1, false)
	state["inWorld"] = false
	submissions := 0
	rpc := fakeRPC(t, func(method string, _ map[string]any) any {
		if method == "agent.execute" {
			submissions++
			return map[string]any{"status": "accepted"}
		}
		return state
	})
	p := newPlayer(rpc, credentials{key: "test-key"}, "test-model", t.TempDir(), map[string]any{"commands": []any{map[string]any{"name": "ui.invoke"}}})
	p.group = &sessionGroup{rpc: rpc, sessions: map[int]managedSession{1: {Account: 1, Character: 1}}, states: map[int]map[string]any{1: state}}
	p.contract["commands"] = append(p.contract["commands"].([]any), groupCommandDefinitions()...)
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		schema := object(object(object(body["text"])["format"])["schema"])
		names := jsonText(object(object(schema["properties"])["name"])["enum"])
		if strings.Contains(names, "ui.invoke") || !strings.Contains(names, openAccountCommand) {
			t.Error("incomplete startup offered gameplay instead of recovery", names)
		}
		// A malicious/nonconforming response must still fail the execution gate.
		d := decision{Kind: "command", Session: 1, Name: "ui.invoke", ArgumentsJSON: `{"id":"login-account","revision":"current","action":"set","value":"unassigned-account"}`, Memory: "Recover the disconnected character.", Reason: "Try logging in through a form.", WaitSeconds: 1}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": jsonText(d)}}}}})
	}))
	defer model.Close()
	p.endpoint = model.URL
	if err := p.step(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if submissions != 0 || !strings.Contains(jsonText(p.recent), "controller account commands") {
		t.Fatal("model bypassed assigned-account startup through generic UI")
	}
}

func TestGroupAllowsStopBeforeWorldEntry(t *testing.T) {
	state := groupState(1, false)
	state["inWorld"] = false
	rpc := fakeRPC(t, func(string, map[string]any) any { return state })
	p := newPlayer(rpc, credentials{key: "test-key"}, "test-model", t.TempDir(), nil)
	p.group = &sessionGroup{rpc: rpc, sessions: map[int]managedSession{1: {Account: 1, Character: 1}}, states: map[int]map[string]any{1: state}}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		d := decision{Kind: "stop", ArgumentsJSON: `{}`, Reason: "startup unavailable", WaitSeconds: 1}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": jsonText(d)}}}}})
	}))
	defer model.Close()
	p.endpoint = model.URL
	if err := p.step(context.Background(), state); err == nil || !strings.Contains(err.Error(), "agent stopped: startup unavailable") {
		t.Fatal("offline session prevented controller shutdown", err)
	}
}
