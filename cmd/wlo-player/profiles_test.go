package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProfilesPreserveCustomizationAndSeparateMemories(t *testing.T) {
	root := t.TempDir()
	aria, aDir, err := loadProfile(root, "Aria")
	if err != nil {
		t.Fatal(err)
	}
	meteo, mDir, err := loadProfile(root, "Meteo")
	if err != nil {
		t.Fatal(err)
	}
	if aria.Name != "Aria" || meteo.Name != "Meteo" || aria.Preferences == meteo.Preferences {
		t.Fatal("missing distinct personas")
	}
	if err := configureProfileAccounts(&aria, aDir, "1,2", 4); err != nil {
		t.Fatal(err)
	}
	aria.Preferences = "Look for beautiful beaches."
	data, _ := json.Marshal(aria)
	if err := os.WriteFile(filepath.Join(aDir, "profile.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	reloaded, _, err := loadProfile(root, "ARIA")
	if err != nil || reloaded.Preferences != aria.Preferences || jsonText(reloaded.Accounts) != "[1,2]" {
		t.Fatal("profile customization was overwritten", err)
	}
	a := newPlayer(nil, credentials{}, "test", aDir, nil)
	if err := a.remember("Found a beach."); err != nil {
		t.Fatal(err)
	}
	m := newPlayer(nil, credentials{}, "test", mDir, nil)
	if m.memory != "" {
		t.Fatal("Meteo inherited Aria's memory")
	}
	a = newPlayer(nil, credentials{}, "test", aDir, nil)
	if a.memory != "Found a beach." {
		t.Fatal("Aria's memory did not survive restart")
	}
	for _, id := range []string{"../escape", "a/b", "a\\b", "", ".", "aria "} {
		if _, _, err := loadProfile(root, id); err == nil {
			t.Fatalf("unsafe profile accepted: %q", id)
		}
	}
}

func TestProfileAccountsAreRequiredAndEnforced(t *testing.T) {
	p, directory, err := loadProfile(t.TempDir(), "Aria")
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range []string{"", "0", "1,1", "5", "no"} {
		candidate := p
		if configureProfileAccounts(&candidate, directory, selection, 4) == nil {
			t.Fatalf("invalid account selection %q accepted", selection)
		}
	}
	if err := configureProfileAccounts(&p, directory, "2,4", 4); err != nil {
		t.Fatal(err)
	}
	g := &sessionGroup{accounts: make([]credentials, 4), allowedAccounts: map[int]bool{2: true, 4: true}}
	if _, err := g.open(context.Background(), 1, 1); err == nil {
		t.Fatal("unassigned account reached login")
	}
	g.rpc = fakeRPC(t, func(string, map[string]any) any { t.Fatal("empty snapshot should not call native host"); return nil })
	state, err := g.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	rows := state["availableAccounts"].([]any)
	if len(rows) != 2 || object(rows[0])["account"] != 2 || object(rows[1])["account"] != 4 {
		t.Fatal("unassigned accounts exposed", rows)
	}
}

func TestProfilePreferencesReachModel(t *testing.T) {
	p := newPlayer(nil, credentials{key: "test"}, "test", t.TempDir(), nil)
	p.profile = &controllerProfile{Name: "Aria", Preferences: "Find interesting beaches.", Accounts: []int{1}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		instructions := textValue(request["instructions"])
		if !strings.Contains(instructions, p.profile.Preferences) || !strings.Contains(instructions, playInstructions) {
			t.Error("missing persona or gameplay rules")
		}
		d := decision{Kind: "wait", ArgumentsJSON: "{}", WaitSeconds: 1, Memory: "Visit a beach."}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": jsonText(d)}}}}})
	}))
	defer server.Close()
	p.endpoint = server.URL
	if _, err := p.decide(context.Background(), groupState(1, false)); err != nil {
		t.Fatal(err)
	}
}

func TestReservationsExcludeCompetitorsAndRelease(t *testing.T) {
	directory := t.TempDir()
	a, b := &reservations{directory: directory}, &reservations{directory: directory}
	defer a.close()
	defer b.close()
	key := reservationKey("account", "PrivateLogin")
	if strings.Contains(key, "PrivateLogin") || key != reservationKey("account", "privatelogin") {
		t.Fatal("unsafe account identity")
	}
	if err := a.acquire(key); err != nil {
		t.Fatal(err)
	}
	if err := b.acquire(key); !errors.Is(err, errReserved) {
		t.Fatal("competitor acquired reserved account", err)
	}
	ok, err := b.available(key)
	if err != nil || ok {
		t.Fatal("reserved account advertised as available", err)
	}
	g := &sessionGroup{accounts: []credentials{{account: "PrivateLogin"}}, reservations: b}
	if _, err := g.open(context.Background(), 1, 1); !errors.Is(err, errReserved) {
		t.Fatal("reserved account reached native host", err)
	}
	a.release(key)
	if err := b.acquire(key); err != nil {
		t.Fatal("released account unavailable", err)
	}
}

func TestReservationHelper(t *testing.T) {
	directory := os.Getenv("WLO_TEST_RESERVATION_CHILD")
	if directory == "" {
		return
	}
	r := &reservations{directory: directory}
	if err := r.acquire("crash-check"); err != nil {
		t.Fatal(err)
	}
	fmt.Println("reservation-ready")
	time.Sleep(time.Minute)
}

func TestReservationsRecoverAfterControllerCrash(t *testing.T) {
	directory := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestReservationHelper$")
	cmd.Env = append(os.Environ(), "WLO_TEST_RESERVATION_CHILD="+directory)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	line, err := bufio.NewReader(pipe).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "reservation-ready" {
		t.Fatal("child did not reserve account", err, line)
	}
	r := &reservations{directory: directory}
	defer r.close()
	if err := r.acquire("crash-check"); !errors.Is(err, errReserved) {
		t.Fatal("cross-process exclusion failed", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if err := r.acquire("crash-check"); err != nil {
		t.Fatal("crash left stale reservation", err)
	}
}
