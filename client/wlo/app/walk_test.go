package app

import (
	"testing"
	"time"

	"wonderland-go/internal/protocol"
)

// enteredClient is a test client standing on Ship Deck, with its sent
// packets collected.
func enteredClient(t *testing.T) (*Client, *time.Time, *[][]byte) {
	c := testClient(t)
	now := time.Unix(0, 0)
	c.Now = func() time.Time { return now }
	c.dispatch(selfPacket(10001, 2, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Dango"))
	if c.World == nil {
		t.Fatal("no world")
	}
	var sent [][]byte
	c.World.OnLeg = func(f, x, y int) { sent = append(sent, []byte{protocol.CommandMovement, byte(f)}) }
	return c, &now, &sent
}

// TestHoldWalk: holding the button after a ground click re-aims the walk at
// the pointer every 400 ms; releasing stops re-aiming.
func TestHoldWalk(t *testing.T) {
	c, now, sent := enteredClient(t)
	c.GroundClick(300, 350)
	first := len(*sent)
	if first == 0 {
		t.Fatal("click did not walk")
	}
	c.GroundHold(true, 320, 360) // too soon
	if len(*sent) != first {
		t.Fatal("re-aimed before 400 ms")
	}
	*now = now.Add(walkRepeat)
	c.GroundHold(true, 320, 360)
	if len(*sent) == first {
		t.Fatal("held button did not re-aim")
	}
	c.GroundHold(false, 0, 0)
	n := len(*sent)
	*now = now.Add(walkRepeat)
	c.GroundHold(true, 320, 360) // a press that started on a control
	if len(*sent) != n {
		t.Fatal("re-aimed after release")
	}
}

// TestArrowWalk: an arrow key walks 0x50 pixels that way, not while a text
// field has focus.
func TestArrowWalk(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.Input.Focused = c.ChatBar.Message
	c.WalkKeys(true, false, false, false)
	if c.World.Walking() {
		t.Fatal("walked while typing")
	}
	c.Input.Focused = nil
	x := c.World.Player.X
	c.WalkKeys(true, false, false, false)
	if !c.World.Walking() {
		t.Fatal("arrow key did not walk")
	}
	for i := 0; i < 100 && c.World.Walking(); i++ {
		*now = now.Add(16 * time.Millisecond)
		c.World.Step(*now)
	}
	if c.World.Player.X >= x || c.World.Player.X < x-walkKeyReach-20 {
		t.Fatalf("x %d after walking left from %d", c.World.Player.X, x)
	}
}
