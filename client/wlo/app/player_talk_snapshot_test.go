package app

import (
	"encoding/binary"
	"image"
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

func TestUsedArcadePlayerDialogue(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.dispatch(selfPacket(10001, 2, 10017, 1042, 1075, 1, 444444444, 444444444, nil, "Tester"))
	// Installed used-arcade event: talk 30620 with trailing event flag 1.
	c.dispatch([]byte{20, 1, 0, 0, 0, 1, 1, 7, 0, 0, 1, 0, 0, 0, 0, 156, 119, 1})
	settle(c, now)
	if got := c.Talk.Lines(); len(got) != 1 || string(got[0]) != "I've played this game. What else I can play?" {
		t.Fatalf("arcade dialogue %q", got)
	}
	yellowPixels := 0
	for y := 451; y < 469; y++ {
		for x := 244; x < 620; x++ {
			if c.Screen.Pix[y*c.Screen.W+x] == 0xffe0 {
				yellowPixels++
			}
		}
	}
	if yellowPixels < 100 {
		t.Fatalf("player dialogue text was not rendered: %d yellow pixels", yellowPixels)
	}
	if path := os.Getenv("PLAYER_DIALOGUE_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
}

func TestPlayerDialogueBlinkKeepsArtwork(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.dispatch(selfPacket(10001, 4, 10002, 450, 450, 3, 444444444, 444444444, nil, "Tester"))
	c.dispatch([]byte{20, 1, 0, 0, 0, 2, 1, 7, 0, 0, 1, 0, 0, 0, 0, 177, 119, 1})
	*now = now.Add(200 * time.Millisecond)
	c.Screen.Fill(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0)
	c.Talk.Draw()
	before := append([]uint16(nil), c.Screen.Pix...)
	*now = now.Add(9 * time.Second) // Beyond the randomized 6–8 second blink interval.
	c.Screen.Fill(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0)
	c.Talk.Draw()
	for y := 450; y < 540; y++ {
		for x := 80; x < 200; x++ {
			at := y*ScreenWidth + x
			if before[at] != c.Screen.Pix[at] {
				t.Fatalf("blink changed the body below the eyelid overlay at %d,%d", x, y)
			}
		}
	}
	if !c.Talk.Shown() {
		t.Fatal("blink closed the dialogue")
	}
	if path := os.Getenv("PLAYER_DIALOGUE_BLINK_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
}
