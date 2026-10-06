package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
	"wonderland-gonline/internal/protocol"
)

func TestDuplicateLoginClosesOnlyRejectedSocket(t *testing.T) {
	s, players, wires := worldFixture(t)
	active := players[0]
	s.accounts[active.account.ID] = active.info.ID
	s.sessions[active.info.ID] = active
	host, client := net.Pipe()
	defer client.Close()
	rejected := &Session{conn: host, info: SessionInfo{ID: 3}, lastWindow: time.Now()}
	s.sessions[rejected.info.ID] = rejected
	done := make(chan struct{})
	go func() { s.serve(context.Background(), rejected); close(done) }()
	client.SetDeadline(time.Now().Add(10 * time.Second))
	if err := protocol.Write(client, loginPayload(active.account.Username)); err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{{63, 2}, {0, 19}} {
		got, err := protocol.Read(client)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("login rejection %x, %v; want %x", got, err, want)
		}
	}
	if _, err := protocol.Read(client); !errors.Is(err, io.EOF) {
		t.Fatal("rejected socket did not close", err)
	}
	<-done
	if rejected.account.ID != 0 || s.accounts[active.account.ID] != active.info.ID || s.sessions[active.info.ID] != active || wires[0].Len() != 0 {
		t.Fatal("duplicate login changed the active session")
	}
}

func TestConcurrentLoginReservesAccountOnce(t *testing.T) {
	s, players, _ := worldFixture(t)
	account := players[0].account
	var wg sync.WaitGroup
	sessions := []*Session{{conn: &captureConn{}, info: SessionInfo{ID: 3}}, {conn: &captureConn{}, info: SessionInfo{ID: 4}}}
	results := make([]error, len(sessions))
	for i, c := range sessions {
		wg.Add(1)
		go func() { defer wg.Done(); results[i] = s.login(context.Background(), c, loginPayload(account.Username)) }()
	}
	wg.Wait()
	winners, rejections := 0, 0
	for i, c := range sessions {
		switch {
		case results[i] == nil && c.account.ID == account.ID:
			winners++
		case errors.Is(results[i], errAlreadyLoggedIn) && c.account.ID == 0:
			rejections++
		default:
			t.Fatalf("unexpected login result %v, account %d", results[i], c.account.ID)
		}
	}
	if winners != 1 || rejections != 1 {
		t.Fatalf("winners=%d rejections=%d", winners, rejections)
	}
}
