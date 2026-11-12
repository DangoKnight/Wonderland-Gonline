package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestViewerRejectsCrossOriginAndMissingCapability(t *testing.T) {
	m := NewManager("", "", true)
	mux := http.NewServeMux()
	m.installViewer(mux)
	s := httptest.NewServer(mux)
	defer s.Close()
	u := "ws" + strings.TrimPrefix(s.URL, "http") + "/viewer/ws"
	conn, response, err := websocket.DefaultDialer.Dial(u, http.Header{"Origin": []string{"http://untrusted.invalid"}})
	if conn != nil {
		conn.Close()
	}
	if err == nil || response.StatusCode != http.StatusForbidden {
		t.Fatal("cross-origin viewer accepted")
	}
	conn, _, err = websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = conn.WriteJSON(map[string]string{"ticket": "invalid"}); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err = conn.ReadMessage(); err == nil {
		t.Fatal("uncredentialed viewer received data")
	}
}
func TestViewerRequiresExplicitSession(t *testing.T) {
	m := NewManager("", "", true)
	if _, err := m.openViewer("missing", 1); err == nil {
		t.Fatal("unknown agent accepted")
	}
	m.agents["test"] = &Agent{ID: "test"}
	if _, err := m.openViewer("test", 0); err == nil {
		t.Fatal("active-session alias accepted for persistent viewer")
	}
}
