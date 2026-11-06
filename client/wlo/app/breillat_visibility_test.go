package app

import (
	"os"
	"testing"

	"wonderland-gonline/client/wlo/surface"
)

func TestBreillatConversationRemainsVisible(t *testing.T) {
	c, _, _ := enteredClient(t)
	if bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE"); bundle != "" {
		useCompiledPetAssets(t, c, bundle)
	}
	c.dispatch(selfPacket(10001, 2, 10002, 450, 450, 0, 444444444, 444444444, nil, "Tester"))
	n := c.World.NPCs[5]
	if n == nil || n.Painter == nil {
		t.Fatal("Breillat sprite missing")
	}
	original := n.Action
	for _, delta := range [][2]int{{0, -50}, {-50, -50}, {-50, 0}, {-50, 50}, {0, 50}, {50, 50}, {50, 0}, {50, -50}} {
		c.World.Player.X, c.World.Player.Y = n.X+delta[0], n.Y+delta[1]
		c.dispatch([]byte{6, 2, 1})
		c.dispatch([]byte{20, 1, 0, 0, 0, 1, 1, 3, 5, 0, 1, 0, 0, 0, 0, 236, 119, 0})
		if n.Painter.Bounds(120, 240, n.Action).Empty() {
			t.Errorf("Breillat vanished toward %v: action %d, original %d, template %d", delta, n.Action, original, n.Template)
		}
		dst := surface.New(240, 300)
		n.Painter.Draw(dst, 120, 240, n.Action)
		visible := false
		for _, pixel := range dst.Pix {
			if pixel != 0 {
				visible = true
				break
			}
		}
		if !n.Shown || !visible {
			t.Errorf("Breillat did not draw toward %v", delta)
		}
		c.dispatch([]byte{20, 8})
		if n.Action != original {
			t.Fatal("original pose not restored")
		}
	}
}
