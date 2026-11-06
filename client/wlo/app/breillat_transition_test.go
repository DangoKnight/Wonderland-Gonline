package app

import (
	"bytes"
	"testing"
	"time"
)

func TestBreillatPlayerToNPCDialogue(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.dispatch(selfPacket(10001, 4, 10002, 450, 450, 0, 444444444, 444444444, nil, "Tester"))
	sent := wire(t, c)
	// Captured conversion sequence: player talk 30641, then Breillat 30642.
	c.dispatch([]byte{6, 2, 1})
	c.dispatch([]byte{20, 1, 0, 0, 0, 2, 1, 7, 0, 0, 1, 0, 0, 0, 0, 177, 119, 1})
	*now = now.Add(120 * time.Millisecond)
	c.Frame()
	// The preferred IGG English catalog uses this shorter line for talk 30641.
	if got := string(bytes.Join(c.Talk.Lines(), nil)); got != "why?" {
		t.Fatalf("player line %q", got)
	}
	c.GroundClick(400, 500)
	if c.Talk.Drawn() || c.Talk.Contains(400, 500) {
		t.Fatal("empty player panel lingered while awaiting the next server line")
	}
	c.Frame()
	if p := sent(); len(p) != 1 || !bytes.Equal(p[0], []byte{20, 6}) {
		t.Fatal("player line acknowledgement", p)
	}
	*now = now.Add(16 * time.Millisecond)
	c.dispatch([]byte{20, 1, 0, 0, 0, 3, 1, 3, 5, 0, 1, 0, 0, 0, 0, 178, 119, 0})
	c.Frame()
	if !c.Talk.Shown() || !c.Talk.Drawn() || string(bytes.Join(c.Talk.Lines(), nil)) != "Since you want to talk with me, I will tell you what I think." {
		t.Fatal("next Breillat line was delayed or empty", c.Talk.Lines())
	}
}
