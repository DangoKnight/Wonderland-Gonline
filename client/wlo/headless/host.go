// Package headless runs the native Go client and its API without a browser,
// web server, WebAssembly runtime, audio device or game window.
package headless

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"wonderland-gonline/client/wlo/agentwire"
	"wonderland-gonline/client/wlo/app"
)

const (
	nativeAgentID         = "native"
	minimumTokenBytes     = 32
	requestQueueLimit     = 64
	connectionLimit       = 64
	hostTick              = time.Second / 60
	requestTimeout        = 10 * time.Second
	authenticationTimeout = 5 * time.Second
	writeTimeout          = 3 * time.Second
	viewerReadTimeout     = 10 * time.Second
	viewerFrameInterval   = 125 * time.Millisecond
	viewerTicketLifetime  = time.Hour
	viewerTicketLimit     = 128
	rpcInvalidRequest     = -32602
	rpcOperationFailed    = 3
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}
type call struct {
	ctx    context.Context
	run    func() (any, error)
	result chan outcome
}
type outcome struct {
	data json.RawMessage
	err  error
}
type viewerTicket struct {
	session int
	expires time.Time
}
type host struct {
	w       *app.Workspace
	address string
	token   string
	calls   chan call
	tickets map[string]viewerTicket // owned by the simulation loop
	step    func()
}

func Validate(address, token string) error {
	hostname, _, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(hostname) == nil || !net.ParseIP(hostname).IsLoopback() {
		return errors.New("native API must bind a numeric loopback address")
	}
	if len(token) < minimumTokenBytes {
		return errors.New("set WLO_AGENT_TOKEN to a random secret of at least 32 bytes")
	}
	return nil
}

// Run owns the workspace on this goroutine until shutdown. Socket readers never
// read live game state: queries and actions are serialized here, then marshaled
// before crossing goroutine boundaries. Losing any API/viewer socket leaves this
// loop and the game's TCP connections intact.
func Run(ctx context.Context, address, token string, c *app.Client) error {
	if err := Validate(address, token); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	log.Printf("native headless API: tcp://%s (no web endpoints)", listener.Addr())
	c.AgentEnableSessionHost()
	w := app.NewWorkspace(c)
	defer w.Close()
	h := newHost(w, listener.Addr().String(), token)
	return h.serve(ctx, listener)
}
func newHost(w *app.Workspace, address, token string) *host {
	return &host{w: w, address: address, token: token, calls: make(chan call, requestQueueLimit), tickets: map[string]viewerTicket{}, step: w.TickHeadless}
}

func (h *host) serve(parent context.Context, listener net.Listener) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	defer listener.Close()
	var sockets sync.Map
	var workers sync.WaitGroup
	acceptDone := make(chan struct{})
	defer workers.Wait()
	defer func() {
		cancel()
		listener.Close()
		<-acceptDone
		sockets.Range(func(k, v any) bool { _ = k.(net.Conn).Close(); return true })
	}()
	accepted := make(chan error, 1)
	go func() {
		defer close(acceptDone)
		slots := make(chan struct{}, connectionLimit)
		for {
			conn, err := listener.Accept()
			if err != nil {
				accepted <- err
				return
			}
			select {
			case slots <- struct{}{}:
			default:
				conn.Close()
				continue
			}
			sockets.Store(conn, true)
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-slots; sockets.Delete(conn); conn.Close() }()
				h.connection(ctx, agentwire.New(conn))
			}()
		}
	}()
	ticker := time.NewTicker(hostTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-accepted:
			if ctx.Err() != nil {
				return nil
			}
			return err
		case request := <-h.calls:
			if request.ctx.Err() != nil {
				request.result <- outcome{err: request.ctx.Err()}
				continue
			}
			value, err := request.run()
			var data []byte
			if err == nil {
				data, err = json.Marshal(value)
			}
			request.result <- outcome{data: data, err: err}
		case <-ticker.C:
			h.step()
		}
	}
}
func (h *host) do(ctx context.Context, fn func() (any, error)) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	p := call{ctx: ctx, run: fn, result: make(chan outcome, 1)}
	select {
	case h.calls <- p:
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, errors.New("native API busy")
	}
	select {
	case result := <-p.result:
		return result.data, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (h *host) session(id int) *app.Client {
	if id == 0 {
		return h.w.Current()
	}
	for _, s := range h.w.Sessions {
		if s.ID == id {
			return s.Client
		}
	}
	return nil
}
func identity() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (h *host) connection(ctx context.Context, conn *agentwire.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(authenticationTimeout))
	var hello json.RawMessage
	if conn.ReadJSON(&hello) != nil {
		return
	}
	var viewer struct {
		Ticket string `json:"ticket"`
		After  uint64 `json:"after"`
		Epoch  string `json:"epoch"`
	}
	_ = json.Unmarshal(hello, &viewer)
	if viewer.Ticket != "" {
		h.viewer(ctx, conn, viewer.Ticket, viewer.After, viewer.Epoch)
		return
	}
	var request Request
	if json.Unmarshal(hello, &request) != nil {
		return
	}
	var credentials struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(request.Params, &credentials)
	if request.Method != "auth" || subtle.ConstantTimeCompare([]byte(credentials.Token), []byte(h.token)) != 1 {
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	if conn.WriteJSON(Response{JSONRPC: "2.0", ID: request.ID, Result: "ok"}) != nil {
		return
	}
	for {
		_ = conn.SetReadDeadline(time.Time{})
		request = Request{}
		if conn.ReadJSON(&request) != nil {
			return
		}
		var result json.RawMessage
		var err error
		if request.Method == "agent.observe" {
			result, err = h.observe(ctx, request.Params)
		} else {
			result, err = h.do(ctx, func() (any, error) { return h.dispatch(request) })
		}
		response := Response{JSONRPC: "2.0", ID: request.ID, Result: result}
		if err != nil {
			response.Result = nil
			response.Error = &RPCError{Code: rpcOperationFailed, Message: err.Error()}
		}
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if conn.WriteJSON(response) != nil {
			return
		}
	}
}

func (h *host) dispatch(request Request) (any, error) {
	var p struct {
		ID      string          `json:"id"`
		Session int             `json:"session"`
		Request json.RawMessage `json:"request"`
		Ticket  string          `json:"ticket"`
		Kind    string          `json:"kind"`
		Entry   uint32          `json:"entry"`
	}
	if request.JSONRPC != "2.0" || json.Unmarshal(request.Params, &p) != nil || p.ID != "" && p.ID != nativeAgentID || p.Session < 0 {
		return nil, errors.New("invalid native API parameters")
	}
	if request.Method == "agent.execute" {
		var route struct {
			Session int `json:"session"`
		}
		if json.Unmarshal(p.Request, &route) != nil {
			return nil, errors.New("invalid command")
		}
		p.Session = route.Session
	}
	switch request.Method {
	case "agent.contract":
		return app.AgentContract(), nil
	case "agent.create":
		if len(h.w.Sessions) == 0 {
			if err := h.w.Add(); err != nil {
				return nil, err
			}
		}
		return map[string]any{"id": nativeAgentID, "sessions": h.w.Current().AgentSessions()}, nil
	case "agent.destroy":
		for len(h.w.Sessions) > 0 {
			h.w.Remove(h.w.Sessions[len(h.w.Sessions)-1].ID)
		}
		clear(h.tickets)
		return "ok", nil
	case "agent.list":
		sessions := []map[string]any{}
		if c := h.w.Current(); c != nil {
			sessions = c.AgentSessions()
		}
		return []any{map[string]any{"id": nativeAgentID, "sessions": sessions}}, nil
	}
	c := h.session(p.Session)
	if c == nil {
		return nil, errors.New("session unavailable")
	}
	if request.Method != "agent.state" && request.Method != "agent.execute" && request.Method != "agent.commands" && request.Method != "agent.cancel" {
		if err := c.AgentAccessError(); err != nil {
			return nil, err
		}
	}
	switch request.Method {
	case "agent.state":
		return c.AgentState(), nil
	case "agent.execute":
		return c.AgentSubmit(p.Request)
	case "agent.commands":
		return c.AgentCommandResults(), nil
	case "agent.cancel":
		return c.AgentCancel(p.Ticket), nil
	case "agent.lookup":
		return c.AgentLookup(p.Kind, p.Entry)
	case "agent.navigation":
		var query app.AgentNavigationRequest
		if json.Unmarshal(p.Request, &query) != nil {
			return nil, errors.New("invalid navigation query")
		}
		c = h.session(query.Session)
		if c == nil {
			return nil, errors.New("session unavailable")
		}
		return c.AgentNavigation(query)
	case "agent.viewer.open":
		if p.Session <= 0 {
			return nil, errors.New("viewer requires an explicit positive session ID")
		}
		for key, t := range h.tickets {
			if !time.Now().Before(t.expires) {
				delete(h.tickets, key)
			}
		}
		if len(h.tickets) >= viewerTicketLimit {
			return nil, errors.New("viewer ticket limit reached")
		}
		token, err := identity()
		if err != nil {
			return nil, err
		}
		h.tickets[token] = viewerTicket{session: p.Session, expires: time.Now().Add(viewerTicketLifetime)}
		return map[string]any{"url": fmt.Sprintf("tcp://%s/#%s", h.address, token), "session": p.Session, "expiresInSeconds": int(viewerTicketLifetime.Seconds())}, nil
	default:
		return nil, errors.New("unknown native API method")
	}
}
