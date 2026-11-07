package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func writeClientFrame(w io.Writer, opcode byte, payload []byte, masked bool) error {
	b0 := byte(0x80) | (opcode & 0x0F)
	maskBit := byte(0)
	if masked {
		maskBit = 0x80
	}

	var header []byte
	l := len(payload)
	if l < 126 {
		header = []byte{b0, maskBit | byte(l)}
	} else if l <= 65535 {
		header = make([]byte, 4)
		header[0] = b0
		header[1] = maskBit | 126
		binary.BigEndian.PutUint16(header[2:4], uint16(l))
	} else {
		header = make([]byte, 10)
		header[0] = b0
		header[1] = maskBit | 127
		binary.BigEndian.PutUint64(header[2:10], uint64(l))
	}

	if masked {
		maskKey := [4]byte{0x12, 0x34, 0x56, 0x78}
		header = append(header, maskKey[:]...)
		maskedPayload := make([]byte, l)
		for i := 0; i < l; i++ {
			maskedPayload[i] = payload[i] ^ maskKey[i%4]
		}
		_, err := w.Write(append(header, maskedPayload...))
		return err
	}

	_, err := w.Write(append(header, payload...))
	return err
}

func readServerFrame(r io.Reader) (byte, []byte, error) {
	var b [2]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, nil, err
	}
	opcode := b[0] & 0x0F
	masked := (b[1] & 0x80) != 0
	lenByte := b[1] & 0x7F

	var payloadLen int64
	switch lenByte {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return opcode, nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return opcode, nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint64(ext[:]))
	default:
		payloadLen = int64(lenByte)
	}

	if masked {
		var maskKey [4]byte
		if _, err := io.ReadFull(r, maskKey[:]); err != nil {
			return opcode, nil, err
		}
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return opcode, nil, err
	}
	return opcode, payload, nil
}

func startEchoServer(t *testing.T) (string, func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}

	stop := make(chan struct{})
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				select {
				case <-stop:
					return
				default:
					return
				}
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	cleanup := func() {
		close(stop)
		_ = l.Close()
	}

	return l.Addr().String(), cleanup
}

func performHandshake(t *testing.T, serverAddr, path string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", serverAddr, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to dial gateway: %v", err)
	}

	req := fmt.Sprintf(
		"GET %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"+
			"Sec-WebSocket-Version: 13\r\n\r\n",
		path, serverAddr,
	)

	if _, err := conn.Write([]byte(req)); err != nil {
		_ = conn.Close()
		t.Fatalf("failed to write handshake request: %v", err)
	}

	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		t.Fatalf("failed to read handshake response status: %v", err)
	}
	if !strings.Contains(statusLine, "101") {
		_ = conn.Close()
		t.Fatalf("expected 101 Switching Protocols, got: %s", statusLine)
	}

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			t.Fatalf("failed reading handshake headers: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
	}

	return conn
}

func TestGatewayEcho5Bytes(t *testing.T) {
	echoAddr, stopEcho := startEchoServer(t)
	defer stopEcho()

	cfg := Config{
		DefaultTarget:  echoAddr,
		ValidateTarget: func(target string) error { return nil },
	}

	ts := httptest.NewServer(NewGatewayHandler(cfg))
	defer ts.Close()

	serverAddr := ts.Listener.Addr().String()
	conn := performHandshake(t, serverAddr, "/ws")
	defer conn.Close()

	payload := []byte("hello")
	if err := writeClientFrame(conn, opcodeBinary, payload, true); err != nil {
		t.Fatalf("failed to write client frame: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	op, res, err := readServerFrame(conn)
	if err != nil {
		t.Fatalf("failed to read server echo: %v", err)
	}

	if op != opcodeBinary {
		t.Fatalf("expected opcode %d, got %d", opcodeBinary, op)
	}
	if !bytes.Equal(res, payload) {
		t.Fatalf("expected payload %q, got %q", payload, res)
	}
}

func TestGatewayEcho300Bytes(t *testing.T) {
	echoAddr, stopEcho := startEchoServer(t)
	defer stopEcho()

	cfg := Config{
		DefaultTarget:  echoAddr,
		ValidateTarget: func(target string) error { return nil },
	}

	ts := httptest.NewServer(NewGatewayHandler(cfg))
	defer ts.Close()

	serverAddr := ts.Listener.Addr().String()
	conn := performHandshake(t, serverAddr, "/ws")
	defer conn.Close()

	payload := make([]byte, 300)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("failed to generate random payload: %v", err)
	}

	if err := writeClientFrame(conn, opcodeBinary, payload, true); err != nil {
		t.Fatalf("failed to write 300-byte client frame: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	op, res, err := readServerFrame(conn)
	if err != nil {
		t.Fatalf("failed to read 300-byte server echo: %v", err)
	}

	if op != opcodeBinary {
		t.Fatalf("expected opcode %d, got %d", opcodeBinary, op)
	}
	if !bytes.Equal(res, payload) {
		t.Fatalf("300-byte payload mismatch")
	}
}

func TestGatewayRejectUnmaskedFrame(t *testing.T) {
	echoAddr, stopEcho := startEchoServer(t)
	defer stopEcho()

	cfg := Config{
		DefaultTarget:  echoAddr,
		ValidateTarget: func(target string) error { return nil },
	}

	ts := httptest.NewServer(NewGatewayHandler(cfg))
	defer ts.Close()

	serverAddr := ts.Listener.Addr().String()
	conn := performHandshake(t, serverAddr, "/ws")
	defer conn.Close()

	payload := []byte("unmasked data")
	if err := writeClientFrame(conn, opcodeBinary, payload, false); err != nil {
		t.Fatalf("failed to write unmasked frame: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	op, res, err := readServerFrame(conn)
	if err != nil {
		return
	}

	if op != opcodeClose {
		t.Fatalf("expected close frame (opcode 8), got opcode %d", op)
	}
	if len(res) >= 2 {
		code := binary.BigEndian.Uint16(res[:2])
		if code != closeProtocolError {
			t.Fatalf("expected close code %d (protocol error), got %d", closeProtocolError, code)
		}
	}
}

func TestGatewayDisallowedTarget(t *testing.T) {
	cfg := Config{
		DefaultTarget:  "127.0.0.1:6414",
		ValidateTarget: DefaultValidateTarget,
	}

	ts := httptest.NewServer(NewGatewayHandler(cfg))
	defer ts.Close()

	client := ts.Client()

	tests := []struct {
		name   string
		target string
	}{
		{"remote host", "8.8.8.8:6414"},
		{"disallowed port low", "127.0.0.1:5999"},
		{"disallowed port high", "127.0.0.1:7000"},
		{"invalid format", "not-a-port"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := fmt.Sprintf("%s/ws?target=%s", ts.URL, tc.target)
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				t.Fatalf("NewRequest failed: %v", err)
			}
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Sec-WebSocket-Version", "13")
			req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected status 400 Bad Request, got %d", resp.StatusCode)
			}
		})
	}
}

func TestGatewayHealthz(t *testing.T) {
	cfg := Config{
		DefaultTarget:  "127.0.0.1:6414",
		ValidateTarget: DefaultValidateTarget,
	}

	ts := httptest.NewServer(NewGatewayHandler(cfg))
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("expected 'ok', got %q", string(body))
	}
}
