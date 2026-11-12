package headless

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"wonderland-gonline/client/wlo/agentwire"
	"wonderland-gonline/client/wlo/app"
	"wonderland-gonline/client/wlo/sessionstream"
)

const nativeViewerUpdatesPerSecond = 30

func (h *host) viewer(parent context.Context, conn *agentwire.Conn, token string, after uint64, epoch string) {
	var ticket viewerTicket
	_, err := h.do(parent, func() (any, error) {
		var ok bool
		ticket, ok = h.tickets[token]
		if !ok || !time.Now().Before(ticket.expires) || h.session(ticket.session) == nil {
			return nil, errors.New("invalid viewer ticket")
		}
		return true, nil
	})
	if err != nil {
		return
	}
	viewer, err := identity()
	if err != nil {
		return
	}
	submit := func(ctx context.Context, p app.AgentHostRequest) error {
		p.Session, p.Viewer = ticket.session, viewer
		_, err := h.do(ctx, func() (any, error) {
			c := h.session(ticket.session)
			if c == nil {
				return nil, errors.New("session unavailable")
			}
			return c.AgentHostSubmit(p)
		})
		return err
	}
	if submit(parent, app.AgentHostRequest{Operation: "attach"}) != nil {
		return
	}
	defer func() { _ = submit(parent, app.AgentHostRequest{Operation: "detach"}) }()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	readerDone := make(chan struct{})
	defer func() { conn.Close(); <-readerDone }()
	go func() {
		defer close(readerDone)
		defer cancel()
		for {
			_ = conn.SetReadDeadline(time.Now().Add(viewerReadTimeout))
			var input app.AgentHostRequest
			if conn.ReadJSON(&input) != nil {
				return
			}
			switch input.Operation {
			case "ping":
				continue
			case "takeover", "release", "heartbeat", "input":
			default:
				return
			}
			if submit(ctx, input) != nil {
				return
			}
		}
	}()
	tick := time.NewTicker(time.Second / nativeViewerUpdatesPerSecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if !time.Now().Before(ticket.expires) {
			return
		}
		data, err := h.do(ctx, func() (any, error) {
			c := h.session(ticket.session)
			if c == nil {
				return nil, errors.New("session unavailable")
			}
			batch, err := c.AgentNativeStream(after)
			if err != nil {
				return nil, err
			}
			if epoch != "" && epoch != batch.Epoch {
				after = 0
				batch, err = c.AgentNativeStream(0)
			}
			epoch = batch.Epoch
			return struct {
				Native  any                  `json:"native"`
				Control app.AgentHostControl `json:"control"`
			}{batch, c.AgentHostState()}, err
		})
		if err != nil {
			if errors.Is(err, app.ErrAgentAccessDenied) {
				_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
				_ = conn.WriteJSON(map[string]any{"native": sessionstream.Batch{Version: sessionstream.Version, Error: err.Error()}, "viewer": viewer})
			}
			return
		}
		var frame struct {
			Native  sessionstream.Batch  `json:"native"`
			Control app.AgentHostControl `json:"control"`
			Viewer  string               `json:"viewer"`
		}
		if json.Unmarshal(data, &frame) != nil {
			return
		}
		frame.Viewer = viewer
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if conn.WriteJSON(frame) != nil {
			return
		}
		after = sessionstream.Delivered(frame.Native, after)
	}
}
