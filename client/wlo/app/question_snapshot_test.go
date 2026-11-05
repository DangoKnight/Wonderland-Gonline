package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wonderland-gonline/internal/protocol"
)

func TestXaolanPlayerQuestionShowsBothOptions(t *testing.T) {
	const xaolanHouseMap = 12002
	const xaolanDecisionQuestion = 1
	for option := 0; option < 2; option++ {
		c, now, _ := enteredClient(t)
		c.dispatch(selfPacket(10001, 2, xaolanHouseMap, 502, 455, 0, 444444444, 444444444, nil, "Dango"))
		// Transition directly from the preceding ordinary player dialogue.
		c.dispatch(selfTalkFrame(30306))
		settle(c, now)
		if width, _ := c.Talk.Size(); width < 700 {
			t.Fatal("ordinary dialogue did not use large artwork")
		}
		sent := wire(t, c)
		question := questionFrame(0, xaolanDecisionQuestion)
		question[7] = eventSpeakerSelf
		c.dispatch(question)
		settle(c, now)
		options := c.Talk.Options()
		if len(options) != 2 || string(options[0]) != "Help them." || string(options[1]) != "Turn a blind eye to it." {
			t.Fatal("native Xaolan choices missing", options)
		}
		if width, height := c.Talk.Size(); width >= 700 || height < 200 {
			t.Fatal("question did not fit the compact portrait and choices", width, height)
		}
		for row := range options {
			point := c.Talk.OptionPoint(row)
			if point.Y < 25 || point.Y > 350 || point.X < 0 || point.X >= ScreenWidth {
				t.Fatal("Xaolan option is not in the upper dialogue", point)
			}
		}
		point := c.Talk.OptionPoint(option)
		c.Input.X, c.Input.Y = point.X, point.Y
		c.Frame()
		if dir := os.Getenv("XAOLAN_QUESTION_SNAPSHOT"); dir != "" && option == 0 {
			savePNG(t, dir, c)
		}
		c.GroundClick(point.X, point.Y)
		c.Frame()
		packets := sent()
		if len(packets) != 2 || !bytes.Equal(packets[0], []byte{protocol.CommandEvent, protocol.EventChoice, answerOption + byte(option)}) || !bytes.Equal(packets[1], []byte{protocol.CommandEvent, protocol.EventAcknowledge}) {
			t.Fatal("wrong Xaolan answer/ACK", packets)
		}
	}
}

// TestQuestionSnapshot renders In-Game/Question_Robinson_Hover.png:
// Robinson (map 10003) asks question 1 with three options, the first
// hovered. The prompt, face, option rows, Close button and frame align
// with the capture.
func TestQuestionSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR is not set")
	}
	c := testClient(t)
	now := time.Now()
	c.Now = func() time.Time { return now }
	c.Frame()
	c.G.ServerText = []byte("Wonderland Gonline Server")
	c.dispatch(selfPacket(10001, 2, 10003, 1082, 2235, 0, 444444444, 444444444, []uint16{22003, 21002, 24002}, "Dango"))
	if c.World == nil {
		t.Fatal("no world")
	}
	var robinson uint16
	for _, n := range c.World.NPCs {
		if n.Info.Name == "Robinson" {
			robinson = n.ClickID
		}
	}
	if robinson == 0 {
		t.Fatal("no Robinson on map 10003")
	}
	c.dispatch(questionFrame(robinson, 1))
	settle(c, &now)
	if got := c.Talk.Options(); len(got) != 3 || string(got[0]) != "Beginner's Guide" {
		t.Fatalf("options %q", got)
	}
	p := c.Talk.OptionPoint(0)
	c.Input.X, c.Input.Y = p.X, p.Y
	c.Frame()
	save(t, c, filepath.Join(dir, "question_robinson.png"))
}
