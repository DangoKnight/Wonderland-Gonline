package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestBurkeTalkSnapshot renders Burke the tiger's line: his body in the
// talk window drops 0x44 like his map sprite (In-Game/Burke_Talk.png).
func TestBurkeTalkSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR not set")
	}
	c, now, _ := enteredClient(t)
	c.dispatch(selfPacket(10001, 4, 10003, 1082, 2055, 7, 444444444, 444444444, nil, "Dango"))
	burke := c.World.NPCs[8]
	if burke.Info.SpriteDrop() == 0 {
		t.Fatalf("Burke %+v has no drop", burke.Info)
	}
	c.Talk.Say([]byte("Roaring..."), npcSpeaker(burke), c.World.Player.Name)
	for i := 0; i < 30; i++ {
		*now = now.Add(20 * time.Millisecond)
		c.Frame()
	}
	save(t, c, filepath.Join(dir, "burke_talk.png"))
}
