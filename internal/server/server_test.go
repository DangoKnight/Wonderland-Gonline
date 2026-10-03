package server

import (
	"bytes"
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

func TestLoginWireFlow(t *testing.T) {
	db, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	a, e := db.Register(context.Background(), "tester", "password", "")
	if e != nil {
		t.Fatal(e)
	}
	s := New(config.Default(), db, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	client, host := net.Pipe()
	defer client.Close()
	c := &Session{conn: host, info: SessionInfo{ID: 1}, lastWindow: time.Now()}
	s.sessions[1] = c
	done := make(chan struct{})
	go func() { s.serve(context.Background(), c); close(done) }()
	defer func() { client.Close(); <-done }()
	client.SetDeadline(time.Now().Add(10 * time.Second))
	send := func(p []byte) {
		t.Helper()
		if e := protocol.Write(client, p); e != nil {
			t.Fatal(e)
		}
	}
	read := func() []byte {
		t.Helper()
		p, e := protocol.Read(client)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	send([]byte{0})
	if p := read(); !bytes.Equal(p, append([]byte{1, 9, 101, 0, 1}, []byte("Wonderland Go")...)) {
		t.Fatalf("%x", p)
	}
	if p := read(); p[0] != 54 || p[1] != 201 {
		t.Fatal(p)
	}
	login, _ := protocol.Builder{63, 4}.U16(1205).String("tester")
	login, _ = login.String("password")
	send(login)
	if p := read(); !bytes.Equal(p, protocol.Builder{63, 2}.U32(a.UserID())) {
		t.Fatalf("login response %x", p)
	}
	if p := read(); !bytes.Equal(p, []byte{63, 1}) {
		t.Fatal(p)
	}
	if p := read(); !bytes.Equal(p, []byte{35, 11}) {
		t.Fatal(p)
	}
	send([]byte{63, 2, 1})
	if p := read(); !bytes.Equal(p, []byte{1, 3, 0}) {
		t.Fatal(p)
	}
}
func TestUnauthenticatedSelection(t *testing.T) {
	s := New(config.Default(), nil, &assets.Catalog{}, slog.Default())
	if e := s.dispatch(context.Background(), &Session{}, []byte{63, 2, 1}); e == nil {
		t.Fatal("unauthenticated character selection")
	}
	if e := s.dispatch(context.Background(), &Session{}, []byte{63, 4, 10, 'x'}); e == nil {
		t.Fatal("truncated login")
	}
}
func TestLauncherStatusGolden(t *testing.T) {
	for _, tc := range []struct {
		n     int
		color byte
	}{{0, 1}, {9, 1}, {10, 2}, {29, 2}, {30, 3}} {
		if got := StatusPacket(tc.n); !bytes.Equal(got, []byte{0xc9, 0, 1, 1, 0, tc.color, 101, 0, tc.color}) {
			t.Fatalf("%x", got)
		}
	}
}

func TestConfiguredLauncherStatusGolden(t *testing.T) {
	// Dango Island uses region flag 3: its first row is status ID 301 (0x012d).
	for _, tc := range []struct {
		online int
		color  byte
	}{{0, 1}, {10, 2}, {30, 3}} {
		want := []byte{0xc9, 0, 1, 1, 0, tc.color, 101, 0, tc.color, 0x2d, 1, tc.color}
		if got := StatusPacket(tc.online, 1, 101, 301); !bytes.Equal(got, want) {
			t.Fatalf("configured status %x, want %x", got, want)
		}
	}
}

func TestStatusServiceUsesConfiguredServerIDs(t *testing.T) {
	c := config.Default()
	c.StatusServerIDs = []uint16{301}
	s := New(c, nil, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	client, host := net.Pipe()
	defer client.Close()
	session := &Session{conn: host, info: SessionInfo{ID: 1, Service: "status"}}
	done := make(chan struct{})
	go func() { s.serve(context.Background(), session); close(done) }()
	client.SetDeadline(time.Now().Add(5 * time.Second))
	got, err := io.ReadAll(client)
	<-done
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{0xc9, 0, 1, 0x2d, 1, 1}) {
		t.Fatalf("status endpoint %x", got)
	}
}
