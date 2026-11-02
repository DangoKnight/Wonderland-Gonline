package app

import (
	"bytes"
	"testing"

	"wonderland-gonline/client/wlo/login"
)

func TestMovieHandshakeWire(t *testing.T) {
	c := &Client{Net: login.NewNet()}
	sent := wire(t, c)
	var unhandled [][]byte
	c.Unhandled = func(p []byte) { unhandled = append(unhandled, bytes.Clone(p)) }

	// IDs and opaque fields are independent wire fixtures, not map/movie
	// exceptions. The exchange also works before a world has loaded.
	for i, tc := range []struct {
		request, response []byte
	}{
		{[]byte{186, 12, 1, 0, 0, 0, 0}, []byte{186, 9, 1, 0}},
		{[]byte{186, 12, 0x34, 0x12, 7, 8, 9}, []byte{186, 9, 0x34, 0x12}},
		{[]byte{186, 12, 0, 0, 0, 0, 0}, []byte{186, 9, 0, 0}},
	} {
		c.dispatch(tc.request)
		got := sent()
		if len(got) != i+1 || !bytes.Equal(got[i], tc.response) {
			t.Fatalf("request %v: sent %v", tc.request, got)
		}
		before := len(got)
		c.dispatch([]byte{186, 9, tc.response[2], tc.response[3], 1, 0, 0, 0, 0})
		if got := sent(); len(got) != before {
			t.Fatalf("server acknowledgment was echoed: %v", got)
		}
	}
	if len(unhandled) != 0 {
		t.Fatalf("unhandled compatibility packets: %v", unhandled)
	}
}

func TestMovieHandshakeMalformed(t *testing.T) {
	c := &Client{Net: login.NewNet()}
	sent := wire(t, c)
	var unhandled [][]byte
	c.Unhandled = func(p []byte) { unhandled = append(unhandled, bytes.Clone(p)) }
	var packets [][]byte
	for _, valid := range [][]byte{
		{186, 12, 1, 0, 0, 0, 0},
		{186, 9, 1, 0, 1, 0, 0, 0, 0},
	} {
		for n := 1; n < len(valid); n++ {
			packets = append(packets, bytes.Clone(valid[:n]))
		}
		packets = append(packets, append(bytes.Clone(valid), 0))
	}
	packets = append(packets, []byte{186, 255, 1, 0, 0, 0, 0})
	for _, p := range packets {
		c.dispatch(p)
		if c.event.active || c.event.done || c.movie != nil || c.held {
			t.Fatalf("invalid packet changed interaction state: %v", p)
		}
	}
	if got := sent(); len(got) != 0 {
		t.Fatalf("invalid packets produced replies: %v", got)
	}
	if len(unhandled) != len(packets) {
		t.Fatalf("reported %d invalid packets, want %d", len(unhandled), len(packets))
	}
}
