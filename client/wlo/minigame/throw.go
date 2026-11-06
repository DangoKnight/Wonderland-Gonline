package minigame

import (
	"image"
	"math"
	"sort"
	"time"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/surface"
)

// Throw ports the dodgeball game, sport kind 1 (constructor FUN_001ae788,
// set-up FUN_001ae968, update FUN_001af7d8, draw FUN_001af424). The
// player's side stands on the right of a battle scene, the launch
// template's NPCs on the left; everyone has three lives. Holding the
// button on the player charges a throw, releasing it throws, and a press
// elsewhere in the right court runs there. Teammates and opponents throw
// and run on their own. The game ends three seconds after the player or
// the whole other side is down (FUN_001b1cbc).
type Throw struct {
	Round
	// Background draws the battle scene (FUN_001af700).
	Background func(dst *surface.Surface)
	// Shadow draws a figure's ground shadow.
	Shadow func(dst *surface.Surface, x, y int)

	own, npc   []*thrower // +0x08.., +0x1c..
	player     int        // +0x68, an index into own
	ownAI      bool       // +0x8c: teammates act by themselves
	running    bool       // +0xebc: Start was pressed
	playing    bool       // +0x2e: 1 until someone has lost
	overAt     time.Time  // +0xe60
	won        bool       // +0xe74
	balls      [throwBalls]ball
	ring, live int // +0xbe6, +0xbe5
	// The player's controls.
	down, up   bool // +0x8d, +0x8e
	target     image.Point
	power      int // +0x94
	powerAt    time.Time
	thrown     bool // +0x8f
	thrownAt   time.Time
	frameAt    time.Time
	musicStart bool // +0xebd
}

// Figure is a thrower's sprite.
type Figure interface {
	Draw(dst *surface.Surface, x, y, action int)
	Hold(frame int, wrap bool)
	Height() int // +0x114, for the press on the player
}

// thrower is one figure and its fields in the game object.
type thrower struct {
	fig            Figure
	x, y, tx, ty   int // +0x8c, +0x90, +0x84, +0x88
	action         int // +0x121
	hold           int // +0x11f: a held frame, throwAnimate to animate
	frame          int // +0x11e
	frameAt        time.Time
	lives          int  // +0xd1a / +0xd10
	state          byte // +0xd15 / +0xbec
	choice         int  // +0xd1f / +0xbe7
	decideAt       time.Time
	decideEvery    time.Duration // +0xd50 / +0xcfc
	stateAt, hurt  time.Time
	dx, dy         int // +0xd90/+0xda4 or +0xcd4/+0xce8
	stepX, stepY   time.Time
	side           int
	controlled     bool
	thrownFrameSet bool
}

type ball struct {
	state                byte // +0x110: 0 free, 1 flying, 2 hit, 3 missed
	x, y, tx, ty, dx, dy int
	height, length       int // +0x108, +0x10c
	side                 int // +0x121: 1 thrown by the right side
	stepX, stepY         time.Time
	effect               Effect // +0x122
}

const (
	ThrowKind = 1

	throwBalls      = 50
	throwLives      = 3
	throwTeam       = 4
	throwAnimate    = 0xff
	throwSideOwn    = 1
	throwSideNPC    = 2
	throwRunSpeed   = 0.3 // px/ms, FUN_001afd70 / FUN_001b0f74
	throwBallSpeed  = 0.5 // FUN_001b0688
	throwEpsilon    = 0.0001
	throwArcEpsilon = 0.001
	throwFrameTime  = 100 * time.Millisecond
	throwEndWait    = 3 * time.Second
	throwHurtTime   = time.Second
	throwThinkTime  = time.Second
	throwPostFrame  = 100 * time.Millisecond
	throwChoices    = 7
	throwMaxPower   = 15
	throwPowerSlow  = 60 * time.Millisecond
	throwPowerFast  = 30 * time.Millisecond
	throwPowerKnee  = 9
	throwReachX     = 0x14
	throwReachTop   = 0x1e
	throwFrame      = 30 * time.Millisecond
	// throwDefaultHeight stands in for a missing sprite's height (+0x114).
	throwDefaultHeight = 0x50

	// Actions (+0x121).
	throwStandRight = 0x10
	throwStandLeft  = 0x11
	throwDieRight   = 0x1a
	throwDieLeft    = 0x1b
	throwHurtRight  = 0x1e
	throwHurtLeft   = 0x1f
	throwArmRight   = 0x22
	throwArmLeft    = 0x23
	throwRunLeft    = 0x40
	throwRunRight   = 0x41
	throwRelease    = 3 // the throw leaves the hand on this frame

	// States (+0xd15 / +0xbec).
	throwIdle     = 0
	throwCharging = 1
	throwWantRun  = 2
	throwHurt     = 3
	throwRunning  = 5
	throwWindUp   = 6
)

// The courts (+0x6c and +0x7c) and the line-ups of FUN_001af02c and
// FUN_001aef04.
var (
	throwOwnCourt = image.Rect(500, 0x1ae, 800, 0x226)
	throwNPCCourt = image.Rect(0, 0x1ae, 300, 0x226)
)

// Pictures (FUN_001ae968).
const (
	throwHeart      = "Icon_Heart"
	throwBall       = "Icon_Element_3_1"
	throwHitFX      = "S10203"
	throwMissFX     = "S10004"
	throwBar        = "Str_Bar"
	ThrowExitButton = "ThrowGame_Exit_Btn"
)

// NewThrow lines up the player's side (own[player] is the player) and the
// opponents; ownAI makes the player's teammates play by themselves (no
// party, +0x8c).
func NewThrow(pics *picdb.DB, own []Figure, player int, npc []Figure, ownAI bool, now time.Time) *Throw {
	g := &Throw{Round: newRound(pics), player: player, ownAI: ownAI, playing: true, frameAt: now}
	for i, f := range own {
		if i >= throwTeam {
			break
		}
		x, y := 600+i*0x19, 0x1c2+i*0x19
		g.own = append(g.own, &thrower{fig: f, x: x, y: y, tx: x, ty: y, action: throwStandRight, hold: throwAnimate, frameAt: now,
			lives: throwLives, side: throwSideOwn, controlled: i == player, decideEvery: time.Second, decideAt: now})
	}
	for i, f := range npc {
		if i >= throwTeam {
			break
		}
		x, y := 200-i*0x19, 0x1c2+i*0x19
		g.npc = append(g.npc, &thrower{fig: f, x: x, y: y, tx: x, ty: y, action: throwStandLeft, hold: throwAnimate, frameAt: now,
			lives: throwLives, side: throwSideNPC, decideEvery: time.Second, decideAt: now})
	}
	return g
}

func (g *Throw) Pictures() []string {
	return []string{throwHeart, throwBall, throwHitFX, throwMissFX, throwBar}
}

// Begin is the start form's Start (0x1b4fb0): the game runs.
func (g *Throw) Begin(now time.Time) {
	g.Started, g.running, g.startedAt = true, true, now
	for _, t := range g.all() {
		t.decideAt = now
	}
}

func (g *Throw) all() []*thrower { return append(append([]*thrower(nil), g.own...), g.npc...) }

func (g *Throw) me() *thrower {
	if g.player < 0 || g.player >= len(g.own) {
		return nil
	}
	return g.own[g.player]
}

func (g *Throw) rand(n int) int {
	if n <= 0 || g.Rand == nil {
		return 0
	}
	return g.Rand(n)
}

// elapsed is FUN_001aee84: whether d has passed since *at, restarting it
// when so.
func elapsed(at *time.Time, d time.Duration, now time.Time) bool {
	if now.Sub(*at) >= d {
		*at = now
		return true
	}
	return false
}

// roundEven is Delphi's Round (the FPU's round half to even).
func roundEven(v float64) int { return int(math.RoundToEven(v)) }

// moveStep is FUN_001aff54: each axis moves speed × the milliseconds since
// its own clock toward the target, the longer axis at full speed and the
// shorter one in proportion.
func moveStep(x, y, tx, ty, dx, dy int, speed float64, stepX, stepY *time.Time, now time.Time) (int, int) {
	axis := func(v, t, mine, other int, at *time.Time, full bool) int {
		step := roundEven(float64(now.Sub(*at).Milliseconds()) * speed)
		*at = now
		if !full {
			step = roundEven(float64(mine) / (float64(other) + throwEpsilon) * float64(step))
		}
		switch {
		case v < t:
			return min(v+step, t)
		case v > t:
			return max(v-step, t)
		}
		return v
	}
	nx := axis(x, tx, dx, dy, stepX, dx >= dy)
	ny := axis(y, ty, dy, dx, stepY, dx < dy)
	return nx, ny
}

// arrived is FUN_001b0148 / FUN_001b1a70: the longer axis is done.
func arrived(t *thrower) bool {
	if t.dx < t.dy {
		return t.y == t.ty
	}
	return t.x == t.tx
}

// aim is FUN_001afedc / FUN_001b195c.
func (t *thrower) aim(tx, ty int) {
	t.tx, t.ty = tx, ty
	t.dx, t.dy = abs(t.x-tx), abs(t.y-ty)
}

func (t *thrower) pose(action, hold int, now time.Time) {
	if action != t.action {
		t.frame, t.frameAt = 0, now
	} else if hold == throwAnimate && t.hold != throwAnimate {
		t.frameAt = now // a held frame starts animating from where it is
	}
	t.action, t.hold = action, hold
	if hold != throwAnimate {
		t.frame = hold
	}
}

func (t *thrower) stand(now time.Time) {
	a := throwStandRight
	if t.side == throwSideNPC {
		a = throwStandLeft
	}
	t.pose(a, throwAnimate, now)
}

// animate advances an animating figure's frame (+0x11e) at the roles'
// 100 ms rate.
func (t *thrower) animate(now time.Time) {
	if t.hold != throwAnimate {
		t.frame = t.hold
		return
	}
	if n := int(now.Sub(t.frameAt) / throwFrameTime); n > 0 {
		t.frame += n
		t.frameAt = t.frameAt.Add(time.Duration(n) * throwFrameTime)
	}
}

// Click is the press on the ground (FUN_001af944).
func (g *Throw) Click(x, y int, now time.Time) {
	p := g.me()
	if !g.running || p == nil || p.state != throwIdle {
		return
	}
	height := throwDefaultHeight
	if p.fig != nil {
		height = p.fig.Height()
	}
	switch {
	case abs(x-p.x) <= throwReachX && y >= p.y-height+throwReachTop && y <= p.y:
		p.state, g.powerAt, g.power = throwCharging, now, 0
	case image.Pt(x, y).In(image.Rect(throwOwnCourt.Min.X, throwOwnCourt.Min.Y, throwOwnCourt.Max.X+1, throwOwnCourt.Max.Y+1)):
		p.state = throwWantRun
	default:
		p.state = throwIdle
	}
	g.down, g.up, g.target = true, false, image.Pt(x, y)
}

// Release is the button's release (FUN_001afa90).
func (g *Throw) Release(x, y int, _ time.Time) {
	p := g.me()
	if p == nil {
		return
	}
	g.down, g.up = false, true
	if p.state == throwIdle {
		g.target = image.Pt(x, y)
	}
}

// Update runs FUN_001af7d8 on the 30 ms game frame.
func (g *Throw) Update(now time.Time) {
	if now.Sub(g.frameAt) > time.Second {
		g.frameAt = now.Add(-throwFrame)
	}
	for !now.Before(g.frameAt.Add(throwFrame)) {
		g.frameAt = g.frameAt.Add(throwFrame)
		g.step(g.frameAt)
	}
}

func (g *Throw) step(now time.Time) {
	for _, t := range g.all() {
		t.animate(now)
	}
	if !g.running || g.Done() {
		return
	}
	if !g.musicStart {
		g.musicStart = true
		if g.Music != nil {
			g.Music("Sound\\BGM0018.wav")
		}
	}
	g.charge(now)
	g.releaseThrow(now)
	g.afterThrow(now)
	g.runPlayer(now)
	g.hits(now)
	g.think(g.npc, now)
	if g.ownAI {
		g.think(g.own, now)
	}
	for _, t := range g.own {
		if t.state == throwHurt && elapsed(&t.hurt, throwHurtTime, now) {
			t.stand(now)
			t.state = throwIdle
		}
	}
	g.flyBalls(now)
	g.result(now)
}

// charge is FUN_001afadc: power rises every 60 ms, then every 30 ms.
func (g *Throw) charge(now time.Time) {
	p := g.me()
	if p == nil || p.lives == 0 || p.state != throwCharging || !g.down {
		return
	}
	p.pose(throwArmRight, 0, now)
	every := throwPowerSlow
	if g.power >= throwPowerKnee {
		every = throwPowerFast
	}
	if elapsed(&g.powerAt, every, now) {
		g.power = min(g.power+1, throwMaxPower)
	}
}

// releaseThrow is FUN_001afb60: a release while charging plays the throw
// and lets go on frame 3; a press in the court starts the run.
func (g *Throw) releaseThrow(now time.Time) {
	p := g.me()
	if p == nil || p.lives == 0 {
		return
	}
	if !g.up {
		if g.down && p.state == throwWantRun {
			p.aim(g.target.X, g.target.Y)
			p.state, p.stepX, p.stepY = throwRunning, now, now
		}
		return
	}
	if p.state == throwCharging {
		p.pose(throwArmRight, throwAnimate, now)
		if p.frame == throwRelease {
			g.launch(p, throwSideOwn, g.power, now)
			p.hold = throwRelease
			g.thrown, g.power, g.thrownAt = true, 0, now
			p.state = throwIdle
		}
	}
}

// afterThrow is FUN_001afca0: the arm comes back a frame every 100 ms.
func (g *Throw) afterThrow(now time.Time) {
	p := g.me()
	if p == nil {
		return
	}
	if p.state == throwWantRun || p.state == throwRunning {
		g.thrown = false
		return
	}
	if !g.thrown {
		return
	}
	p.action = throwArmRight
	if elapsed(&g.thrownAt, throwPostFrame, now) {
		if p.hold == 0 || p.hold == throwAnimate {
			p.stand(now)
			p.state, g.thrown = throwIdle, false
		} else {
			p.hold--
			p.frame = p.hold
		}
	}
}

// runPlayer is FUN_001afd70.
func (g *Throw) runPlayer(now time.Time) {
	p := g.me()
	if p == nil || p.state != throwRunning {
		return
	}
	g.run(p, now)
	if arrived(p) {
		p.state = throwIdle
		p.stand(now)
		if !g.up && g.down {
			p.state = throwCharging
		}
	}
}

func (g *Throw) run(t *thrower, now time.Time) {
	t.x, t.y = moveStep(t.x, t.y, t.tx, t.ty, t.dx, t.dy, throwRunSpeed, &t.stepX, &t.stepY, now)
	if t.tx < t.x {
		t.pose(throwRunLeft, throwAnimate, now)
	} else {
		t.pose(throwRunRight, throwAnimate, now)
	}
}

// think is FUN_001b0b38 for one side's computer-run figures.
func (g *Throw) think(side []*thrower, now time.Time) {
	for _, t := range side {
		if t.controlled || t.lives == 0 {
			continue
		}
		if t.state == throwHurt {
			// FUN_001b1618.
			if elapsed(&t.hurt, throwHurtTime, now) {
				t.state = throwIdle
				t.decideAt = now
				t.decideEvery = time.Duration(500+g.rand(500)) * time.Millisecond
				t.stand(now)
			}
			continue
		}
		if t.state == throwIdle {
			t.choice = 5
			if elapsed(&t.decideAt, t.decideEvery, now) {
				t.choice = g.rand(throwChoices)
			}
		}
		switch t.choice {
		case 1, 6:
			g.aiThrow(t, now)
		case 2, 3:
			g.aiRun(t, now)
		}
	}
}

func (g *Throw) armAction(t *thrower) int {
	if t.side == throwSideNPC {
		return throwArmLeft
	}
	return throwArmRight
}

// aiThrow is FUN_001b0cf0: aim for a second, wind up, throw on frame 3.
func (g *Throw) aiThrow(t *thrower, now time.Time) {
	switch t.state {
	case throwIdle:
		t.pose(g.armAction(t), 0, now)
		t.state, t.stateAt = throwCharging, now
	case throwCharging:
		if elapsed(&t.stateAt, throwThinkTime, now) {
			t.state = throwWindUp
			t.pose(g.armAction(t), throwAnimate, now)
		}
	case throwWindUp:
		if t.frame == throwRelease {
			g.launch(t, t.side, g.rand(throwMaxPower+1), now)
			t.stand(now)
			t.state = throwIdle
			t.decideEvery = time.Duration(1000+g.rand(1000)) * time.Millisecond
			t.decideAt = now
		}
	}
}

// aiRun is FUN_001b0f74: run to a random spot of the own court.
func (g *Throw) aiRun(t *thrower, now time.Time) {
	switch t.state {
	case throwIdle:
		court := throwOwnCourt
		x := court.Min.X + g.rand(court.Dx())
		if t.side == throwSideNPC {
			court = throwNPCCourt
			x = g.rand(court.Max.X)
		}
		y := max(court.Min.Y+g.rand(court.Dy()), court.Min.Y)
		t.aim(x, y)
		t.state, t.stepX, t.stepY = throwWantRun, now, now
	case throwWantRun:
		g.run(t, now)
		if arrived(t) {
			t.state = throwIdle
			t.stand(now)
			t.decideEvery = time.Duration(1000+g.rand(1000)) * time.Millisecond
			t.decideAt = now
		}
	}
}

// launch is FUN_001b01b4 and FUN_001b03b0: a ball leaves the thrower's
// hand toward a point power × 15, 25 or 40 pixels plus 230 away, or
// toward a random opponent for the computer.
func (g *Throw) launch(t *thrower, side, power int, now time.Time) {
	if g.live >= throwBalls+1 {
		return
	}
	b := &g.balls[g.ring]
	*b = ball{state: 1, side: side, stepX: now, stepY: now}
	reach := 15
	switch {
	case power >= 11:
		reach = 40
	case power >= 6:
		reach = 25
	}
	if side == throwSideOwn {
		b.x, b.y = t.x-0x1e, t.y-0x32
		b.tx, b.ty = b.x-power*reach-0xe6, b.y+0x32
		if !t.controlled && g.rand(3) == 1 && len(g.npc) > 0 {
			o := g.npc[g.rand(len(g.npc))]
			b.tx, b.ty = o.x, o.y
		}
	} else {
		b.x, b.y = t.x+0x1e, t.y-0x32
		if g.rand(3) < 2 && len(g.own) > 0 {
			o := g.own[g.rand(len(g.own))]
			b.tx, b.ty = o.x, o.y
		} else {
			b.tx, b.ty = b.x+power*reach+0xe6, b.y+0x32
		}
	}
	b.dx, b.dy = abs(b.x-b.tx), abs(b.y-b.ty)
	b.length = roundEven(math.Sqrt(float64(b.dx*b.dx + b.dy*b.dy)))
	g.ring = (g.ring + 1) % throwBalls
	g.live++
	g.sound("Sound\\Wav1608.Wav")
}

// flyBalls is FUN_001b0688 and FUN_001b0824: the balls fly at 0.5 px/ms
// on the leap arc and miss where they land.
func (g *Throw) flyBalls(now time.Time) {
	for i := range g.balls {
		b := &g.balls[i]
		if b.state != 1 {
			continue
		}
		b.x, b.y = moveStep(b.x, b.y, b.tx, b.ty, b.dx, b.dy, throwBallSpeed, &b.stepX, &b.stepY, now)
		left := roundEven(math.Sqrt(float64((b.tx-b.x)*(b.tx-b.x) + (b.ty-b.y)*(b.ty-b.y))))
		a := roundEven((0.5 - math.Abs(float64(left)/(float64(b.length)+throwArcEpsilon)-0.5)) * 360)
		h := roundEven(float64(a) - 0.003*float64(a)*float64(a))
		b.height = 2 * h
		if (b.side == throwSideOwn && b.x <= b.tx) || (b.side == throwSideNPC && b.tx <= b.x) {
			g.sound("Sound\\SEB0051.Wav")
			b.effect.Play(throwMissFX, 70*time.Millisecond, 0x1e, b.x+10, b.y+100, 12, 12, 1, now)
			b.x, b.y, b.tx, b.ty = 0, 0, 0, 0
			b.state = 3
			if g.live > 0 {
				g.live--
			}
		}
	}
}

// hits is FUN_001b1338 with FUN_001b1700: a ball hits whoever stands
// where it is aimed while it passes them.
func (g *Throw) hits(now time.Time) {
	for i := range g.balls {
		b := &g.balls[i]
		if b.state != 1 {
			continue
		}
		victims, hurt, die := g.npc, throwHurtLeft, throwDieLeft
		back, ahead := 0x14, 0x46
		if b.side == throwSideNPC {
			victims, hurt, die = g.own, throwHurtRight, throwDieRight
			back, ahead = 0x46, 0x14
		}
		for _, t := range victims {
			if t.state == throwHurt || t.lives == 0 {
				continue
			}
			if t.y < b.ty-0xf || t.y > b.ty+0xf || t.x < b.tx-back || t.x > b.tx+ahead ||
				b.x < t.x-0x14 || b.x > t.x+0x14 || b.y < t.y-0x46 || b.y > t.y+0xf {
				continue
			}
			t.pose(hurt, throwAnimate, now)
			t.state, t.hurt = throwHurt, now
			t.lives--
			b.state = 2
			b.effect.Play(throwHitFX, 70*time.Millisecond, 0x1e, b.x+10, b.y, 7, 7, 1, now)
			if t.lives == 0 {
				t.pose(die, throwAnimate, now)
			}
			if g.live > 0 {
				g.live--
			}
			g.sound("Sound\\SEB0036.Wav")
		}
	}
}

// result is FUN_001b1cbc.
func (g *Throw) result(now time.Time) {
	if !g.playing {
		if now.Sub(g.overAt) >= throwEndWait {
			g.End(g.won)
		}
		return
	}
	if p := g.me(); p != nil && p.lives == 0 {
		g.playing, g.won, g.overAt = false, false, now
		return
	}
	for _, t := range g.npc {
		if t.lives != 0 {
			return
		}
	}
	g.playing, g.won, g.overAt = false, true, now
}

// Draw is FUN_001af424: the scene, then figures and balls by depth, each
// figure with its hearts, then the power bar.
func (g *Throw) Draw(dst *surface.Surface, _, _ int, now time.Time) {
	dst.Fill(image.Rect(0, 0, dst.W, dst.H), 0)
	if g.Background != nil {
		g.Background(dst)
	}
	type item struct {
		y int
		t *thrower
		b *ball
	}
	var items []item
	for _, t := range g.all() {
		items = append(items, item{y: t.y, t: t})
	}
	for i := range g.balls {
		items = append(items, item{y: g.balls[i].ty, b: &g.balls[i]})
	}
	// FUN_001b0a10's exchange sort keeps equal keys in their order.
	sort.SliceStable(items, func(i, j int) bool { return items[i].y < items[j].y })
	for _, it := range items {
		if it.t != nil {
			g.drawThrower(dst, it.t)
			continue
		}
		b := it.b
		switch b.state {
		case 1:
			g.picture(dst, throwBall, b.x, b.y-b.height)
		case 2, 3:
			if g.Pics != nil {
				b.effect.Draw(dst, g.Pics, now)
			}
			if !b.effect.Active() {
				b.state = 0
			}
		}
	}
	g.drawPower(dst)
}

func (g *Throw) drawThrower(dst *surface.Surface, t *thrower) {
	die := throwDieRight
	if t.side == throwSideNPC {
		die = throwDieLeft
	}
	if t.lives == 0 && t.action != die {
		t.action, t.hold = die, throwAnimate
	}
	if t.action == die && t.frame >= 1 {
		t.hold, t.frame = 1, 1
	}
	if g.Shadow != nil {
		g.Shadow(dst, t.x, t.y)
	}
	if t.fig != nil {
		t.fig.Hold(t.frame, true)
		t.fig.Draw(dst, t.x, t.y, t.action)
	}
	// FUN_001b1b0c: a heart for each life above the head.
	for i := 0; i < t.lives && i < throwLives; i++ {
		g.picture(dst, throwHeart, t.x-0xf+i*0xf, t.y-0x5a)
	}
}

// drawPower is FUN_001b188c: while charging, the bar's right half fills
// two pixels a point beside the left half's frame.
func (g *Throw) drawPower(dst *surface.Surface) {
	p := g.me()
	if p == nil || p.state != throwCharging || g.Pics == nil {
		return
	}
	i := g.Pics.Find(throwBar)
	if i < 0 {
		return
	}
	fill := 0x1e - 2*g.power
	g.Pics.DrawRect(dst, i, p.x+0x14, p.y-0x14-2*g.power, image.Rect(6, fill, 0xc, 0x1e), true)
	g.Pics.DrawRect(dst, i, p.x+0x14, p.y-0x32, image.Rect(0, 0, 6, 0x1e), true)
}
