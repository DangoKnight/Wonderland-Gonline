package headless

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/agentwire"
	"wonderland-gonline/client/wlo/app"
)

func TestNativeListenerRequiresExplicitLocalSecret(t *testing.T) {
	for _, address := range []string{"0.0.0.0:9000", "localhost:9000", ":9000", "192.0.2.1:9000"} {
		if Validate(address, strings.Repeat("s", minimumTokenBytes)) == nil {
			t.Fatal(address)
		}
	}
	if Validate("127.0.0.1:9000", "short") == nil {
		t.Fatal("short token accepted")
	}
	if err := Validate("127.0.0.1:0", strings.Repeat("s", minimumTokenBytes)); err != nil {
		t.Fatal(err)
	}
}
func TestNativeAPIAuthAndConnectionIndependence(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("s", minimumTokenBytes)
	c := &app.Client{Now: time.Now}
	c.AgentEnableSessionHost()
	w := &app.Workspace{Sessions: []*app.Session{{ID: 1, Client: c}}}
	h := newHost(w, listener.Addr().String(), token)
	h.step = func() {}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.serve(ctx, listener) }()
	connect := func(secret string) *agentwire.Conn {
		socket, err := net.Dial("tcp", listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		conn := agentwire.New(socket)
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "auth", "params": map[string]string{"token": secret}}); err != nil {
			t.Fatal(err)
		}
		return conn
	}
	bad := connect("wrong")
	if bad.ReadJSON(new(Response)) == nil {
		t.Fatal("wrong secret accepted")
	}
	bad.Close()
	first := connect(token)
	defer first.Close()
	var response Response
	if first.ReadJSON(&response) != nil || response.Error != nil {
		t.Fatal(response)
	}
	second := connect(token)
	if second.ReadJSON(&response) != nil {
		t.Fatal("second auth failed")
	}
	second.Close()
	if err := first.WriteJSON(Request{JSONRPC: "2.0", ID: json.RawMessage(`2`), Method: "agent.contract", Params: json.RawMessage(`{"session":1}`)}); err != nil {
		t.Fatal(err)
	}
	if err := first.ReadJSON(&response); err != nil || response.Error != nil {
		t.Fatal(response, err)
	}
	if err := first.WriteJSON(map[string]any{}); err != nil {
		t.Fatal(err)
	}
	response = Response{}
	if err := first.ReadJSON(&response); err != nil || response.Error == nil {
		t.Fatal("malformed request reused previous method", response, err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown retained API connections")
	}
}
