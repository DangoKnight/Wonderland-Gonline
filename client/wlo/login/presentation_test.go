package login

import (
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestPresentationClientCannotDialOrSend(t *testing.T) {
	n := NewNet()
	defer n.Shutdown()
	var dials atomic.Int32
	n.Dial = func(string, string) (net.Conn, error) { dials.Add(1); return nil, net.ErrClosed }
	n.SetPresentationOnly()
	n.Connect("127.0.0.1")
	n.QueryStatus("127.0.0.1")
	for _, packet := range [][]byte{{0}, {1, 1}, {20, 1}, {50, 1}} {
		if err := n.Send(packet); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(100 * time.Millisecond)
	if dials.Load() != 0 || len(n.pending) != 0 || n.login != nil || n.status != nil {
		t.Fatal("replica contacted game server")
	}
}
