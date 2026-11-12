// Package viewer transports hosted presentation and exclusive human input.
// Windowing, rendering, preferences and session UI belong to the normal client.
package viewer

import (
	"context"
	"errors"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"wonderland-gonline/client/wlo/agentwire"
	"wonderland-gonline/client/wlo/sessionstream"
)

const (
	sceneWidth  = 800
	sceneHeight = 600

	viewerInputBuffer    = 64
	viewerFrameBytes     = 16 << 20
	viewerNativeQueue    = 8
	viewerInputLagFrames = 6
	viewerHeartbeat      = time.Second
	viewerReconnect      = time.Second
	viewerTimeout        = 5 * time.Second
)

type control struct {
	Owner      string `json:"owner"`
	Viewer     string `json:"viewer"`
	Generation uint64 `json:"generation"`
}
type frame struct {
	Sequence uint64               `json:"sequence"`
	Native   *sessionstream.Batch `json:"native"`
	Control  control              `json:"control"`
	Viewer   string               `json:"viewer"`
}
type outbound struct {
	data       map[string]any
	connection uint64
}
type Connection struct {
	mu             sync.Mutex
	control        control
	viewer         string
	connected      bool
	connection     uint64
	disconnect     context.CancelFunc
	native         chan sessionstream.Batch
	applied        uint64
	head           uint64
	epoch          string
	nativeError    string
	cancel         context.CancelFunc
	done           chan struct{}
	input          chan outbound
	sequence       uint64
	lastGeneration uint64
}

type Status struct {
	Connected bool
	Owns      bool
	Owner     string
	Ready     bool
	Error     string
}

// State is safe for sidebar queries without consuming presentation events.
func (g *Connection) State() Status {
	g.mu.Lock()
	defer g.mu.Unlock()
	ready := g.applied >= g.head || g.head-g.applied <= viewerInputLagFrames
	return Status{Connected: g.connected, Owns: g.connected && g.control.Owner == "viewer" && g.control.Viewer == g.viewer, Owner: g.control.Owner, Ready: g.nativeError == "" && ready, Error: g.nativeError}
}

// PollNative transfers ordered presentation events to the normal game loop.
func (g *Connection) PollNative() (sessionstream.Batch, bool) {
	select {
	case batch := <-g.native:
		return batch, true
	default:
		return sessionstream.Batch{}, false
	}
}
func (g *Connection) Acknowledge(sequence uint64, epoch string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.applied = sequence
	g.epoch = epoch
}
func (g *Connection) ReportNativeError(message string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nativeError = message
}
func (g *Connection) Input(kind string, fields map[string]any) {
	if kind != "reset" && !g.State().Ready {
		return
	}
	if fields == nil {
		fields = map[string]any{}
	}
	fields["operation"], fields["kind"] = "input", kind
	g.send(fields, true)
}
func (g *Connection) ResetInput() { g.Input("reset", nil) }
func (g *Connection) ToggleControl() {
	state := g.State()
	if !state.Ready {
		return
	}
	if state.Owns {
		g.send(map[string]any{"operation": "release"}, false)
	} else if state.Connected && state.Owner == "api" {
		g.send(map[string]any{"operation": "takeover"}, false)
	}
}
func (g *Connection) Close() {
	if g.cancel != nil {
		g.cancel()
		<-g.done
	}
}

func New(address string) (*Connection, error) {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" || (u.Scheme != "tcp" && u.Scheme != "http" && u.Scheme != "https") || u.Fragment == "" || u.User != nil {
		return nil, errors.New("attach requires a complete tcp://, http:// or https:// viewer URL including its ticket fragment")
	}
	ticket := u.Fragment
	u.Fragment = ""
	u.RawQuery = ""
	u.Path = "/viewer/ws"
	if u.Scheme == "tcp" {
		u.Path = "/"
	} else if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	ctx, cancel := context.WithCancel(context.Background())
	g := &Connection{input: make(chan outbound, viewerInputBuffer), native: make(chan sessionstream.Batch, viewerNativeQueue), cancel: cancel, done: make(chan struct{})}
	go func() { defer close(g.done); g.connect(ctx, u.String(), ticket) }()
	return g, nil
}
func (g *Connection) connect(ctx context.Context, address, ticket string) {
	for ctx.Err() == nil {
		g.connectionLoop(ctx, address, ticket)
		g.mu.Lock()
		terminal := g.nativeError != ""
		g.mu.Unlock()
		if terminal {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(viewerReconnect):
		}
	}
}

type viewerConnection interface {
	ReadJSON(any) error
	WriteJSON(any) error
	SetReadLimit(int64)
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
	Close() error
}

func dialViewer(ctx context.Context, address string) (viewerConnection, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "tcp" {
		conn, err := (&net.Dialer{Timeout: viewerTimeout}).DialContext(ctx, "tcp", u.Host)
		if err != nil {
			return nil, err
		}
		return agentwire.New(conn), nil
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, address, nil)
	return conn, err
}
func (g *Connection) connectionLoop(ctx context.Context, address, ticket string) {
	conn, err := dialViewer(ctx, address)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(viewerTimeout))
	g.mu.Lock()
	after, epoch := g.applied, g.epoch
	g.mu.Unlock()
	if conn.WriteJSON(map[string]any{"ticket": ticket, "native": true, "after": after, "epoch": epoch}) != nil {
		return
	}
	local, cancel := context.WithCancel(ctx)
	defer cancel()
	g.mu.Lock()
	g.connection++
	identity := g.connection
	g.disconnect = cancel
	g.mu.Unlock()
	defer func() { g.mu.Lock(); g.connected = false; g.control = control{}; g.mu.Unlock() }()
	go func() { <-local.Done(); _ = conn.Close() }()
	go func() {
		defer cancel()
		conn.SetReadLimit(viewerFrameBytes)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(viewerTimeout))
			var f frame
			if conn.ReadJSON(&f) != nil {
				return
			}
			if f.Native == nil || f.Native.Version != sessionstream.Version {
				g.ReportNativeError("host does not support native spectator data")
				return
			}
			g.mu.Lock()
			if g.connection != identity || local.Err() != nil {
				g.mu.Unlock()
				return
			}
			g.connected = true
			g.control = f.Control
			g.viewer = f.Viewer
			g.head = f.Native.Head
			g.mu.Unlock()
			if f.Native.Error != "" {
				g.ReportNativeError(f.Native.Error)
				return
			}
			if len(f.Native.Chunks) > 0 || len(f.Native.Tail) > 0 || epoch == "" {
				select {
				case g.native <- *f.Native:
				case <-local.Done():
					return
				}
				epoch = f.Native.Epoch
			}
		}
	}()
	tick := time.NewTicker(viewerHeartbeat)
	defer tick.Stop()
	for {
		var data map[string]any
		select {
		case <-local.Done():
			return
		case event := <-g.input:
			if event.connection != identity {
				continue
			}
			data = event.data
		case <-tick.C:
			g.mu.Lock()
			s, v, connected := g.control, g.viewer, g.connected
			g.mu.Unlock()
			if !connected {
				continue
			}
			operation := "ping"
			if s.Owner == "viewer" && s.Viewer == v {
				operation = "heartbeat"
			}
			data = map[string]any{"operation": operation, "generation": s.Generation}
		}
		_ = conn.SetWriteDeadline(time.Now().Add(viewerTimeout))
		if conn.WriteJSON(data) != nil {
			return
		}
	}
}
func (g *Connection) send(data map[string]any, owner bool) {
	g.mu.Lock()
	s, v, connected, connection := g.control, g.viewer, g.connected, g.connection
	g.mu.Unlock()
	if !connected || owner && (s.Owner != "viewer" || s.Viewer != v) {
		return
	}
	if s.Generation != g.lastGeneration {
		g.sequence = 0
		g.lastGeneration = s.Generation
	}
	g.sequence++
	data["generation"] = s.Generation
	data["sequence"] = g.sequence
	select {
	case g.input <- outbound{data, connection}:
	default:
		// Losing a key-up must never leave a held input behind. Reconnection
		// revokes control and starts readonly instead of replaying the backlog.
		g.mu.Lock()
		g.connected = false
		cancel := g.disconnect
		g.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		for {
			select {
			case <-g.input:
			default:
				return
			}
		}
	}
}
