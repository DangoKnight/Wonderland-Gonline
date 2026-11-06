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

func TestAltOneKneelsAndSendsNativePose(t *testing.T) {
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
	// Alt+1 holds action 13 (0x4bd7bc → 46, the kneeling sit), turned to
	// the standing facing 6: action 52.
	c.World.Player.Direction = 14
	if !c.EmoteKey('1', shiftAlt) {
		t.Fatal("Alt+1 not handled")
	}
	if p := <-got; !bytes.Equal(p, []byte{32, 2, 52}) || c.World.Player.Direction != 52 {
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
