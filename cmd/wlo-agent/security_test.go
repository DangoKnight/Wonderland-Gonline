package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRPCAuthenticationAndOrigin(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"
	s := httptest.NewServer(rpcHandler(NewManager("", "", true), token))
	defer s.Close()
	for _, test := range []struct {
		name, token, origin string
		status              int
	}{
		{"missing token", "", "", http.StatusUnauthorized},
		{"wrong token", strings.Repeat("x", len(token)), "", http.StatusUnauthorized},
		{"foreign browser", token, "https://unrelated.example", http.StatusForbidden},
		{"CLI", token, "", http.StatusSwitchingProtocols},
		{"same origin", token, s.URL, http.StatusSwitchingProtocols},
	} {
		t.Run(test.name, func(t *testing.T) {
			header := http.Header{}
			if test.token != "" {
				header.Set("Authorization", "Bearer "+test.token)
			}
			if test.origin != "" {
				header.Set("Origin", test.origin)
			}
			conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(s.URL, "http"), header)
			if response == nil || response.StatusCode != test.status {
				t.Fatalf("response=%v error=%v", response, err)
			}
			if test.status != http.StatusSwitchingProtocols {
				if conn != nil {
					conn.Close()
					t.Fatal("unauthorized upgrade")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			if err := conn.WriteJSON(Request{JSONRPC: "2.0", Method: "agent.state", Params: []byte(`{"id":"missing"}`), ID: 1}); err != nil {
				t.Fatal(err)
			}
			var reply Response
			if err := conn.ReadJSON(&reply); err != nil {
				t.Fatal(err)
			}
			if reply.Error == nil || reply.Error.Message != "not found" {
				t.Fatalf("unexpected reply: %+v", reply)
			}
		})
	}
}

func TestRPCListenRequiresLoopback(t *testing.T) {
	for _, address := range []string{"127.0.0.1:9000", "[::1]:9000"} {
		if err := validateRPCListen(address); err != nil {
			t.Fatal(err)
		}
	}
	for _, address := range []string{":9000", "0.0.0.0:9000", "example.com:9000"} {
		if err := validateRPCListen(address); err == nil {
			t.Fatalf("accepted %s", address)
		}
	}
}

func TestCreateRejectsUnsupportedCredentials(t *testing.T) {
	if _, err := NewManager("", "", true).Create("account", "password"); err == nil {
		t.Fatal("credentials silently ignored")
	}
}

func TestRPCRejectsOversizedRequest(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"
	s := httptest.NewServer(rpcHandler(NewManager("", "", true), token))
	defer s.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(s.URL, "http"), http.Header{"Authorization": {"Bearer " + token}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, []byte(strings.Repeat(" ", maxRPCRequestBytes+1))); err != nil {
		t.Fatal(err)
	}
	_, _, err = conn.ReadMessage()
	if !websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
		t.Fatalf("expected size-limit close, got %v", err)
	}
}
