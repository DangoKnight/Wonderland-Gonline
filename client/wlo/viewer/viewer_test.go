package viewer

import (
	"context"
	"net"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/agentwire"
	"wonderland-gonline/client/wlo/sessionstream"
)

func TestViewerNativeTCPTransport(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan map[string]any, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		wire := agentwire.New(conn)
		_ = wire.SetDeadline(time.Now().Add(time.Second))
		var hello map[string]any
		if wire.ReadJSON(&hello) != nil || hello["ticket"] != "native-ticket" {
			return
		}
		if wire.WriteJSON(frame{Native: &sessionstream.Batch{Version: sessionstream.Version, Epoch: "test", Start: time.Now()}, Control: control{Owner: "viewer", Viewer: "native", Generation: 2}, Viewer: "native"}) != nil {
			return
		}
		var input map[string]any
		if wire.ReadJSON(&input) == nil {
			got <- input
		}
	}()
	g := &Connection{input: make(chan outbound, viewerInputBuffer), native: make(chan sessionstream.Batch, viewerNativeQueue)}
	done := make(chan struct{})
	go func() { defer close(done); g.connectionLoop(ctx, "tcp://"+listener.Addr().String(), "native-ticket") }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		g.mu.Lock()
		ready := g.connected
		g.mu.Unlock()
		if ready {
			break
		}
		time.Sleep(time.Millisecond)
	}
	g.send(map[string]any{"operation": "input", "kind": "reset"}, true)
	select {
	case input := <-got:
		if input["operation"] != "input" || input["generation"] != float64(2) {
			t.Fatal(input)
		}
	case <-time.After(time.Second):
		t.Fatal("native viewer did not authenticate and send input")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("native viewer did not disconnect")
	}
}

func TestViewerInputRequiresCurrentController(t *testing.T) {
	g := &Connection{input: make(chan outbound, viewerInputBuffer), native: make(chan sessionstream.Batch, viewerNativeQueue)}
	g.connected = true
	g.connection = 1
	g.viewer = "mine"
	g.control = control{Owner: "api", Generation: 1}
	g.send(map[string]any{"operation": "input"}, true)
	if len(g.input) != 0 {
		t.Fatal("readonly viewer sent input")
	}
	g.control = control{Owner: "viewer", Viewer: "another", Generation: 2}
	g.send(map[string]any{"operation": "input"}, true)
	if len(g.input) != 0 {
		t.Fatal("another viewer's lease permitted input")
	}
	g.control.Viewer = "mine"
	g.send(map[string]any{"operation": "input"}, true)
	p := <-g.input
	if p.connection != 1 || p.data["generation"] != uint64(2) || p.data["sequence"] != uint64(1) {
		t.Fatal(p)
	}
	g.control.Generation = 3
	g.send(map[string]any{"operation": "input"}, true)
	p = <-g.input
	if p.data["generation"] != uint64(3) || p.data["sequence"] != uint64(1) {
		t.Fatal("generation did not reset sequence", p)
	}
}
func TestViewerOverflowDisconnectsRatherThanLosingKeyUp(t *testing.T) {
	g := &Connection{input: make(chan outbound, 1), connected: true, viewer: "mine", control: control{Owner: "viewer", Viewer: "mine", Generation: 2}}
	closed := false
	g.disconnect = func() { closed = true }
	g.send(map[string]any{"operation": "input"}, true)
	g.send(map[string]any{"operation": "input"}, true)
	if !closed || g.connected || len(g.input) != 0 {
		t.Fatal("overflow did not discard held input and detach")
	}
}

func TestViewerReconnectResumesAppliedFrame(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ack := make(chan struct{})
	resumed := make(chan map[string]any, 1)
	go func() {
		first, err := listener.Accept()
		if err != nil {
			return
		}
		wire := agentwire.New(first)
		var hello map[string]any
		if wire.ReadJSON(&hello) != nil {
			return
		}
		batch := sessionstream.Batch{Version: sessionstream.Version, Epoch: "session", Head: 20, Start: time.Now(), TailFirst: 1, Tail: []sessionstream.Frame{{}}}
		_ = wire.WriteJSON(frame{Native: &batch})
		<-ack
		first.Close()
		second, err := listener.Accept()
		if err != nil {
			return
		}
		defer second.Close()
		wire = agentwire.New(second)
		if wire.ReadJSON(&hello) == nil {
			resumed <- hello
		}
	}()
	g, err := New("tcp://" + listener.Addr().String() + "/#ticket")
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	deadline := time.Now().Add(2 * time.Second)
	received := false
	for time.Now().Before(deadline) {
		if _, ok := g.PollNative(); ok {
			received = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !received {
		close(ack)
		t.Fatal("no initial batch")
	}
	g.Acknowledge(7, "session")
	close(ack)
	select {
	case hello := <-resumed:
		if hello["after"] != float64(7) || hello["epoch"] != "session" || hello["native"] != true {
			t.Fatal("resumed received instead of applied cursor", hello)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("viewer did not reconnect")
	}
}
