package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// TestPeers drives another player through the server's own packets: it
// appears (AC4 from game.AppearancePacket), walks (the 6/1 echo), speaks
// (2/2) and leaves (AC12 to map 0, as Server.leaveWorld sends it).
func TestPeers(t *testing.T) {
	c, now, _ := enteredClient(t)
	ann := game.Character{ID: 20002, Slot: 1, Name: "Ann", Level: 3, Element: 2, Body: 1, Head: 0,
		Color1: 444444444, Color2: 444444444, Map: 10017, X: 900, Y: 1100}
	p, err := ann.AppearancePacket(true)
	if err != nil {
		t.Fatal(err)
	}
	c.dispatch(p)
	peer := c.World.Peers[ann.ID]
	if peer == nil || string(peer.Name) != "Ann" || peer.X != 900 || peer.Y != 1100 || peer.Level != 3 {
		t.Fatalf("peer %+v", peer)
	}

	move := protocol.Builder{protocol.CommandMovement, protocol.MovementMove}.U32(ann.ID).U8(3).U16(840).U16(1150)
	c.dispatch(move)
	if !peer.Walker.Walking() {
		t.Fatal("6/1 did not walk the peer")
	}
	for i := 0; i < 200 && peer.Walker.Walking(); i++ {
		*now = now.Add(16 * time.Millisecond)
		c.World.Step(*now)
	}
	if peer.Walker.Walking() || peer.X > 860 || peer.Y < 1140 {
		t.Fatalf("peer at (%d,%d)", peer.X, peer.Y)
	}

	// Our own echo moves nobody.
	self := protocol.Builder{protocol.CommandMovement, protocol.MovementMove}.U32(c.World.Player.ID).U8(0).U16(1).U16(1)
	c.dispatch(self)
	if c.World.Player.X != 1042 {
		t.Fatal("own echo moved the player")
	}

	c.dispatch(protocol.Builder{protocol.CommandChat, protocol.ChatMapMessage}.U32(ann.ID).Bytes([]byte("hi")))
	if last := c.Chat.Lines[len(c.Chat.Lines)-1]; string(last.Text) != "(Local)Ann:hi" {
		t.Fatalf("chat line %q", last.Text)
	}

	if dir := os.Getenv("SNAPSHOT_DIR"); dir != "" {
		c.Frame()
		save(t, c, filepath.Join(dir, "world_peer.png"))
	}

	c.dispatch(protocol.Builder{protocol.CommandMapAcknowledgment}.U32(ann.ID).U16(0).U16(0).U16(0).U16(0).U8(0))
	if c.World.Peers[ann.ID] != nil {
		t.Fatal("departure left the peer")
	}
}
