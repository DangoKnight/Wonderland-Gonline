package app

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/internal/protocol"
)

// selfTalkFrame is the server's kind-1 frame for a line the player says
// (subject 7).
func selfTalkFrame(talk uint16) []byte {
	p := []byte{protocol.CommandEvent, 1, 0, 0, 0, 1, eventKindTalk, eventSpeakerSelf, 0, 0, 1, 0, 0, 0, 0}
	p = binary.LittleEndian.AppendUint16(p, talk)
	return append(p, 0)
}

// TestPlayerTalkSnapshot renders the player's line of
// In-Game/Player_Dialogue.png (talk 20306) in the wide form with
// the large art.
func TestPlayerTalkSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR is not set")
	}
	c := testClient(t)
	now := time.Now()
	c.Now = func() time.Time { return now }
	c.Frame()
	// The capture's character: body 4, whose large art is 5607.
	c.dispatch(selfPacket(10001, 4, 10003, 1082, 2235, 7, 444444444, 444444444, nil, "Dango"))
	if c.World == nil {
		t.Fatal("no world")
	}
	c.Talk.Lowered = true // the capture's mode, HUD hidden
	c.dispatch(selfTalkFrame(20306))
	settle(c, &now)
	if got := c.Talk.Lines(); len(got) != 2 || string(got[0]) != "I was on the ship then suddenly it started " {
		t.Fatalf("lines %q", got)
	}
	save(t, c, filepath.Join(dir, "player_talk.png"))
}
