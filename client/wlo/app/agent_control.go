package app

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"
	"wonderland-gonline/client/wlo/seui"
)

const (
	AgentAPIVersion       = 2
	agentQueueLimit       = 64
	agentHistoryLimit     = 128
	agentCommandsPerFrame = 8
	agentRequestBytes     = 16384
	agentDefaultTimeout   = 10 * time.Second
	agentMaxTimeoutMS     = 30000
)

type AgentArgument struct {
	Schema    *seui.AutomationSchema `json:"schema,omitempty"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Min       int                    `json:"min,omitempty"`
	Max       int                    `json:"max,omitempty"`
	Sensitive bool                   `json:"sensitive,omitempty"`
}
type AgentCommandDefinition struct {
	Name         string                           `json:"name"`
	Arguments    []AgentArgument                  `json:"arguments"`
	Prerequisite string                           `json:"prerequisite"`
	Variants     map[string]seui.AutomationSchema `json:"variants,omitempty"`
}
type AgentRequest struct {
	Generation uint64                     `json:"generation,omitempty"`
	Context    *AgentContext              `json:"context,omitempty"`
	Session    int                        `json:"session,omitempty"`
	Command    string                     `json:"command"`
	Arguments  map[string]json.RawMessage `json:"arguments"`
	TimeoutMS  int                        `json:"timeoutMs,omitempty"`
}

type AgentContext struct {
	Character uint32 `json:"character"`
	Map       uint16 `json:"map"`
	Turn      uint64 `json:"turn"`
}
type AgentCommandResult struct {
	AfterSequence uint64     `json:"afterSequence"`
	ID            string     `json:"id"`
	Command       string     `json:"command"`
	Status        string     `json:"status"`
	Error         string     `json:"error,omitempty"`
	QueuedAt      time.Time  `json:"queuedAt"`
	ExecutedAt    *time.Time `json:"executedAt,omitempty"`
	// Local execution never claims an authoritative server outcome.
	ServerConfirmed bool `json:"serverConfirmed"`
}
type agentQueuedCommand struct {
	generation uint64
	host       *AgentHostRequest
	context    *AgentContext
	result     AgentCommandResult
	args       map[string]any
	deadline   time.Time
}
type agentRuntime struct {
	serverAttached   atomic.Bool
	serverPermission atomic.Int32
	hosted           bool
	viewers          map[string]bool
	control          AgentHostControl
	lease            time.Time
	viewerGame       *Game
	viewerKeys       map[uint16]bool
	viewerHeld       bool
	viewerX, viewerY int
	viewerSequence   uint64
	wantFrame        bool
	frame            AgentHostFrame
	frameAt          time.Time
	observe          atomic.Bool
	sequence         uint64
	events           []AgentEvent
	session          int
	mu               sync.Mutex
	prefix           string
	serial           uint64
	queue            []agentQueuedCommand
	history          []AgentCommandResult
	executing        *AgentCommandResult
}
type agentAction struct {
	definition AgentCommandDefinition
	run        func(*Client, map[string]any) error
}

func AgentContract() any {
	commands := make([]AgentCommandDefinition, 0, len(agentActions))
	for _, a := range agentActions {
		commands = append(commands, a.definition)
	}
	return struct {
		Version    int                                         `json:"version"`
		QueueLimit int                                         `json:"queueLimit"`
		Commands   []AgentCommandDefinition                    `json:"commands"`
		Completion string                                      `json:"completion"`
		Controls   map[string]map[string]seui.AutomationSchema `json:"controls"`
		Sessions   string                                      `json:"sessions"`
		Queries    map[string]seui.AutomationSchema            `json:"queries"`
	}{AgentAPIVersion, agentQueueLimit, commands, "executed means local action dispatched; observe game state and server packet sequences for server completion or rejection", seui.AutomationContract(), "optional request.session selects an existing workspace session; handles and tickets remain session-owned", map[string]seui.AutomationSchema{"navigation": seui.SchemaOf(AgentNavigationRequest{}), "lookup": seui.SchemaOf(struct {
		Kind    string `json:"kind"`
		ID      uint32 `json:"id"`
		Session int    `json:"session,omitempty"`
	}{})}}
}

// AgentSubmit is safe to call outside the game loop. Arguments are validated
// before admission; live eligibility is rechecked on the owning client's frame.
// Credentials exist only in the pending request, never its retained result.
func (c *Client) AgentSubmit(data []byte) (AgentCommandResult, error) {
	if len(data) > agentRequestBytes {
		return AgentCommandResult{}, fmt.Errorf("request too large")
	}
	var request AgentRequest
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&request); err != nil {
		return AgentCommandResult{}, fmt.Errorf("invalid request")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return AgentCommandResult{}, fmt.Errorf("invalid trailing request data")
	}
	var action *agentAction
	for i := range agentActions {
		if agentActions[i].definition.Name == request.Command {
			action = &agentActions[i]
			break
		}
	}
	if action == nil {
		return AgentCommandResult{}, fmt.Errorf("unknown command")
	}
	if err := c.agentCommandAccess(request.Command); err != nil {
		return AgentCommandResult{}, err
	}
	if len(request.Arguments) != len(action.definition.Arguments) {
		return AgentCommandResult{}, fmt.Errorf("incorrect arguments")
	}
	values := map[string]any{}
	for _, a := range action.definition.Arguments {
		raw, ok := request.Arguments[a.Name]
		if !ok {
			return AgentCommandResult{}, fmt.Errorf("missing argument: %s", a.Name)
		}
		raw = bytes.TrimSpace(raw)
		switch a.Type {
		case "integer":
			var n float64
			if err := json.Unmarshal(raw, &n); err != nil || bytes.Equal(raw, []byte("null")) || math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n || n < float64(a.Min) || n > float64(a.Max) {
				return AgentCommandResult{}, fmt.Errorf("invalid integer: %s", a.Name)
			}
			values[a.Name] = int(n)
		case "boolean":
			var v bool
			if err := json.Unmarshal(raw, &v); err != nil || bytes.Equal(raw, []byte("null")) {
				return AgentCommandResult{}, fmt.Errorf("invalid boolean: %s", a.Name)
			}
			values[a.Name] = v
		case "string":
			var v string
			if err := json.Unmarshal(raw, &v); err != nil || len(v) < a.Min || len(v) > a.Max || bytes.Equal(raw, []byte("null")) {
				return AgentCommandResult{}, fmt.Errorf("invalid string: %s", a.Name)
			}
			values[a.Name] = v
		case "json":
			if !json.Valid(raw) {
				return AgentCommandResult{}, fmt.Errorf("invalid JSON argument: %s", a.Name)
			}
			if a.Schema != nil {
				if err := seui.ValidateAutomationSchema(raw, *a.Schema); err != nil {
					return AgentCommandResult{}, err
				}
			}
			values[a.Name] = append(json.RawMessage(nil), raw...)
		}
	}
	if variants := action.definition.Variants; len(variants) > 0 {
		operation, _ := values["operation"].(string)
		schema, ok := variants[operation]
		if !ok {
			return AgentCommandResult{}, fmt.Errorf("unknown operation")
		}
		if err := seui.ValidateAutomationSchema(values["parameters"].(json.RawMessage), schema); err != nil {
			return AgentCommandResult{}, err
		}
	}
	if request.TimeoutMS < 0 || request.TimeoutMS > agentMaxTimeoutMS {
		return AgentCommandResult{}, fmt.Errorf("timeoutMs must be 0..30000")
	}
	timeout := agentDefaultTimeout
	if request.TimeoutMS > 0 {
		timeout = time.Duration(request.TimeoutMS) * time.Millisecond
	}
	r := &c.agent
	r.mu.Lock()
	defer r.mu.Unlock()
	if request.Session < 0 || request.Session != 0 && request.Session != r.session {
		return AgentCommandResult{}, fmt.Errorf("session does not own this client")
	}
	if len(r.queue) >= agentQueueLimit {
		return AgentCommandResult{}, fmt.Errorf("command queue full")
	}
	if r.hosted && (r.control.Owner != "api" || request.Generation != 0 && request.Generation != r.control.Generation) {
		return AgentCommandResult{}, fmt.Errorf("input owner or generation changed")
	}
	if r.prefix == "" {
		var token [16]byte
		if _, err := rand.Read(token[:]); err != nil {
			return AgentCommandResult{}, fmt.Errorf("command identity unavailable")
		}
		r.prefix = hex.EncodeToString(token[:])
	}
	r.serial++
	now := c.Now()
	r.observe.Store(true)
	result := AgentCommandResult{ID: fmt.Sprintf("%s:%d", r.prefix, r.serial), Command: request.Command, Status: "accepted", QueuedAt: now, AfterSequence: r.sequence}
	r.queue = append(r.queue, agentQueuedCommand{generation: r.control.Generation, result: result, args: values, deadline: now.Add(timeout), context: request.Context})
	return result, nil
}

func (c *Client) AgentCommandResults() []AgentCommandResult {
	r := &c.agent
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]AgentCommandResult{}, r.history...)
	if r.executing != nil {
		out = append(out, *r.executing)
	}
	for _, q := range r.queue {
		out = append(out, q.result)
	}
	return out
}
func (c *Client) AgentCancel(id string) bool {
	r := &c.agent
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, q := range r.queue {
		if q.result.ID == id {
			result := q.result
			result.Status = "cancelled"
			r.retain(result)
			copy(r.queue[i:], r.queue[i+1:])
			r.queue[len(r.queue)-1] = agentQueuedCommand{}
			r.queue = r.queue[:len(r.queue)-1]
			return true
		}
	}
	return false
}
func (r *agentRuntime) retain(result AgentCommandResult) {
	if len(r.history) == agentHistoryLimit {
		copy(r.history, r.history[1:])
		r.history = r.history[:len(r.history)-1]
	}
	r.history = append(r.history, result)
}

// A new connection must not inherit queued actions from a previous character.
func (c *Client) agentReset() {
	c.agentUI = agentUIRuntime{serial: c.agentUI.serial}
	r := &c.agent
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, q := range r.queue {
		result := q.result
		result.Status = "cancelled"
		r.retain(result)
		r.queue[i] = agentQueuedCommand{}
	}
	r.queue = nil
}
func (c *Client) agentTick() {
	c.agentHostTick()
	c.agentRetireHiddenControls()
	for range agentCommandsPerFrame {
		r := &c.agent
		r.mu.Lock()
		if len(r.queue) == 0 {
			r.mu.Unlock()
			return
		}
		q := r.queue[0]
		copy(r.queue, r.queue[1:])
		r.queue[len(r.queue)-1] = agentQueuedCommand{}
		r.queue = r.queue[:len(r.queue)-1]
		inFlight := q.result
		inFlight.Status = "executing"
		r.executing = &inFlight
		r.mu.Unlock()
		result := q.result
		now := c.Now()
		result.ExecutedAt = &now
		if !now.Before(q.deadline) {
			result.Status = "timed_out"
		} else if q.host != nil {
			result.Status = "executed"
			if err := c.agentHostExecute(*q.host); err != nil {
				result.Status = "rejected"
				result.Error = err.Error()
			}
		} else if c.agentHosted() && (q.generation != c.AgentHostState().Generation || c.AgentHostState().Owner != "api") {
			result.Status = "rejected"
			result.Error = "input owner or generation changed"
		} else if q.context != nil && *q.context != c.agentContext() {
			result.Status = "rejected"
			result.Error = "game context changed; inspect current character, map and turn"
		} else {
			result.Status = "executed"
			if err := c.agentCommandAccess(result.Command); err != nil {
				result.Status = "rejected"
				result.Error = err.Error()
			} else {
				for _, a := range agentActions {
					if a.definition.Name == result.Command {
						c.recordNativeCommand(a, q.args)
						if c.native != nil {
							c.native.depth++
						}
						if err := a.run(c, q.args); err != nil {
							result.Status = "rejected"
							result.Error = err.Error()
						}
						if c.native != nil {
							c.native.depth--
						}
						break
					}
				}
			}
		}
		r.mu.Lock()
		r.executing = nil
		r.retain(result)
		r.mu.Unlock()
	}
}
