package app

import (
	"testing"
	"time"
)

// TestLostConnection follows the original's disconnect in the game
// (Chat_02/Connection_Lost.png): the message form shows
// "Connection lost" with Leave and Prev, the scene darkens over 15 game
// frames and freezes, the player cannot walk, and Prev returns to the
// server list while Leave closes the client.
func TestLostConnection(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.Frame()
	c.disconnected()
	if !c.Lost.Visible || string(c.Lost.Text.Text) != "Connection lost" || !c.Lost.Leave.Visible || !c.Lost.Prev.Visible || c.Lost.Finish.Visible {
		t.Fatalf("form visible %v, text %q", c.Lost.Visible, c.Lost.Text.Text)
	}
	if c.Servers.Visible {
		t.Fatal("the server list replaced the game")
	}
	for i := 0; i < fadeSteps+2; i++ {
		*now = now.Add(fadeFrame)
		c.Frame()
	}
	if c.fade.frozen == nil {
		t.Fatal("the scene did not freeze")
	}
	x, y := c.World.Player.X, c.World.Player.Y
	c.GroundClick(100, 100)
	*now = now.Add(time.Second)
	c.Frame()
	if c.World.Player.X != x || c.World.Player.Y != y {
		t.Fatal("the player walked offline")
	}
	c.Lost.Prev.OnClick()
	if c.Lost.Visible || c.World != nil || !c.Servers.Visible || c.fade.frozen != nil {
		t.Fatalf("after Prev: form %v, world %v, servers %v", c.Lost.Visible, c.World != nil, c.Servers.Visible)
	}
	c.Lost.Leave.OnClick()
	if !c.Exit {
		t.Fatal("Leave did not exit")
	}
}

// AC0:19's reason becomes visible on the rejected login socket's close.
func TestDuplicateLoginNotification(t *testing.T) {
	c := testClient(t)
	c.Login.Show()
	c.dispatch([]byte{0, 19})
	c.disconnected()
	if !c.Lost.Visible || string(c.Lost.Text.Text) != "Character is already logged in:19" {
		t.Fatalf("duplicate login notice: visible %v, text %q", c.Lost.Visible, c.Lost.Text.Text)
	}
	if c.G.InGame || c.World != nil || c.disconnectText != nil {
		t.Fatal("rejected login entered world or retained stale reason")
	}
}
