package login

import (
	"net"
	"testing"
	"time"
)

func TestShutdownClosesLateDialResults(t *testing.T) {
	for _, status := range []bool{false, true} {
		t.Run(map[bool]string{false: "login", true: "status"}[status], func(t *testing.T) {
			near, far := net.Pipe()
			defer far.Close()
			started := make(chan struct{})
			release := make(chan struct{})
			n := NewNet()
			n.Dial = func(string, string) (net.Conn, error) { close(started); <-release; return near, nil }
			if status {
				n.QueryStatus("localhost")
			} else {
				n.Connect("localhost")
			}
			<-started
			n.Shutdown()
			close(release)
			if err := far.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, err := far.Read(make([]byte, 1)); err == nil {
				t.Fatal("late socket not closed")
			} else if e, ok := err.(net.Error); ok && e.Timeout() {
				t.Fatal("late socket leaked")
			}
			n.mu.Lock()
			connected := n.login != nil
			n.mu.Unlock()
			if n.StatusActive() || connected {
				t.Fatal("disposed socket became active")
			}
		})
	}
}
