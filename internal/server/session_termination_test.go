package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/config"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

func TestTerminateSessionReleasesAccountForReconnect(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := db.Register(ctx, "tester", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	s := New(config.Default(), db, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	connect := func(id uint64) (net.Conn, <-chan struct{}) {
		client, host := net.Pipe()
		c := &Session{conn: host, info: SessionInfo{ID: id}, lastWindow: time.Now()}
		s.mu.Lock()
		s.sessions[id] = c
		s.mu.Unlock()
		done := make(chan struct{})
		go func() { s.serve(ctx, c); close(done) }()
		t.Cleanup(func() {
			client.Close()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("session cleanup timed out")
			}
		})
		client.SetDeadline(time.Now().Add(5 * time.Second))
		p, _ := protocol.Builder{63, 4}.String("tester")
		p, _ = p.String("password")
		if err := protocol.Write(client, p); err != nil {
			t.Fatal(err)
		}
		for _, prefix := range []byte{63, 63, 35} {
			p, err := protocol.Read(client)
			if err != nil || p[0] != prefix {
				t.Fatal("login", p, err)
			}
		}
		return client, done
	}
	client, done := connect(1)
	if !s.Kick(1) {
		t.Fatal("connected session not terminated")
	}
	if _, err := protocol.Read(client); err == nil {
		t.Fatal("terminated connection still readable")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("termination did not clean up")
	}
	if s.Kick(1) || len(s.Sessions()) != 0 {
		t.Fatal("terminated session remained published")
	}
	if got, err := db.Authenticate(ctx, "tester", "password"); err != nil || got.ID != account.ID {
		t.Fatal("termination changed account", err)
	}
	_, done = connect(2)
	if !s.Kick(2) {
		t.Fatal("reconnected session missing")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reconnected session cleanup timed out")
	}
}
