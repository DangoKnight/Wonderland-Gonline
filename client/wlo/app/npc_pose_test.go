package app

import "testing"

func TestKelanHouseFallenNPCDispatch(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.dispatch(selfPacket(10001, 4, 12004, 462, 435, 7, 444444444, 444444444, nil, "Dango"))
	c.dispatch([]byte{22, 4, 2, 0, 255, 1, 125, 1, 148, 1, 1, 0, 0, 0, 0, 26})
	c.dispatch([]byte{22, 9, 2, 0, 26, 1})
	c.Frame()
	n := c.World.NPCs[2]
	if n == nil || n.Action != 26 || !n.Fixed || n.Frame != 1 {
		t.Fatalf("house pose lost through dispatcher/render: %+v", n)
	}
	painter, ok := n.Painter.(interface{ FrameSize(int) (int, int) })
	if !ok {
		t.Fatal("missing NPC painter")
	}
	if width, height := painter.FrameSize(n.Action); width == 0 || height == 0 {
		t.Fatal("fallen sprite unavailable")
	}
}
