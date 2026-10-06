package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
