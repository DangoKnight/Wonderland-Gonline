// wlo-player controls a native client using observations and semantic commands.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

const (
	rpcTimeout              = 15 * time.Second
	startupTimeout          = 90 * time.Second
	pollInterval            = time.Second
	decisionInterval        = 3 * time.Second
	nativeSession           = 1
	maxMemoryBytes          = 8000
	defaultGameplayDuration = time.Hour
	characterSlotCount      = 2
)

type credentials struct{ account, password, key string }

func readCredentials(path string, index int) (credentials, error) {
	accounts, err := readAccounts(path)
	if err != nil {
		return credentials{}, err
	}
	if index < 1 || index > len(accounts) {
		return credentials{}, errors.New("account index is outside the local secrets entries")
	}
	return accounts[index-1], nil
}

func readAccounts(path string) ([]credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read local secrets file: %w", err)
	}
	var accounts []credentials
	var current credentials
	var accountSuffix string
	key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		separator := strings.IndexAny(line, "=:")
		if separator < 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(line[:separator]))
		value := strings.Trim(strings.TrimSpace(line[separator+1:]), "\"'")
		label := strings.TrimRight(name, "_0123456789")
		switch label {
		case "openai_api_key":
			if key == "" {
				key = value
			}
		case "username", "account":
			current.account = value
			accountSuffix = strings.TrimPrefix(name, label)
		case "password":
			if strings.TrimPrefix(name, label) != accountSuffix {
				return nil, errors.New("username/password suffixes do not match in local secrets")
			}
			current.password = value
			if current.account != "" {
				accounts = append(accounts, current)
				current = credentials{}
			}
		}
	}
	if key == "" {
		return nil, errors.New("add OPENAI_API_KEY to .secrets or the environment before starting")
	}
	if len(accounts) == 0 {
		return nil, errors.New("no game accounts found in local secrets")
	}
	seenAccounts := map[string]bool{}
	for i := range accounts {
		identity := strings.ToLower(accounts[i].account)
		if seenAccounts[identity] {
			return nil, errors.New("duplicate game account entries in local secrets")
		}
		seenAccounts[identity] = true
		accounts[i].key = key
	}
	return accounts, nil
}

type rpcClient struct {
	conn    net.Conn
	reader  *bufio.Reader
	serial  int
	session int // Selected explicitly for every hosted account action.
}

func (r *rpcClient) call(method string, params any) (map[string]any, error) {
	r.serial++
	if err := r.conn.SetDeadline(time.Now().Add(rpcTimeout)); err != nil {
		return nil, err
	}
	if err := json.NewEncoder(r.conn).Encode(map[string]any{"jsonrpc": "2.0", "id": r.serial, "method": method, "params": params}); err != nil {
		return nil, err
	}
	var response struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	// Decode through a persistent buffered reader so coalesced messages survive.
	line, err := r.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(line, &response); err != nil {
		return nil, err
	}
	if response.ID != r.serial {
		return nil, errors.New("native API response ID mismatch")
	}
	if response.Error != nil {
		return nil, errors.New(response.Error.Message)
	}
	var result any
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, err
	}
	if mapped, ok := result.(map[string]any); ok {
		return mapped, nil
	}
	return map[string]any{"value": result}, nil
}

func route() map[string]any { return map[string]any{"id": "native", "session": nativeSession} }
func (r *rpcClient) sessionID() int {
	if r.session == 0 {
		return nativeSession
	}
	return r.session
}
func (r *rpcClient) route() map[string]any {
	return map[string]any{"id": "native", "session": r.sessionID()}
}
func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func textValue(v any) string      { s, _ := v.(string); return s }
func boolValue(v any) bool        { b, _ := v.(bool); return b }
func jsonText(v any) string       { b, _ := json.Marshal(v); return string(b) }

func control(state map[string]any) map[string]any { return object(object(state["details"])["control"]) }
func canAct(state map[string]any) bool            { return textValue(control(state)["owner"]) == "api" }

func (r *rpcClient) submit(state map[string]any, command string, args map[string]any) (map[string]any, error) {
	if !canAct(state) {
		return nil, errors.New("spectator owns input")
	}
	return r.call("agent.execute", map[string]any{"id": "native", "request": map[string]any{
		"session": r.sessionID(), "command": command, "arguments": args,
		"context": object(state["details"])["context"], "generation": control(state)["generation"],
	}})
}

func hasForm(state map[string]any, name string) bool {
	forms, _ := state["openForms"].([]any)
	for _, form := range forms {
		if textValue(form) == name {
			return true
		}
	}
	return false
}

func sleep(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		return nil
	}
}

// Each startup action is submitted once and checked by its ticket. Character
// creation uses normal form callbacks and waits for the server's updated roster.
func login(ctx context.Context, rpc *rpcClient, c credentials, slot int, directory string, designers ...characterDesigner) error {
	profile, err := loadCharacterProfile(directory)
	if err != nil {
		return err
	}
	sent := map[string]bool{}
	tickets := map[string]string{}
	lastForms := ""
	deadline := time.Now().Add(3 * startupTimeout)
	nameExecuted := false
	designAttempts := 0
	if profile.Design != nil {
		designAttempts = 1
	}
	for time.Now().Before(deadline) {
		state, err := rpc.call("agent.state", rpc.route())
		if err != nil {
			return err
		}
		if boolValue(state["inWorld"]) {
			return nil
		}
		results, _ := state["commands"].([]any)
		for _, value := range results {
			result := object(value)
			stage := tickets[textValue(result["id"])]
			if stage == "" {
				continue
			}
			if stage == "create-next-name" && textValue(result["status"]) == "executed" {
				nameExecuted = true
			}
			if status := textValue(result["status"]); status == "rejected" || status == "timed_out" || status == "cancelled" {
				return fmt.Errorf("startup stage %s %s", stage, status)
			}
			if textValue(result["status"]) == "executed" {
				if strings.HasPrefix(stage, "create-stat-") || strings.HasPrefix(stage, "create-color-") || strings.HasPrefix(stage, "create-carousel-") || stage == "create-role" {
					delete(sent, stage)
				}
				delete(tickets, textValue(result["id"]))
			}
		}
		if textValue(state["status"]) == "agent_access_blocked" {
			return errors.New(textValue(state["error"]))
		}
		if !canAct(state) {
			if err := sleep(ctx, pollInterval); err != nil {
				return err
			}
			continue
		}
		if hasForm(state, creationForm) && canAct(state) {
			creation := creationValue(state)
			if number(creation["step"]) == creationNameStep && !boolValue(creation["waiting"]) {
				if nameExecuted && !boolValue(creation["nameAccepted"]) {
					profile.RejectedNames = append(profile.RejectedNames, profile.Name)
					if len(profile.RejectedNames) > maxRejectedCreationNames {
						profile.RejectedNames = profile.RejectedNames[len(profile.RejectedNames)-maxRejectedCreationNames:]
					}
					profile.Name, profile.Design = "", nil
					if err := saveCharacterProfile(directory, profile); err != nil {
						return err
					}
					sent, tickets, nameExecuted = map[string]bool{}, map[string]string{}, false
				}
				if profile.Design == nil {
					if designAttempts >= creationNameAttempts {
						return errors.New("character name attempts exhausted; inspect the saved design and server notice")
					}
					if len(designers) == 0 || designers[0] == nil {
						return errors.New("character creation requires an agent designer")
					}
					started := time.Now()
					design, err := designers[0](ctx, state, profile.RejectedNames)
					deadline = deadline.Add(time.Since(started))
					if err != nil {
						return fmt.Errorf("character design failed: %w", err)
					}
					roles, _ := creation["roles"].([]any)
					if err := design.validate(roles); err != nil {
						return err
					}
					profile.Name, profile.Design = design.Name, &design
					designAttempts++
					if err := saveCharacterProfile(directory, profile); err != nil {
						return err
					}
				}
			}
			if profile.Design != nil {
				roles, _ := creation["roles"].([]any)
				if err := profile.Design.validate(roles); err != nil {
					return err
				}
			}
		}
		forms := jsonText(state["openForms"])
		if forms != lastForms {
			fmt.Println("Startup forms:", forms)
			lastForms = forms
		}
		action, err := chooseStartupAction(state, c, slot, profile, sent)
		if err != nil {
			return err
		}
		if action.key != "" && !sent[action.key] && canAct(state) {
			result, err := rpc.submit(state, action.command, action.args)
			if err != nil {
				message := err.Error()
				for _, secret := range []string{c.account, c.password, c.key, profile.DeletionCode} {
					if secret != "" {
						message = strings.ReplaceAll(message, secret, "[redacted]")
					}
				}
				return fmt.Errorf("startup stage %s failed: %s", action.key, message)
			}
			sent[action.key] = true
			tickets[textValue(result["id"])] = action.key
			fmt.Println("Submitted", action.key)
		}
		if err := sleep(ctx, pollInterval); err != nil {
			return err
		}
	}
	return errors.New("startup did not reach the world; inspect the host log and creation form")
}
func startHost(ctx context.Context, root string, logFile *os.File) (*exec.Cmd, *rpcClient, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	address := listener.Addr().String()
	listener.Close()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, nil, err
	}
	token := hex.EncodeToString(secret)
	cmd := exec.CommandContext(ctx, filepath.Join(root, "wonderland-client.exe"), "-headless", "-api-listen", address,
		"-assets", filepath.Join(root, "client-assets.zip"), "-serverini", filepath.Join(root, "SERVER.INI"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "WLO_AGENT_TOKEN="+token)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	deadline := time.Now().Add(startupTimeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			rpc := &rpcClient{conn: conn, reader: bufio.NewReader(conn)}
			if _, err := rpc.call("auth", map[string]any{"token": token}); err != nil {
				conn.Close()
				return cmd, nil, err
			}
			return cmd, rpc, nil
		}
		if err := sleep(ctx, pollInterval); err != nil {
			return cmd, nil, err
		}
	}
	return cmd, nil, errors.New("native host startup timed out; inspect host.log")
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	root := flag.String("root", filepath.Dir(executable), "exported client directory")
	account := flag.Int("account", 0, "initial account entry in .secrets; 0 lets the agent choose")
	slot := flag.Int("character", 1, "existing character slot, 1 or 2")
	model := flag.String("model", "gpt-5-mini", "OpenAI model with structured output support")
	limit := flag.Int("decisions", 0, "optional maximum gameplay/coordinator decisions; character designs are additional; 0 applies no decision cap")
	duration := flag.Duration("duration", defaultGameplayDuration, "maximum elapsed gameplay time after entering the world")
	viewer := flag.Bool("spectator", true, "open the native spectator")
	profileName := flag.String("profile", "default", "persistent controller profile, e.g. Aria or Meteo")
	profileAccounts := flag.String("accounts", "", "save this profile's allowed account numbers, e.g. 1,2")
	setupProfile := flag.Bool("setup-profile", false, "save/validate profile configuration without starting gameplay")
	coordinationDir := flag.String("coordination-dir", "", "shared controller lock directory; defaults to the current user's configuration directory")
	flag.Parse()
	if *limit < 0 || *slot < 1 || *slot > characterSlotCount || *duration <= 0 {
		return errors.New("invalid duration, decision limit or character slot")
	}
	accounts, err := readAccounts(filepath.Join(*root, ".secrets"))
	if err != nil {
		return err
	}
	if *account < 0 || *account > len(accounts) {
		return errors.New("initial account is outside the supplied accounts")
	}
	if *coordinationDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		*coordinationDir = filepath.Join(base, "WonderlandGonline", "controller-locks")
	}
	locks := &reservations{directory: *coordinationDir}
	defer locks.close() // Registered before host cleanup: connections close first.
	absoluteRoot, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	if err := locks.acquire(reservationKey("profile", absoluteRoot+":"+strings.ToLower(*profileName))); err != nil {
		return fmt.Errorf("profile: %w", err)
	}
	profile, profileDirectory, err := loadProfile(*root, *profileName)
	if err != nil {
		return err
	}
	if err := configureProfileAccounts(&profile, profileDirectory, *profileAccounts, len(accounts)); err != nil {
		return err
	}
	allowedAccounts := map[int]bool{}
	for _, id := range profile.Accounts {
		allowedAccounts[id] = true
	}
	if *account > 0 && !allowedAccounts[*account] {
		return errors.New("initial account is outside this profile's allowed set")
	}
	if *setupProfile {
		fmt.Printf("Profile %s configured in %s\n", profile.Name, profileDirectory)
		return nil
	}
	if *account > 0 {
		if err := locks.acquire(reservationKey("account", accounts[*account-1].account)); err != nil {
			return fmt.Errorf("initial account: %w", err)
		}
	}
	c := accounts[0]
	if *account > 0 {
		c = accounts[*account-1]
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	logRoot := profileDirectory
	if err := os.MkdirAll(logRoot, 0700); err != nil {
		return err
	}
	hostLog, err := os.OpenFile(filepath.Join(logRoot, "host.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer hostLog.Close()
	host, rpc, err := startHost(ctx, *root, hostLog)
	if host != nil {
		defer func() { _ = host.Process.Kill(); _ = host.Wait() }()
	}
	if err != nil {
		return err
	}
	defer rpc.conn.Close()
	fmt.Println("Native host started.")
	gameplayCtx, stopGameplay := context.WithCancelCause(ctx)
	defer stopGameplay(nil)
	var gameplayTimer *time.Timer
	startBudget := func() {
		if gameplayTimer == nil {
			gameplayTimer = time.AfterFunc(*duration, func() { stopGameplay(context.DeadlineExceeded) })
		}
	}
	defer func() {
		if gameplayTimer != nil {
			gameplayTimer.Stop()
		}
	}()
	// Interrupt pending local RPC reads as well as model requests at expiry.
	stopRPC := context.AfterFunc(gameplayCtx, func() { _ = rpc.conn.Close() })
	defer stopRPC()
	contract, err := rpc.call("agent.contract", route())
	if err != nil {
		return err
	}
	player := newPlayer(rpc, c, *model, logRoot, contract)
	player.profile = &profile
	player.group = &sessionGroup{rpc: rpc, root: *root, accounts: accounts,
		sessions: map[int]managedSession{}, startBudget: startBudget, reservations: locks, allowedAccounts: allowedAccounts,
		states: map[int]map[string]any{}, viewers: map[int]*spectatorProcess{}, showViewers: *viewer, designCharacter: player.designCharacter}
	if *account > 0 {
		if _, err := player.group.open(gameplayCtx, *account, *slot); err != nil {
			return err
		}
	}
	commands, _ := player.contract["commands"].([]any)
	player.contract["commands"] = append(commands, groupCommandDefinitions()...)
	fmt.Println("Exploration started. Take control in the spectator to pause the agent. Ctrl+C stops the controller and its host.")
	if err := play(gameplayCtx, rpc, player, *limit); err != nil {
		return err
	}
	if errors.Is(context.Cause(gameplayCtx), context.DeadlineExceeded) {
		fmt.Println("Gameplay time limit reached. Play journal saved in agent/.")
	} else if gameplayCtx.Err() == nil {
		fmt.Println("Decision limit reached. Play journal saved in agent/.")
	}
	return nil
}

func play(ctx context.Context, rpc *rpcClient, player *player, limit int) error {
	for decisions := 0; limit == 0 || decisions < limit; {
		if ctx.Err() != nil {
			return nil
		}
		var state map[string]any
		var err error
		if player != nil && player.group != nil {
			state, err = player.group.snapshot()
		} else {
			state, err = rpc.call("agent.state", rpc.route())
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if textValue(state["status"]) == "agent_access_blocked" {
			return errors.New(textValue(state["error"]))
		}
		if (player == nil || player.group == nil) && !boolValue(state["inWorld"]) {
			return errors.New("game session left the world; inspect the spectator or host log before restarting")
		}
		if player != nil && player.group != nil && !player.group.hasReadySession() || (player == nil || player.group == nil) && (!canAct(state) || boolValue(state["walking"]) || boolValue(state["movieActive"]) ||
			(boolValue(object(state["details"])["sceneFrozen"]) && !boolValue(object(state["talk"])["shown"]))) {
			if err := sleep(ctx, pollInterval); err != nil {
				return nil
			}
			continue
		}
		if err := player.step(ctx, state); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		decisions++
		if err := sleep(ctx, decisionInterval); err != nil {
			return nil
		}
	}

	return nil
}
