package agentwire

import (
	"net"
	"strings"
	"testing"
)

func TestBoundedJSONFraming(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	go func() { _, _ = a.Write([]byte("{\"value\":1}\n{\"value\":2}\n")) }()
	c := New(b)
	for _, want := range []int{1, 2} {
		var p struct{ Value int }
		if err := c.ReadJSON(&p); err != nil || p.Value != want {
			t.Fatal(p, err)
		}
	}
}
func TestOversizedMessageRejected(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	go func() { _, _ = a.Write([]byte(strings.Repeat("x", 128) + "\n")) }()
	c := New(b)
	c.SetReadLimit(64)
	if c.ReadJSON(new(any)) == nil {
		t.Fatal("oversized message accepted")
	}
}
