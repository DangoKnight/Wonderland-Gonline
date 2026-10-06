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

func TestNativePoseBatchAndMalformed(t *testing.T) {
	w := &World{Player: Player{ID: 1, Direction: 11}, Peers: map[uint32]*Peer{2: {Player: Player{ID: 2, Direction: 12}}}}
	if !w.ApplyPose([]byte{2, 1, 0, 0, 0, 26, 2, 0, 0, 0, 17, 99, 0, 0, 0, 7}) {
		t.Fatal("batch rejected")
	}
	if w.Player.Direction != 26 || w.Peers[2].Direction != 17 {
		t.Fatal("pose batch wrong")
	}
	for _, p := range [][]byte{{2}, {2, 1, 0, 0, 0}, {2, 1, 0, 0, 0, 8, 2}, {1, 1, 0, 0, 0, 8}} {
		if w.ApplyPose(p) || w.Player.Direction != 26 {
			t.Fatalf("malformed packet mutated state: %v", p)
		}
	}
}

func TestNativeExpressionsRepeatAndExpire(t *testing.T) {
	now := time.Unix(100, 0)
	pics := picdb.New()
	img := image.NewNRGBA(image.Rect(0, 0, 32, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 32; x++ {
			ink := color.NRGBA{R: 255, A: 255}
			if y >= 32 {
				ink = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, ink)
		}
	}
	pics.Add("E101", img)
	env := &seui.Env{Pics: pics, Screen: surface.New(800, 600)}
	w := &World{Env: env, Player: Player{ID: 1, X: 200, Y: 300, Direction: 17}}
	if !w.StartExpression(1, 101, now) {
		t.Fatal("expression rejected")
	}
	w.drawExpressions(0, 0, now)
	if env.Screen.Pix[175*800+218] != 0xf800 {
		t.Fatal("first expression frame misplaced")
	}
	w.drawExpressions(0, 0, now.Add(200*time.Millisecond))
	if env.Screen.Pix[175*800+218] != 0x001f {
		t.Fatal("second frame not drawn")
	}
	if !w.StartExpression(1, 101, now.Add(time.Second)) || w.Player.Direction != 17 {
		t.Fatal("repeat changed held pose")
	}
	w.drawExpressions(0, 0, now.Add(time.Second+1599*time.Millisecond))
	if len(w.Expressions) != 1 {
		t.Fatal("animation expired early")
	}
	w.drawExpressions(0, 0, now.Add(time.Second+1600*time.Millisecond))
	if len(w.Expressions) != 0 {
		t.Fatal("four passes did not expire")
	}
	for _, id := range []uint32{0, 2} {
		if w.StartExpression(id, 101, now) {
			t.Fatal("unknown player expression accepted")
		}
	}
	for _, code := range []byte{0, 125, 255} {
		if w.StartExpression(1, code, now) {
			t.Fatal("invalid expression accepted")
		}
	}
}

func TestNativePeerStopAndEquipment(t *testing.T) {
	w := &World{Peers: map[uint32]*Peer{2: {Player: Player{ID: 2, Items: []uint16{100}}}}}
	if !w.ApplyPeerMovement([]byte{1, 2, 0, 0, 0, 17, 44, 0, 55, 0}, time.Now()) {
		t.Fatal("stop rejected")
	}
	p := w.Peers[2]
	if p.Direction != 17 || p.X != 44 || p.Y != 55 || p.Walker.Walking() {
		t.Fatal("stop planned another walk")
	}
	if !w.ApplyPeerEquipment([]byte{2, 0, 0, 0, 0x34, 0x12}) || len(p.Items) != 1 || p.Items[0] != 0x1234 {
		t.Fatal("equipment snapshot wrong")
	}
	if w.ApplyPeerEquipment([]byte{2, 0, 0, 0, 7}) || p.Items[0] != 0x1234 {
		t.Fatal("partial item word accepted")
	}
	if !w.ApplyPeerEquipment([]byte{2, 0, 0, 0}) || len(p.Items) != 0 {
		t.Fatal("empty equipment not cleared")
	}
}
