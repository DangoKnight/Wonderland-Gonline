package app

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestTimedStatusShortcutRequests(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	c.timedStatusPacket([]byte{5, 17, 3, 1, 120, 0, 0, 0, 100})
	if c.timedStatus.boost == nil || !c.timedStatus.boost.Visible {
		t.Fatal("missing EXP shortcut")
	}
	c.timedStatus.boost.Click()
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{5, 17}) {
		t.Fatal(got)
	}
	c.timedStatusPacket([]byte{23, 106, 1, 1, 60, 0, 1})
	c.timedStatus.pass.Click()
	if got := sent(); len(got) != 2 || !bytes.Equal(got[1], []byte{23, 111, 2}) {
		t.Fatal(got)
	}
	c.timedStatusPacket([]byte{23, 204, 1, 1})
	c.timedStatus.boost.Click()
	if got := sent(); len(got) != 3 || !bytes.Equal(got[2], []byte{23, 110, 0}) {
		t.Fatal(got)
	}
	c.clearTimedStatus()
	if c.timedStatus.boost.Visible || c.timedStatus.pass.Visible {
		t.Fatal("logout retained shortcuts")
	}
}

func TestTimedStatusNativePackets(t *testing.T) {
	c, _, _ := shoreClient(2)
	if !c.timedStatusPacket([]byte{5, 17, 3, 1, 120, 0, 0, 0, 100}) || c.timedStatus.kind != game.TimedItemPotion || c.timedStatus.tenths != 100 || c.timedStatus.minutes != 120 {
		t.Fatal(c.timedStatus)
	}
	c.timedStatusPacket([]byte{5, 17, 2, 1})
	if c.timedStatus.kind != game.TimedItemPotion || c.timedStatus.minutes != 120 {
		t.Fatal("rejection changed active effect")
	}
	c.timedStatusPacket([]byte{23, 204, 2, 1})
	if !c.timedStatus.paused {
		t.Fatal("native Stopwatch pause ignored")
	}
	c.timedStatusPacket([]byte{23, 204, 2, 0})
	if c.timedStatus.paused {
		t.Fatal("native Stopwatch resume ignored")
	}
	c.timedStatusPacket([]byte{23, 106, 1, 1, 60, 0, 1})
	if c.timedStatus.training != 60 {
		t.Fatal(c.timedStatus)
	}
	c.timedStatusPacket([]byte{23, 106, 2})
	c.timedStatusPacket([]byte{5, 17, 4, 0})
	if c.timedStatus.training != 0 || c.timedStatus.kind != game.TimedItemNone {
		t.Fatal("expiry", c.timedStatus)
	}
	for _, p := range [][]byte{{5, 17}, {5, 17, 3}, {23, 106, 1}, {5, 14, 1}} {
		c.timedStatusPacket(p)
	}
}
