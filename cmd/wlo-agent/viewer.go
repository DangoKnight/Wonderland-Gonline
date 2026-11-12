package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"wonderland-gonline/internal/protocol"
)

const (
	viewerTicketLifetime        = time.Hour
	viewerFrameInterval         = 125 * time.Millisecond
	viewerWriteTimeout          = 3 * time.Second
	viewerReadTimeout           = 10 * time.Second
	viewerMessageLimit          = 8192
	viewerNativeProtocolVersion = 1 // Matches client/wlo/sessionstream.Version.
	viewerTicketLimit           = 128
)

type viewerTicket struct {
	Agent   string
	Session int
	Expires time.Time
}
type viewerRegistry struct {
	mu      sync.Mutex
	tickets map[string]viewerTicket
}

func viewerIdentity() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func (m *Manager) openViewer(id string, session int) (any, error) {
	if _, ok := m.Get(id); !ok || session <= 0 {
		return nil, errors.New("agent or explicit session unavailable")
	}
	if _, err := m.StructuredAPI(id, "state", json.RawMessage(jsonNumber(session))); err != nil {
		return nil, err
	}
	token, err := viewerIdentity()
	if err != nil {
		return nil, err
	}
	r := &m.viewers
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tickets == nil {
		r.tickets = map[string]viewerTicket{}
	}
	for k, t := range r.tickets {
		if !time.Now().Before(t.Expires) {
			delete(r.tickets, k)
		}
	}
	if len(r.tickets) >= viewerTicketLimit {
		return nil, errors.New("viewer ticket limit reached")
	}
	r.tickets[token] = viewerTicket{Agent: id, Session: session, Expires: time.Now().Add(viewerTicketLifetime)}
	return map[string]any{"path": "/viewer/#" + token, "expiresInSeconds": int(viewerTicketLifetime.Seconds()), "session": session}, nil
}
func jsonNumber(n int) []byte { b, _ := json.Marshal(n); return b }
func (m *Manager) installViewer(mux *http.ServeMux) {
	files, _ := fs.Sub(hostFS, "host")
	mux.Handle("/ui/", http.FileServer(http.FS(files)))
	mux.Handle("/wasm_exec.js", http.FileServer(http.FS(files)))
	mux.HandleFunc("/viewer/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		data, err := fs.ReadFile(files, "index.html")
		if err != nil {
			http.Error(w, "viewer unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/viewer/ws", m.viewerSocket)
}
func (m *Manager) viewerSocket(w http.ResponseWriter, r *http.Request) {
	// Gorilla's default origin check rejects cross-origin browser connections.
	conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(viewerMessageLimit)
	_ = conn.SetReadDeadline(time.Now().Add(viewerReadTimeout))
	var hello struct {
		Ticket string `json:"ticket"`
		Native bool   `json:"native"`
		After  uint64 `json:"after"`
		Epoch  string `json:"epoch"`
	}
	if conn.ReadJSON(&hello) != nil {
		return
	}
	m.viewers.mu.Lock()
	ticket, ok := m.viewers.tickets[hello.Ticket]
	m.viewers.mu.Unlock()
	if !ok || !time.Now().Before(ticket.Expires) {
		return
	}
	identity, err := viewerIdentity()
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	agent, exists := m.Get(ticket.Agent)
	if !exists {
		return
	}
	// CDP requires the browser's context; tie cancellation to the viewer
	// without replacing the browser executor with an HTTP request context.
	frameContext, stopFrames := context.WithCancel(agent.ctx)
	stopViewer := context.AfterFunc(ctx, stopFrames)
	defer stopViewer()
	defer stopFrames()
	submit := func(p map[string]any) error {
		p["viewer"] = identity
		p["session"] = ticket.Session
		b, _ := json.Marshal(p)
		_, err := m.StructuredAPI(ticket.Agent, "hostControl", b)
		return err
	}
	defer func() { _ = submit(map[string]any{"operation": "detach"}) }()
	if err := submit(map[string]any{"operation": "attach"}); err != nil {
		return
	}
	go func() {
		defer cancel()
		for {
			var input map[string]any
			_ = conn.SetReadDeadline(time.Now().Add(viewerReadTimeout))
			if conn.ReadJSON(&input) != nil {
				return
			}
			operation, _ := input["operation"].(string)
			switch operation {
			case "ping":
				continue
			case "takeover", "release", "heartbeat", "input":
			default:
				return
			}
			if err := submit(input); err != nil {
				return
			}
		}
	}()
	interval := viewerFrameInterval
	if hello.Native {
		interval = time.Second / 30
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var previous uint64
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if !time.Now().Before(ticket.Expires) {
			return
		}
		method, argument := "viewerFrame", json.RawMessage(jsonNumber(ticket.Session))
		if hello.Native {
			method = "nativeSync"
			argument, _ = json.Marshal(map[string]any{"session": ticket.Session, "after": hello.After, "epoch": hello.Epoch})
		}
		frame, err := m.structuredAPI(frameContext, ticket.Agent, method, argument)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("viewer frame unavailable: %v", err)
				if strings.Contains(err.Error(), protocol.AgentAccessDeniedMessage) {
					refusal := map[string]any{"error": protocol.AgentAccessDeniedMessage}
					if hello.Native {
						refusal["native"] = map[string]any{"version": viewerNativeProtocolVersion, "error": protocol.AgentAccessDeniedMessage}
					}
					_ = conn.SetWriteDeadline(time.Now().Add(viewerWriteTimeout))
					_ = conn.WriteJSON(refusal)
				}
			}
			return
		}
		var value map[string]json.RawMessage
		if json.Unmarshal(frame, &value) != nil {
			return
		}
		var sequence uint64
		_ = json.Unmarshal(value["sequence"], &sequence)
		if !hello.Native && sequence == previous {
			delete(value, "jpeg")
		} else {
			previous = sequence
		}
		value["viewer"], _ = json.Marshal(identity)
		_ = conn.SetWriteDeadline(time.Now().Add(viewerWriteTimeout))
		if conn.WriteJSON(value) != nil {
			return
		}
		if hello.Native {
			hello.After, hello.Epoch = nativeCursor(value["native"], hello.After, hello.Epoch)
		}
	}
}

func nativeCursor(raw json.RawMessage, after uint64, epoch string) (uint64, string) {
	var p struct {
		Epoch     string            `json:"epoch"`
		TailFirst uint64            `json:"tailFirst"`
		Tail      []json.RawMessage `json:"tail"`
		Chunks    []struct {
			First uint64 `json:"first"`
			Count int    `json:"count"`
		} `json:"chunks"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return after, epoch
	}
	if len(p.Tail) > 0 {
		after = p.TailFirst + uint64(len(p.Tail)) - 1
	} else if len(p.Chunks) > 0 {
		last := p.Chunks[len(p.Chunks)-1]
		after = last.First + uint64(last.Count) - 1
	}
	return after, p.Epoch
}
