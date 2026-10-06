package minigame

import (
	"image"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/surface"
)

// stubRole is a 40 × 60 monster sprite standing on its feet.
type stubRole struct{}

func (stubRole) Draw(*surface.Surface, int, int, int) {}
func (stubRole) Bounds(x, y, _ int) image.Rectangle {
	return image.Rect(x-20, y-60, x+20, y)
}
func (stubRole) FrameSize(int) (int, int) { return 40, 60 }
func (stubRole) Hold(int, bool)           {}

func newHunt(t *testing.T, picks ...int) (*Hunter, *time.Time, *[]bool) {
	t.Helper()
	now := time.Unix(0, 0)
	h := NewHunter(picdb.New(), now)
	h.NewMonster = func(uint16) Monster { return stubRole{} }
	k := 0
	h.Rand = func(n int) int {
		v := picks[k%len(picks)] % n
		k++
		return v
	}
	var results []bool
	h.Result = func(win bool) { results = append(results, win) }
	h.Body = 1
	return h, &now, &results
}

func (h *Hunter) run(now *time.Time, frames int) {
	for range frames {
		*now = now.Add(hunterFrame)
		h.Step(*now)
	}
}

// TestHunterFacing: FUN_00411fd8's eight walking directions.
func TestHunterFacing(t *testing.T) {
	for _, tc := range []struct{ dx, dy, want int }{
		{0, -10, 0}, {-10, -10, 1}, {-10, 0, 2}, {-10, 10, 3},
		{0, 10, 4}, {10, 10, 5}, {10, 0, 6}, {10, -10, 7}, {100, 10, 6}, {10, 100, 4},
	} {
		m := &monster{x: 100, y: 100, tx: 100 + tc.dx, ty: 100 + tc.dy}
		m.face()
		if m.action != tc.want {
			t.Errorf("(%d, %d): facing %d, want %d", tc.dx, tc.dy, m.action, tc.want)
		}
	}
}

// TestHunterStep: a monster moves its pace times the elapsed time on both
// axes, then stands at its target; walkers stop at the ground line.
func TestHunterStep(t *testing.T) {
	now := time.Unix(0, 0)
	m := &monster{id: MonsterA, x: 100, y: 400, tx: 200, ty: 400, stepAt: now}
	m.face()
	m.step(now.Add(100 * time.Millisecond))
	if m.x != 110 || m.y != 400 || m.arrived {
		t.Fatalf("after 100 ms at 0.1 px/ms: (%d, %d) arrived %v", m.x, m.y, m.arrived)
	}
	m.x = 200
	m.step(now.Add(130 * time.Millisecond))
	if !m.arrived || m.action != actionStand+6 {
		t.Fatalf("at the target: arrived %v action %d", m.arrived, m.action)
	}
	w := &monster{id: MonsterB, x: 100, y: 310, tx: 100, ty: 100, stepAt: now}
	w.step(now.Add(time.Second))
	if w.y != hunterGroundY || !w.arrived {
		t.Fatalf("walker above the ground line: y %d", w.y)
	}
}

// TestHunterAttack: a monster that reaches the player costs a life once
// per attack, with the body's cry, and the screen shakes meanwhile.
func TestHunterAttack(t *testing.T) {
	h, now, _ := newHunt(t, 0)
	var sounds []string
	h.Sound = func(p string) { sounds = append(sounds, p) }
	h.Start(*now)
	m := h.monsters[0]
	m.x, m.y, m.tx, m.ty = hunterPlayerX, hunterPlayerY, hunterPlayerX, hunterPlayerY
	m.countdown = 1
	m.arrived = true
	h.run(now, 20)
	if !m.attacking || h.lives != hunterLives-1 {
		t.Fatalf("attacking %v lives %d", m.attacking, h.lives)
	}
	if len(sounds) == 0 || sounds[len(sounds)-1] != hurtSounds[1] {
		t.Fatalf("sounds %q", sounds)
	}
	if !h.shaking {
		t.Fatal("no shake during the attack")
	}
	h.run(now, 50)
	if m.attacking || h.lives != hunterLives-1 || h.shaking {
		t.Fatalf("after the attack: attacking %v lives %d shaking %v", m.attacking, h.lives, h.shaking)
	}
}

// TestHunterShot: a click hurts the monsters under the pointer; at 0 they
// die, count 5 points, and go after three 50-frame ticks.
func TestHunterShot(t *testing.T) {
	h, now, _ := newHunt(t, 0)
	h.Start(*now)
	h.lives = 100 // later monsters reach the player meanwhile
	m := h.monsters[0]
	m.x, m.y = 300, 400
	dst := surface.New(800, 600)
	h.Draw(dst, 300, 380, *now)
	if !m.hovered {
		t.Fatal("monster not under the pointer")
	}
	h.Click(300, 380)
	h.run(now, 1)
	if !m.dead || h.kills != 1 || h.score != hunterScorePer || m.action != actionDie && m.action != actionDie+1 {
		t.Fatalf("dead %v kills %d score %d action %d", m.dead, h.kills, h.score, m.action)
	}
	h.run(now, 3*hunterTick)
	for _, o := range h.monsters {
		if o == m {
			t.Fatal("the dead monster was not removed")
		}
	}
}

// TestHunterResult: surviving the clock wins, 100 frames after it ends;
// losing every life loses.
func TestHunterResult(t *testing.T) {
	h, now, results := newHunt(t, 0)
	h.Start(*now)
	h.NewMonster = func(uint16) Monster { return nil } // nothing attacks
	h.monsters = nil
	h.run(now, hunterSeconds*hunterTick+hunterEndWait+1)
	if len(*results) != 1 || !(*results)[0] {
		t.Fatalf("results %v, seconds %d", *results, h.seconds)
	}
	h, now, results = newHunt(t, 0)
	h.Start(*now)
	h.lives = 0
	h.run(now, hunterEndWait+1)
	if len(*results) != 1 || (*results)[0] {
		t.Fatalf("results %v with no lives", *results)
	}
}
