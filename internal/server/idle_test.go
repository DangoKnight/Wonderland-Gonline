package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"wonderland-go/internal/assets"
	"wonderland-go/internal/config"
	"wonderland-go/internal/game"
)

func TestIdleDeadlineTransitions(t *testing.T) {
	cfg := config.Default()
	s := New(cfg, nil, &assets.Catalog{}, slog.Default())
	c := &Session{}
	now := time.Date(2026, time.October, 3, 0, 0, 0, 0, time.UTC)
	if got := s.idleReadDeadline(c, now); !got.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("login deadline = %v", got)
	}
	// A pending creation name remains in the login phase.
	c.pendingName = "New character"
	if got := s.idleReadDeadline(c, now); !got.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("creation deadline = %v", got)
	}
	c.character = &game.Character{}
	for _, ready := range []bool{false, true} {
		c.ready = ready
		if got := s.idleReadDeadline(c, now); !got.IsZero() {
			t.Fatalf("gameplay ready=%v retained login deadline: %v", ready, got)
		}
	}
	s.Config.WorldIdleSeconds = 45
	if got := s.idleReadDeadline(c, now); !got.Equal(now.Add(45 * time.Second)) {
		t.Fatalf("configured gameplay deadline = %v", got)
	}
	c.character = nil
	if got := s.idleReadDeadline(c, now); !got.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("return to login deadline = %v", got)
	}
	s.Config.IdleSeconds = 0
	if got := s.idleReadDeadline(c, now); !got.IsZero() {
		t.Fatalf("disabled login deadline = %v", got)
	}
}

// Accelerate nonzero read deadlines without waiting minutes in socket tests.
// Zero deadlines still reach the real connection unchanged.
type acceleratedIdleConn struct{ net.Conn }

func (c acceleratedIdleConn) SetReadDeadline(deadline time.Time) error {
	if !deadline.IsZero() {
		deadline = time.Now().Add(30 * time.Millisecond)
	}
	return c.Conn.SetReadDeadline(deadline)
}

func TestIdleSocketPolicy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		world   bool
		seconds int
		expires bool
	}{
		{"login expires", false, 600, true},
		{"login disabled", false, 0, false},
		{"gameplay disabled", true, 0, false},
		{"gameplay expires", true, 60, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			if tc.world {
				cfg.WorldIdleSeconds = tc.seconds
			} else {
				cfg.IdleSeconds = tc.seconds
			}
			s := New(cfg, nil, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			client, host := net.Pipe()
			// An inherited login deadline must be cleared when timeout is disabled.
			if err := host.SetReadDeadline(time.Now().Add(-time.Second)); err != nil {
				t.Fatal(err)
			}
			c := &Session{conn: acceleratedIdleConn{host}, info: SessionInfo{ID: 1}}
			if tc.world {
				c.character = &game.Character{}
			}
			s.sessions[1] = c
			done := make(chan struct{})
			go func() { s.serve(context.Background(), c); close(done) }()
			t.Cleanup(func() {
				client.Close()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Error("socket cleanup timed out")
				}
			})
			if tc.expires {
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("idle connection did not expire")
				}
			} else {
				select {
				case <-done:
					t.Fatal("disabled idle timeout closed connection")
				case <-time.After(100 * time.Millisecond):
				}
			}
		})
	}
}
