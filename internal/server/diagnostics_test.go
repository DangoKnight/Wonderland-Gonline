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
	"wonderland-go/internal/assets"
	"wonderland-go/internal/config"
	"wonderland-go/internal/protocol"
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

func TestLuckyDrawTraceDecodedCatalogAndResult(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Debug("request", packetTraceAttrs([]byte{104, 1, 7}, false)...)
	logger.Debug("result", packetTraceAttrs([]byte{104, 1, 2, 8, 1}, true)...)
	logger.Debug("catalog", packetTraceAttrs([]byte{104, 1, 1, 2, 1, 176, 125, 2}, true)...)
	for _, want := range []string{`"payload_hex":"680107"`, `"lucky_draw_intent":"spin"`, `"expected_request_bytes":2`, `"payload_hex":"6801020801"`, `"lucky_draw_used":2`, `"reward_count":1`} {
		if !strings.Contains(logs.String(), want) {
			t.Fatal("missing diagnostic", want, logs.String())
		}
	}
}

func TestLuckyDrawTraceKeepsSensitivePacketsPrivateAndBoundsMalformedDraws(t *testing.T) {
	for _, sent := range []bool{false, true} {
		for _, prefix := range [][]byte{{63, 4}, {9, 1}, {35, 1}, {35, 12}, {2, 1}, {14, 1}} {
			attrs := packetTraceAttrs(append(prefix, []byte("private-password-or-message")...), sent)
			for i := 0; i < len(attrs); i += 2 {
				if attrs[i] == "payload_hex" {
					t.Fatal("sensitive packet exposed", prefix, sent)
				}
			}
		}
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	payload := append([]byte{104, 99}, bytes.Repeat([]byte{255}, 100)...)
	logger.Debug("request", packetTraceAttrs(payload, false)...)
	if !strings.Contains(logs.String(), `"payload_truncated":true`) || strings.Contains(logs.String(), strings.Repeat("ff", 100)) {
		t.Fatal("unbounded malformed trace", logs.String())
	}
}

func TestNativePoseTraceShowsOwnerAndExpression(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Debug("request", packetTraceAttrs([]byte{32, 1, 9}, false)...)
	logger.Debug("reply", packetTraceAttrs([]byte{32, 1, 17, 39, 0, 0, 9}, true)...)
	for _, want := range []string{`"payload_hex":"200109"`, `"payload_hex":"20011127000009"`} {
		if !strings.Contains(logs.String(), want) {
			t.Fatal("missing expression trace", logs.String())
		}
	}
	logger.Debug("malformed", packetTraceAttrs(append([]byte{32, 1}, bytes.Repeat([]byte{255}, 100)...), false)...)
	if !strings.Contains(logs.String(), `"payload_truncated":true`) || strings.Contains(logs.String(), strings.Repeat("ff", 100)) {
		t.Fatal("unbounded expression trace", logs.String())
	}
}

func TestNativeMovementTraceBoundsPayload(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Debug("movement", packetTraceAttrs([]byte{6, 2, 255, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, false)...)
	if !strings.Contains(logs.String(), `"payload_hex":"0602ff0102030405060708090a0b0c"`) {
		t.Fatal("missing AC6:2 trace", logs.String())
	}
	logger.Debug("oversized", packetTraceAttrs(append([]byte{6, 2}, bytes.Repeat([]byte{255}, 100)...), false)...)
	if !strings.Contains(logs.String(), `"payload_truncated":true`) || strings.Contains(logs.String(), strings.Repeat("ff", 100)) {
		t.Fatal("unbounded movement trace", logs.String())
	}
}

func TestPotentialPillTraceBoundsPayload(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Debug("request", packetTraceAttrs(append([]byte{23, 126, 50, 2}, bytes.Repeat([]byte{255}, 100)...), false)...)
	logger.Debug("reply", packetTraceAttrs([]byte{23, 213, 1, 2, 12}, true)...)
	for _, want := range []string{`"payload_hex":"177e3202"`, `"payload_truncated":true`, `"payload_hex":"17d501020c"`} {
		if !strings.Contains(logs.String(), want) {
			t.Fatal(want, logs.String())
		}
	}
	if strings.Contains(logs.String(), "ffffffff") {
		t.Fatal("unbounded potential trace")
	}
}
