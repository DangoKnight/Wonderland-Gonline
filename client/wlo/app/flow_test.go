package app

import (
	"bytes"
	"net"
	"strconv"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/internal/protocol"
)

// until runs frames until cond holds.
func until(t *testing.T, c *Client, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for", what)
		}
		c.Frame()
		time.Sleep(time.Millisecond)
	}
}

// receive waits for the next packet the fake server read.
func receive(t *testing.T, got <-chan []byte, what string) []byte {
	t.Helper()
	select {
	case p := <-got:
		return p
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for", what)
		return nil
	}
}

func TestLoginFlow(t *testing.T) {
	c := testClient(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	// Only the login socket reaches the fake server; the status query fails.
	c.Net.Dial = func(network, address string) (net.Conn, error) {
		if _, port, _ := net.SplitHostPort(address); port != strconv.Itoa(login.LoginPort) {
			return nil, errNoNetwork
		}
		return net.Dial(network, ln.Addr().String())
	}
	got := make(chan []byte, 4)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			p, err := protocol.Read(conn)
			if err != nil {
				return
			}
			got <- p
			switch p[0] {
			case protocol.CommandDiscovery:
				protocol.Write(conn, []byte{1, 9, 101, 0, 1, 'S'})
			case protocol.CommandLogin:
				protocol.Write(conn, []byte{63, 1})
			}
		}
	}()

	c.Frame()
	c.Servers.Regions.Selected = 0
	c.Servers.Regions.OnSelect(0)
	c.Servers.Connect(0)
	if p := receive(t, got, "action 0"); !bytes.Equal(p, []byte{0}) {
		t.Fatalf("first packet % x", p)
	}
	until(t, c, "the account form", func() bool { return c.Login.Visible })
	if c.Servers.Visible || c.G.ServerWord != 101 || !c.G.ServerFlag || string(c.G.ServerText) != "S" {
		t.Fatalf("after 1/9: servers %v, word %d, flag %v, text %q", c.Servers.Visible, c.G.ServerWord, c.G.ServerFlag, c.G.ServerText)
	}

	// The account field upper-cases letters; Enter moves to the password.
	for _, b := range []byte("tester") {
		c.UI.Char(b)
	}
	c.UI.KeyDown(0x0d, 0)
	if c.Input.Focused != c.Login.Password {
		t.Fatal("Enter in the account field should focus the password")
	}
	for _, b := range []byte("secret") {
		c.UI.Char(b)
	}
	c.UI.KeyDown(0x0d, 0)
	p := receive(t, got, "the login packet")
	want := []byte{63, 4, 0xbb, 0x04, 6, 'T', 'E', 'S', 'T', 'E', 'R', 6, 's', 'e', 'c', 'r', 'e', 't'}
	if !bytes.HasPrefix(p, want) {
		t.Fatalf("login packet % x", p)
	}
	if c.Login.Visible || c.Login.LoginTime.IsZero() || len(c.Login.Password.Text) != 0 {
		t.Fatal("the form should hide, wipe the password and start the timer")
	}
	until(t, c, "the character list", func() bool { return c.Login.LoginTime.IsZero() })
}
