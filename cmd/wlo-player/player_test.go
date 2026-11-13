package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestControllerAllowsGameChatWithoutOpeningRestrictedCommands(t *testing.T) {
	names := []string{"chat.send", "world.interactNPC", "chat.admin", "mail.send", "social.friend", "trade.offer", "login.submit", "server.shutdown"}
	commands := make([]any, 0, len(names))
	for _, name := range names {
		commands = append(commands, map[string]any{"name": name})
	}
	p := newPlayer(nil, credentials{}, "test-model", t.TempDir(), map[string]any{"commands": commands})
	for _, name := range names {
		want := name == "chat.send" || name == "world.interactNPC"
		if p.allowed[name] != want {
			t.Errorf("command %s allowed=%v, want %v", name, p.allowed[name], want)
		}
	}
	filtered, _ := p.contract["commands"].([]any)
	if len(filtered) != 2 || textValue(object(filtered[0])["name"]) != "chat.send" {
		t.Fatal("chat command was not exposed in the model contract", filtered)
	}
}

func TestCredentialsNeverNeedModelAccess(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	path := filepath.Join(t.TempDir(), ".secrets")
	if err := os.WriteFile(path, []byte("Username: first\nPassword: private1\nusername_02=second\npassword_02=private2\nOPENAI_API_KEY=test-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := readCredentials(path, 2)
	if err != nil || c.account != "second" || c.password != "private2" || c.key != "test-key" {
		t.Fatal("incorrect credential routing")
	}
	if _, err := readCredentials(path, 3); err == nil {
		t.Fatal("missing account accepted")
	}
	t.Setenv("OPENAI_API_KEY", "environment-key")
	c, err = readCredentials(path, 1)
	if err != nil || c.key != "environment-key" {
		t.Fatal("environment key was not preferred")
	}
}

func TestGameplayDeadlineEndsDuringWalking(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		defer server.Close()
		_, _ = bufio.NewReader(server).ReadBytes('\n')
		_ = json.NewEncoder(server).Encode(map[string]any{"id": 1, "result": map[string]any{"inWorld": true, "walking": true}})
	}()
	rpc := &rpcClient{conn: client, reader: bufio.NewReader(client)}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := play(ctx, rpc, nil, 0); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != context.DeadlineExceeded {
		t.Fatal("play stopped before its deadline")
	}
}

func TestGameplayDeadlineCancelsPendingModelRequest(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		defer server.Close()
		_, _ = bufio.NewReader(server).ReadBytes('\n')
		_ = json.NewEncoder(server).Encode(map[string]any{"id": 1, "result": map[string]any{"inWorld": true, "details": map[string]any{"control": map[string]any{"owner": "api"}}}})
	}()
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body any
		_ = json.NewDecoder(r.Body).Decode(&body)
		<-r.Context().Done()
	}))
	defer model.Close()
	rpc := &rpcClient{conn: client, reader: bufio.NewReader(client)}
	p := newPlayer(rpc, credentials{key: "test-key"}, "test-model", t.TempDir(), map[string]any{})
	p.endpoint = model.URL
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := play(ctx, rpc, p, 0); err != nil {
		t.Fatal(err)
	}
	if rpc.serial != 1 || ctx.Err() != context.DeadlineExceeded {
		t.Fatal("deadline allowed another game action")
	}
}

func TestRPCSupportsAuthAndChecksCorrelation(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		defer server.Close()
		reader := bufio.NewReader(server)
		_, _ = reader.ReadBytes('\n')
		_, _ = server.Write([]byte("{\"id\":1,\"result\":\"ok\"}\n"))
		_, _ = reader.ReadBytes('\n')
		_, _ = server.Write([]byte("{\"id\":99,\"result\":{}}\n"))
	}()
	rpc := &rpcClient{conn: client, reader: bufio.NewReader(client)}
	if _, err := rpc.call("auth", map[string]any{"token": "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := rpc.call("agent.state", route()); err == nil {
		t.Fatal("mismatched response accepted")
	}
}

func TestModelReceivesStateWithoutSecretsAndUsesStructuredOutput(t *testing.T) {
	c := credentials{account: "player", password: "password-value", key: "private-key"}
	p := newPlayer(nil, c, "test-model", t.TempDir(), map[string]any{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-key" {
			t.Error("missing model authentication")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		input := textValue(body["input"])
		if strings.Contains(input, c.key) || strings.Contains(input, c.password) || strings.Contains(input, "hidden-npc") {
			t.Error("private or invisible data sent to model")
		}
		if body["store"] != false || object(object(body["text"])["format"])["strict"] != true {
			t.Error("missing structured output configuration")
		}
		d := decision{Kind: "wait", ArgumentsJSON: "{}", Memory: "Visit the village next.", Reason: "Read the dialogue.", WaitSeconds: 2}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": jsonText(d)}}}}})
	}))
	defer server.Close()
	p.endpoint = server.URL
	d, err := p.decide(context.Background(), map[string]any{"talk": c.password, "npcs": []any{map[string]any{"name": "hidden-npc", "visible": false}}})
	if err != nil || d.Kind != "wait" {
		t.Fatal(d, err)
	}
	if err := p.remember(d.Memory); err != nil {
		t.Fatal(err)
	}
	if err := p.remember("Updated memory"); err != nil {
		t.Fatal(err)
	}
	reloaded := newPlayer(nil, c, "test-model", p.logRoot, map[string]any{})
	if reloaded.memory != "Updated memory" {
		t.Fatal("memory did not survive restart")
	}
}

func TestModelOperationNamesAreConstrainedToFilteredLiveContract(t *testing.T) {
	p := newPlayer(nil, credentials{key: "test-key"}, "test-model", t.TempDir(), map[string]any{
		"commands": []any{map[string]any{"name": "world.interactNPC"}, map[string]any{"name": "login.submit"}},
		"queries":  map[string]any{"navigation": map[string]any{}, "lookup": map[string]any{}},
	})
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		schema := object(object(object(body["text"])["format"])["schema"])
		names := object(object(schema["properties"])["name"])["enum"]
		if jsonText(names) != `["","lookup","navigation","world.interactNPC"]` {
			t.Error("model can invent operation names or lost native queries", names)
		}
		d := decision{Kind: "command", Name: "world.interactNPC", ArgumentsJSON: `{"id":8}`, WaitSeconds: 1}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": jsonText(d)}}}}})
	}))
	defer model.Close()
	p.endpoint = model.URL
	d, err := p.decide(context.Background(), nil)
	if err != nil || d.Name != "world.interactNPC" {
		t.Fatal(d, err)
	}
}

func TestViewerTakeoverDiscardsModelAction(t *testing.T) {
	state := map[string]any{"details": map[string]any{"context": map[string]any{"map": 1}, "control": map[string]any{"owner": "api", "generation": 1}}}
	viewer := map[string]any{"details": map[string]any{"context": map[string]any{"map": 1}, "control": map[string]any{"owner": "viewer", "generation": 2}}}
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		defer server.Close()
		_, _ = bufio.NewReader(server).ReadBytes('\n')
		_ = json.NewEncoder(server).Encode(map[string]any{"id": 1, "result": viewer})
	}()
	rpc := &rpcClient{conn: client, reader: bufio.NewReader(client)}
	p := newPlayer(rpc, credentials{key: "test-key"}, "test-model", t.TempDir(), map[string]any{})
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := decision{Kind: "command", Name: "walk.to", ArgumentsJSON: `{"x":10,"y":20}`, Memory: "Explore.", Reason: "Walk north.", WaitSeconds: 1}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": jsonText(d)}}}}})
	}))
	defer model.Close()
	p.endpoint = model.URL
	if err := p.step(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if rpc.serial != 1 {
		t.Fatal("action submitted after takeover")
	}
	if sameContext(state, viewer) {
		t.Fatal("old decision survived ownership generation change")
	}
}

func TestModelQuotaFailureDoesNotExposeResponseOrCredentials(t *testing.T) {
	p := newPlayer(nil, credentials{key: "private-key"}, "test-model", t.TempDir(), map[string]any{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "insufficient_quota", "message": "private-key"}})
	}))
	defer server.Close()
	p.endpoint = server.URL
	_, err := p.decide(context.Background(), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "billing/credits") || strings.Contains(err.Error(), "private-key") {
		t.Fatal("unsafe or unhelpful quota diagnostic")
	}
}
