package world

import (
	"image"
	"image/color"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
)

type scenePainter struct {
	NPCPainter
	restarted int
}

func (*scenePainter) FrameSize(action int) (int, int) {
	if action == 27 {
		return 20, 30
	}
	return 0, 0
}
func (p *scenePainter) RestartAnimation() { p.restarted++ }

func TestNativeNPCAnimationAndExpression(t *testing.T) {
	now := time.Unix(100, 0)
	painter := &scenePainter{}
	n := &NPC{ClickID: 1, X: 200, Y: 300, Shown: true, Action: 8, Painter: painter, Fixed: true, Frame: 3, route: npcRoute{queued: true}}
	w := &World{NPCs: map[uint16]*NPC{1: n}}
	// AC22:8's byte action selects the first available sprite in its family.
	if !w.ApplyNPCPresentation(8, []byte{1, 0, 26, 255}, now) || n.Action != 27 || n.Fixed || n.route.queued || painter.restarted != 1 {
		t.Fatal("native animation not applied", n)
	}
	if !w.ApplyNPCPresentation(8, []byte{1, 0, 26, 0}, now) || painter.restarted != 2 {
		t.Fatal("same action did not restart")
	}
	if !w.ApplyNPCPresentation(7, []byte{1, 0, 2}, now) || w.NPCExpressions[1].Code != 102 {
		t.Fatal("NPC expression did not add 100")
	}
	for _, p := range [][]byte{{}, {1}, {1, 0}, {1, 0, 2, 0}} {
		if w.ApplyNPCPresentation(7, p, now) {
			t.Fatal("malformed expression accepted")
		}
	}
	for _, p := range [][]byte{{}, {1, 0, 26}, {1, 0, 26, 0, 0}} {
		if w.ApplyNPCPresentation(8, p, now) {
			t.Fatal("malformed animation accepted")
		}
	}
	if w.ApplyNPCPresentation(7, []byte{1, 0, 255}, now) || n.Action != 27 {
		t.Fatal("invalid expression affected pose")
	}
	// An absent actor is ignored, as by the native map actor lookup.
	if !w.ApplyNPCPresentation(7, []byte{2, 0, 2}, now) || len(w.NPCExpressions) != 1 {
		t.Fatal("unknown actor mutated scene")
	}
	// Use an independent strip to check the expression's lifecycle and namespace.
	pics := picdb.New()
	strip := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			strip.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	pics.Add("E102", strip)
	w.Env = &seui.Env{Pics: pics, Screen: surface.New(800, 600)}
	n.Painter = nil
	w.drawExpressions(0, 0, now)
	if len(w.NPCExpressions) != 1 {
		t.Fatal("NPC expression removed early")
	}
	w.drawExpressions(0, 0, now.Add(800*time.Millisecond))
	if len(w.NPCExpressions) != 0 || n.Action != 27 {
		t.Fatal("NPC expression did not expire independently")
	}
}
