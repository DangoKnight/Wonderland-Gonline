package server

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/protocol"
)

func TestRejectedCommandIsVisibleWithoutCredentials(t *testing.T) {
	var logs bytes.Buffer
	s := New(config.Default(), nil, &assets.Catalog{}, slog.New(slog.NewJSONHandler(&logs, nil)))
	client, host := net.Pipe()
	defer client.Close()
	c := &Session{conn: host, info: SessionInfo{ID: 1, Service: "world"}, lastWindow: time.Now()}
	done := make(chan struct{})
	go func() { s.serve(context.Background(), c); close(done) }()
	client.SetDeadline(time.Now().Add(5 * time.Second))
	// An unauthenticated creation command must be rejected without exposing its body.
	payload := append([]byte{9, 1}, []byte("secret-login-and-deletion-password")...)
	if err := protocol.Write(client, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.Read(client); err != io.EOF {
		t.Fatalf("expected disconnect, got %v", err)
	}
	<-done
	output := logs.String()
	for _, expected := range []string{`"level":"WARN"`, `"msg":"client command rejected"`, `"action":9`, `"code":1`, `"map_ready":false`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %s in %s", expected, output)
		}
	}
	if strings.Contains(output, "secret-login") {
		t.Fatal("credential payload leaked into logs")
	}
}

func TestPacketTraceContainsOnlyMetadata(t *testing.T) {
	var logs bytes.Buffer
	wire := &captureConn{}
	c := &Session{conn: wire, log: slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	if err := c.send(append([]byte{63, 4}, []byte("private-password")...)); err != nil {
		t.Fatal(err)
	}
	output := logs.String()
	if !strings.Contains(output, `"msg":"packet sent"`) || !strings.Contains(output, `"action":63`) || !strings.Contains(output, `"code":4`) {
		t.Fatal(output)
	}
	if strings.Contains(output, "private-password") {
		t.Fatal("credential payload leaked into trace")
	}
}
