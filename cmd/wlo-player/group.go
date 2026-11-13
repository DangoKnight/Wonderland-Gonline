package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

const (
	openAccountCommand  = "controller.openAccount"
	closeSessionCommand = "controller.closeSession"
	watchSessionCommand = "controller.watchSession"
)

type managedSession struct {
	Account   int
	Character int
}

type spectatorProcess struct {
	command *exec.Cmd
	done    chan struct{}
}

// All access runs on the controller loop. Only the native host owns game state.
type sessionGroup struct {
	rpc             *rpcClient
	root            string
	accounts        []credentials
	sessions        map[int]managedSession
	states          map[int]map[string]any
	viewers         map[int]*spectatorProcess
	showViewers     bool
	startBudget     func()
	reservations    *reservations
	allowedAccounts map[int]bool
	designCharacter characterDesigner
}

func (g *sessionGroup) ids() []int {
	ids := make([]int, 0, len(g.sessions))
	for id := range g.sessions {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func (g *sessionGroup) snapshot() (map[string]any, error) {
	rows := []any{}
	var primary map[string]any
	primaryID := nativeSession
	g.states = map[int]map[string]any{}
	for _, id := range g.ids() {
		g.rpc.session = id
		state, err := g.rpc.call("agent.state", g.rpc.route())
		if err != nil {
			return nil, err
		}
		g.states[id] = state
		member := g.sessions[id]
		rows = append(rows, map[string]any{"session": id, "account": member.Account, "character": member.Character, "state": playerObservation(state)})
		if primary == nil || !readyForDecision(primary) && readyForDecision(state) {
			primary = state
			primaryID = id
		}
	}
	g.rpc.session = primaryID
	if primary == nil {
		primary = map[string]any{"inWorld": false, "details": map[string]any{"control": map[string]any{"owner": "api"}}}
	}
	accounts := []any{}
	for index := range g.accounts {
		if g.allowedAccounts != nil && !g.allowedAccounts[index+1] {
			continue
		}
		active := 0
		for id, member := range g.sessions {
			if member.Account == index+1 {
				active = id
			}
		}
		available := true
		if g.reservations != nil {
			var err error
			available, err = g.reservations.available(reservationKey("account", g.accounts[index].account))
			if err != nil {
				return nil, fmt.Errorf("check account reservation: %w", err)
			}
		}
		accounts = append(accounts, map[string]any{"account": index + 1, "activeSession": active, "available": available, "characterSlots": []int{1, characterSlotCount}})
	}
	result := playerObservation(primary)
	result["controlledSessions"] = rows
	result["availableAccounts"] = accounts
	return result, nil
}

func readyForDecision(state map[string]any) bool {
	return canAct(state) && !boolValue(state["walking"]) && !boolValue(state["movieActive"]) &&
		(!boolValue(object(state["details"])["sceneFrozen"]) || boolValue(object(state["talk"])["shown"]))
}

func (g *sessionGroup) hasReadySession() bool {
	if len(g.sessions) == 0 {
		return true
	}
	for _, state := range g.states {
		if readyForDecision(state) {
			return true
		}
	}
	return false
}

func (g *sessionGroup) workspaceSessions() ([]any, error) {
	list, err := g.rpc.call("agent.list", map[string]any{})
	if err != nil {
		return nil, err
	}
	hosts, _ := list["value"].([]any)
	if len(hosts) != 1 {
		return nil, errors.New("native host list is unavailable")
	}
	sessions, _ := object(hosts[0])["sessions"].([]any)
	return sessions, nil
}

func (g *sessionGroup) waitSession(ctx context.Context, id int, present bool) (int, error) {
	deadline := time.Now().Add(startupTimeout)
	for time.Now().Before(deadline) {
		rows, err := g.workspaceSessions()
		if err != nil {
			return 0, err
		}
		found := false
		for _, value := range rows {
			candidate := number(object(value)["id"])
			if present && g.sessions[candidate].Account == 0 {
				return candidate, nil
			}
			if candidate == id {
				found = true
			}
		}
		if !present && !found {
			return id, nil
		}
		if err := sleep(ctx, pollInterval); err != nil {
			return 0, err
		}
	}
	return 0, errors.New("workspace change did not complete")
}

func (g *sessionGroup) workspace(ctx context.Context, operation string, parameters any) error {
	rows, err := g.workspaceSessions()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		if operation != "add" {
			return errors.New("workspace is empty")
		}
		_, err = g.rpc.call("agent.create", map[string]any{})
		return err
	}
	var source map[string]any
	for _, value := range rows {
		id := number(object(value)["id"])
		g.rpc.session = id
		state, err := g.rpc.call("agent.state", g.rpc.route())
		if err != nil {
			return err
		}
		if !canAct(state) {
			return errors.New("workspace changes are paused during spectator intervention")
		}
		source = state
	}
	_, err = g.rpc.submit(source, "workspace.action", map[string]any{"operation": operation, "parameters": parameters})
	return err
}

func (g *sessionGroup) open(ctx context.Context, account, character int) (map[string]any, error) {
	if account < 1 || account > len(g.accounts) || character < 1 || character > characterSlotCount {
		return nil, errors.New("choose a supplied account and character slot 1 or 2")
	}
	if g.allowedAccounts != nil && !g.allowedAccounts[account] {
		return nil, errors.New("account is outside this profile's allowed set")
	}
	for id, member := range g.sessions {
		if member.Account == account {
			if member.Character != character {
				return nil, errors.New("close this account's active session before selecting its other character")
			}
			g.rpc.session = id
			state, err := g.rpc.call("agent.state", g.rpc.route())
			if err != nil {
				return nil, err
			}
			if !boolValue(state["inWorld"]) {
				return g.loginSession(ctx, id, account, character)
			}
			return map[string]any{"session": id, "alreadyOpen": true}, nil
		}
	}
	key := reservationKey("account", g.accounts[account-1].account)
	registered := false
	if g.reservations != nil {
		if err := g.reservations.acquire(key); err != nil {
			return nil, fmt.Errorf("account %d: %w", account, err)
		}
		defer func() {
			if !registered {
				g.reservations.release(key)
			}
		}()
	}
	rows, err := g.workspaceSessions()
	if err != nil {
		return nil, err
	}
	id := 0
	for _, row := range rows {
		candidate := number(object(row)["id"])
		if g.sessions[candidate].Account == 0 {
			id = candidate
			break
		}
	}
	if id == 0 {
		if err := g.workspace(ctx, "add", nil); err != nil {
			return nil, err
		}
		id, err = g.waitSession(ctx, 0, true)
		if err != nil {
			return nil, err
		}
	}
	g.sessions[id] = managedSession{Account: account, Character: character}
	registered = true // Retain the reservation even if login needs attention.
	return g.loginSession(ctx, id, account, character)
}

// Resume the controller-owned startup path without allocating another session
// or releasing the account reservation when a recoverable startup error occurs.
func (g *sessionGroup) loginSession(ctx context.Context, id, account, character int) (map[string]any, error) {
	g.rpc.session = id
	directory := filepath.Join(g.root, "agent", fmt.Sprintf("account-%d-character-%d", account, character))
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	if err := login(ctx, g.rpc, g.accounts[account-1], character, directory, g.designCharacter); err != nil {
		return map[string]any{"session": id, "needsAttention": true}, err
	}
	if g.startBudget != nil {
		g.startBudget()
	}
	if g.showViewers {
		if err := g.watch(id); err != nil {
			return map[string]any{"session": id}, err
		}
	}
	return map[string]any{"session": id, "account": account, "character": character}, nil
}

func (g *sessionGroup) watch(id int) error {
	if _, ok := g.sessions[id]; !ok {
		return errors.New("session is not controlled by this agent")
	}
	if viewer := g.viewers[id]; viewer != nil {
		select {
		case <-viewer.done:
		default:
			return nil
		}
	}
	g.rpc.session = id
	result, err := g.rpc.call("agent.viewer.open", g.rpc.route())
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(g.root, "wonderland-client.exe"), "-attach", textValue(result["url"]), "-assets", filepath.Join(g.root, "client-assets.zip"))
	cmd.Dir = g.root
	if err := cmd.Start(); err != nil {
		return err
	}
	viewer := &spectatorProcess{command: cmd, done: make(chan struct{})}
	g.viewers[id] = viewer
	go func() { _ = cmd.Wait(); close(viewer.done) }()
	return nil
}

func (g *sessionGroup) close(ctx context.Context, id int) error {
	member, ok := g.sessions[id]
	if !ok {
		return errors.New("session is not controlled by this agent")
	}
	if err := g.workspace(ctx, "close", map[string]any{"ID": id}); err != nil {
		return err
	}
	if _, err := g.waitSession(ctx, id, false); err != nil {
		return err
	}
	delete(g.sessions, id)
	delete(g.states, id)
	if g.reservations != nil {
		g.reservations.release(reservationKey("account", g.accounts[member.Account-1].account))
	}
	return nil
}

func (g *sessionGroup) execute(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	switch name {
	case openAccountCommand:
		return g.open(ctx, number(args["account"]), number(args["character"]))
	case closeSessionCommand:
		id := number(args["session"])
		return map[string]any{"closedSession": id}, g.close(ctx, id)
	case watchSessionCommand:
		id := number(args["session"])
		return map[string]any{"watchingSession": id}, g.watch(id)
	}
	return nil, errors.New("unknown controller command")
}

func groupCommandDefinitions() []any {
	return []any{
		map[string]any{"name": openAccountCommand, "arguments": []any{map[string]any{"name": "account", "type": "integer", "min": 1}, map[string]any{"name": "character", "type": "integer", "min": 1, "max": characterSlotCount}}, "prerequisite": "choose an available supplied account; creates its character if empty; opens an independent native session"},
		map[string]any{"name": closeSessionCommand, "arguments": []any{map[string]any{"name": "session", "type": "integer", "min": 1}}, "prerequisite": "close one of your controlled sessions; spectator intervention blocks workspace changes"},
		map[string]any{"name": watchSessionCommand, "arguments": []any{map[string]any{"name": "session", "type": "integer", "min": 1}}, "prerequisite": "open a native spectator window for one controlled session"},
	}
}

// Decode parameters independently of model schema and reject fractional IDs.
func validateGroupArguments(name string, args map[string]any) error {
	for key, value := range args {
		n, ok := value.(float64)
		if !ok || n != float64(int(n)) || n < 1 {
			return errors.New("controller account, character and session IDs must be positive integers")
		}
		if name == openAccountCommand && key != "account" && key != "character" || name != openAccountCommand && key != "session" {
			return errors.New("unknown controller argument")
		}
	}
	if name == openAccountCommand {
		if args["account"] == nil || args["character"] == nil {
			return errors.New("account and character are required")
		}
	} else if args["session"] == nil {
		return errors.New("session is required")
	}
	return nil
}
