package app

import (
	"bytes"
	"net"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/protocol"
)

func TestAltOneSitsAndSendsNativePose(t *testing.T) {
	c, _, _ := shoreClient(2)
	c.Input = &seui.Input{}
	c.Net = login.NewNet()
	host, client := net.Pipe()
	defer host.Close()
	defer c.Net.Close()
	c.Net.Dial = func(string, string) (net.Conn, error) { return client, nil }
	c.Net.Connect("test")
	// A queued send while connecting and a connected send use the same packet.
	got := make(chan []byte, 1)
	go func() { host.SetReadDeadline(time.Now().Add(5 * time.Second)); p, _ := protocol.Read(host); got <- p }()
	c.World.Player.Direction = 14
	if !c.EmoteKey('1', shiftAlt) {
		t.Fatal("Alt+1 not handled")
	}
	if p := <-got; !bytes.Equal(p, []byte{32, 2, 17}) || c.World.Player.Direction != 17 {
		t.Fatal("sit request", p, c.World.Player.Direction)
	}
	c.World.Player.VehicleID = 48016
	c.World.Player.Direction = 12
	if !c.EmoteKey('1', shiftAlt) || c.World.Player.Direction != 12 {
		t.Fatal("mounted pose changed")
	}
	if c.EmoteKey('1', 0) {
		t.Fatal("unmodified digit consumed")
	}
}
