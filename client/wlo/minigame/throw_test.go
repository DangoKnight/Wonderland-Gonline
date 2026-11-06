package minigame

import (
	"testing"
	"time"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/surface"
)

type stubFigure struct{ drawn []int }

func (f *stubFigure) Draw(_ *surface.Surface, _, _, action int) { f.drawn = append(f.drawn, action) }
func (f *stubFigure) Hold(int, bool)                            {}
func (f *stubFigure) Height() int                               { return 0x50 }

func newThrowGame(t *testing.T, ownAI bool, picks ...int) (*Throw, *time.Time, *[]bool) {
	t.Helper()
	now := time.Unix(0, 0)
	fig := func(n int) []Figure {
		out := make([]Figure, n)
		for i := range out {
			out[i] = &stubFigure{}
		}
		return out
	}
	g := NewThrow(picdb.New(), fig(4), 0, fig(4), ownAI, now)
	k := 0
	g.Rand = func(n int) int {
		if len(picks) == 0 {
			return 0
		}
		v := picks[k%len(picks)] % n
		k++
		return v
	}
	var results []bool
	g.Result = func(win bool) { results = append(results, win) }
	g.Begin(now)
	return g, &now, &results
}

func (g *Throw) frames(now *time.Time, n int) {
	for range n {
		*now = now.Add(throwFrame)
		g.Update(*now)
	}
}

// TestThrowMoveStep: FUN_001aff54 moves the longer axis at full speed and
// the shorter in proportion, never past the target.
func TestThrowMoveStep(t *testing.T) {
	now := time.Unix(0, 0)
	ax, ay := now, now
	later := now.Add(100 * time.Millisecond)
	x, y := moveStep(500, 450, 300, 350, 200, 100, throwRunSpeed, &ax, &ay, later)
	if x != 470 || y != 435 {
		t.Fatalf("(%d, %d), want (470, 435)", x, y)
	}
	ax, ay = now, now
	x, y = moveStep(500, 450, 495, 450, 5, 0, throwRunSpeed, &ax, &ay, later)
	if x != 495 || y != 450 {
		t.Fatalf("overshot: (%d, %d)", x, y)
	}
}

// TestThrowLines: the line-ups of FUN_001af02c and FUN_001aef04.
func TestThrowLines(t *testing.T) {
	g, _, _ := newThrowGame(t, true)
	if g.own[1].x != 625 || g.own[1].y != 475 || g.npc[2].x != 150 || g.npc[2].y != 500 {
		t.Fatalf("own %d,%d npc %d,%d", g.own[1].x, g.own[1].y, g.npc[2].x, g.npc[2].y)
	}
	if !g.own[0].controlled || g.own[1].controlled || g.own[0].lives != throwLives {
		t.Fatal("the player is not own[0]")
	}
}

// TestThrowCharge: a press on the player charges, the release throws on
// frame 3 toward power × reach + 230 to the left, and the ball lands.
func TestThrowCharge(t *testing.T) {
	g, now, _ := newThrowGame(t, false, 0)
	for _, n := range g.npc {
		n.controlled = true // keep the opponents still
	}
	p := g.me()
	g.Click(p.x, p.y-10, *now)
	if p.state != throwCharging {
		t.Fatalf("state %d", p.state)
	}
	g.frames(now, 30) // 900 ms: 9 points at 60 ms, then 30 ms ones
	if g.power < 9 || g.power > throwMaxPower {
		t.Fatalf("power %d", g.power)
	}
	power := g.power
	g.Release(p.x, p.y, *now)
	g.frames(now, 12)
	if g.live != 1 {
		t.Fatalf("live balls %d", g.live)
	}
	b := g.balls[0]
	reach := 25
	if power >= 11 {
		reach = 40
	}
	if b.tx != p.x-0x1e-power*reach-0xe6 || b.ty != p.y-0x32+0x32 {
		t.Fatalf("target (%d, %d) for power %d", b.tx, b.ty, power)
	}
	g.frames(now, 200)
	if g.live != 0 || g.balls[0].state == 1 {
		t.Fatal("the ball never landed")
	}
}

// TestThrowHit: an opponent's ball aimed at the player hits while passing
// and costs a life; losing every life loses three seconds later.
func TestThrowHit(t *testing.T) {
	g, now, results := newThrowGame(t, false)
	for _, n := range g.npc {
		n.controlled = true
	}
	p := g.me()
	p.lives = 1
	b := &g.balls[0]
	*b = ball{state: 1, side: throwSideNPC, x: p.x - 10, y: p.y - 20, tx: p.x, ty: p.y}
	g.live = 1
	g.frames(now, 1)
	if p.lives != 0 || b.state != 2 || p.action != throwDieRight {
		t.Fatalf("lives %d ball %d action %#x", p.lives, b.state, p.action)
	}
	g.frames(now, int(throwEndWait/throwFrame)+2)
	if len(*results) != 1 || (*results)[0] {
		t.Fatalf("results %v", *results)
	}
}

// TestThrowComputer: an opponent that picks a throw aims for a second,
// winds up and throws on frame 3; downing every opponent wins.
func TestThrowComputer(t *testing.T) {
	g, now, results := newThrowGame(t, false, 1, 0, 0, 0)
	o := g.npc[0]
	g.frames(now, 120) // a 1 s decision, 1 s of aiming, then the wind-up
	if g.live == 0 {
		t.Fatalf("no computer throw: state %d choice %d", o.state, o.choice)
	}
	for _, n := range g.npc {
		n.lives = 0
	}
	g.frames(now, int(throwEndWait/throwFrame)+2)
	if len(*results) != 1 || !(*results)[0] {
		t.Fatalf("results %v", *results)
	}
}
