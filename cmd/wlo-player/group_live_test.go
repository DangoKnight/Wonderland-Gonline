package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in integration check against an operator-provided exported client and
// its configured server. It creates empty characters and temporarily teams them.
func TestLiveNativeGroup(t *testing.T) {
	root := os.Getenv("WLO_PLAYER_LIVE_ROOT")
	if root == "" {
		t.Skip("set WLO_PLAYER_LIVE_ROOT to explicitly enable live account integration")
	}
	accounts, err := readAccounts(filepath.Join(root, ".secrets"))
	if err != nil || len(accounts) < 2 {
		t.Fatal("two supplied accounts are required")
	}
	logFile, err := os.Create(filepath.Join(t.TempDir(), "host.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	host, rpc, err := startHost(ctx, root, logFile)
	if host != nil {
		defer func() { _ = host.Process.Kill(); _ = host.Wait() }()
	}
	if err != nil {
		t.Fatal(err)
	}
	defer rpc.conn.Close()
	g := &sessionGroup{rpc: rpc, root: root, accounts: accounts, sessions: map[int]managedSession{}, viewers: map[int]*spectatorProcess{}}
	first, err := g.open(ctx, 1, 1)
	if err != nil {
		t.Fatal("first account startup failed", err)
	}
	second, err := g.open(ctx, 2, 1)
	if err != nil {
		t.Fatal("second account startup failed", err)
	}
	firstID, secondID := first["session"].(int), second["session"].(int)
	if firstID == secondID {
		t.Fatal("accounts share a session")
	}
	if _, err := g.snapshot(); err != nil {
		t.Fatal(err)
	}
	leader, follower := g.states[firstID], g.states[secondID]
	if !boolValue(leader["inWorld"]) || !boolValue(follower["inWorld"]) {
		t.Fatal("both accounts did not enter the world")
	}
	t.Logf("Two independent native sessions ready: %d and %d", firstID, secondID)
	// Wait for map data before using the ordinary nearby-player team operation.
	for i := 0; i < 15; i++ {
		if boolValue(object(leader["details"])["mapReady"]) && boolValue(object(follower["details"])["mapReady"]) && readyForDecision(leader) && readyForDecision(follower) {
			break
		}
		if err := sleep(ctx, pollInterval); err != nil {
			t.Fatal(err)
		}
		if _, err := g.snapshot(); err != nil {
			t.Fatal(err)
		}
		leader, follower = g.states[firstID], g.states[secondID]
	}
	if number(object(leader["map"])["id"]) != number(object(follower["map"])["id"]) {
		t.Fatal("test characters must be on the same map to verify teaming")
	}
	t.Run("party", func(t *testing.T) {
		peers, _ := follower["peers"].([]any)
		visible := false
		for _, peer := range peers {
			if number(object(peer)["id"]) == number(object(leader["player"])["id"]) {
				visible = true
			}
		}
		if !visible {
			t.Skip("characters are not mutually visible; finish private tutorial scenes before verifying party formation")
		}
		rpc.session = secondID
		result, err := rpc.submit(follower, "world.target", map[string]any{"operation": "joinTeam", "id": object(leader["player"])["id"]})
		if err != nil {
			t.Fatal(err)
		}
		t.Log("Join request", jsonText(result))
		confirmed := false
		for i := 0; i < 15; i++ {
			if err := sleep(ctx, pollInterval); err != nil {
				t.Fatal(err)
			}
			if _, err := g.snapshot(); err != nil {
				t.Fatal(err)
			}
			leader = g.states[firstID]
			forms, _ := leader["forms"].([]any)
			for _, rawForm := range forms {
				form := object(rawForm)
				if !boolValue(form["modal"]) {
					continue
				}
				controls, _ := form["controls"].([]any)
				for _, rawControl := range controls {
					control := object(rawControl)
					if !strings.Contains(strings.ToLower(textValue(control["asset"])), "btn_ok_1") || !boolValue(control["enabled"]) {
						continue
					}
					rpc.session = firstID
					if _, err := rpc.submit(leader, "ui.invoke", map[string]any{"id": control["id"], "revision": control["revision"], "action": "activate", "value": nil}); err != nil {
						t.Fatal(err)
					}
					confirmed = true
				}
			}
			if confirmed {
				break
			}
		}
		if !confirmed {
			for _, id := range []int{firstID, secondID} {
				details := object(g.states[id]["details"])
				t.Log("Session", id, "notices", jsonText(details["notices"]), "party", jsonText(details["party"]), "commands", jsonText(g.states[id]["commands"]))
				forms, _ := g.states[id]["forms"].([]any)
				for _, raw := range forms {
					form := object(raw)
					if boolValue(form["modal"]) {
						t.Log("Modal", jsonText(form))
					}
				}
			}
			t.Log("Leader location", jsonText(g.states[firstID]["player"]), "Follower location", jsonText(g.states[secondID]["player"]))
			t.Fatal("team confirmation was not exposed to the API")
		}
		formed := false
		for i := 0; i < 10; i++ {
			if err := sleep(ctx, pollInterval); err != nil {
				t.Fatal(err)
			}
			if _, err := g.snapshot(); err != nil {
				t.Fatal(err)
			}
			party := object(object(g.states[secondID]["details"])["party"])
			members, _ := party["Members"].([]any)
			if len(members) == 2 && number(party["Leader"]) == number(object(g.states[firstID]["player"])["id"]) {
				t.Log("Both characters formed a party")
				formed = true
				break
			}
		}
		if !formed {
			t.Fatal("server did not confirm the two-character party")
		}
	})
	if err := g.close(ctx, secondID); err != nil {
		t.Fatal(err)
	}
	if _, err := g.snapshot(); err != nil || len(g.states) != 1 {
		t.Fatal("closing one character interrupted the remaining character", err)
	}
}
