package app

import (
	"image"
	"testing"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
)

func TestBattleCameraReturnIsContinuous(t *testing.T) {
	for _, mode := range []byte{1, 2} {
		for _, actor := range []clientbattle.Cell{{X: 4, Y: 2}, {X: 2, Y: 2}} {
			target := clientbattle.Cell{X: 2, Y: 2}
			if actor.X == 2 {
				target.X = 4
			}
			m := clientbattle.Motion{Frames: []clientbattle.MotionFrame{{Offset: image.Pt(200, 0), Speed: 1, Hold: 300 * time.Millisecond}},
				CameraMode: mode, Camera: []clientbattle.CameraPoint{{Anchor: 1, At: image.Pt(650, 150), Speed: 1, Trigger: 100}}}
			start, end := m.ReturnStart(), m.Duration()
			from := motionCamera(m, actor, target, start)
			before := motionCamera(m, actor, target, start-time.Millisecond)
			if abs(from.X-before.X) > 2 || abs(from.Y-before.Y) > 2 {
				t.Fatal("return start snapped", mode, from, before)
			}
			previous := from
			origin := battleCameraOrigin()
			for at := start + time.Millisecond; at <= end; at += time.Millisecond {
				p := motionCamera(m, actor, target, at)
				if abs(p.X-origin.X) > abs(previous.X-origin.X) || abs(p.Y-origin.Y) > abs(previous.Y-origin.Y) {
					t.Fatal("return reversed", mode, previous, p)
				}
				if abs(p.X-previous.X) > 2 || abs(p.Y-previous.Y) > 2 {
					t.Fatal("return snapped", mode, previous, p)
				}
				previous = p
			}
			if previous != origin || motionCamera(m, actor, target, end+time.Millisecond) != origin {
				t.Fatal("cleanup would snap", mode, previous)
			}
		}
	}
}

func TestInstalledBattleCameraSettlesBeforeActionEnds(t *testing.T) {
	c, _, _ := enterBattle(t)
	c.battleFrame(false)
	m := c.battle.assets.Motions[10001]
	actor, target := clientbattle.Cell{X: 4, Y: 2}, clientbattle.Cell{X: 2, Y: 2}
	last := motionCamera(m, actor, target, m.Duration()-time.Millisecond)
	if abs(last.X-battleSceneCameraX) > 1 || abs(last.Y-battleSceneCameraY) > 1 {
		t.Fatal("basic attack ends away from battle center", last)
	}
}
