// Package weather ports the client's weather layer (PTR_DAT_004ca2b0): the
// falling leaves, snow, rising steam and bubbles some maps show, and the
// overlays a movie's stage turns on. One layer serves the map and the
// movies, as in the original.
//
// The frame (FUN_004a1d60) paints the map: after the ground the under pass
// FUN_00334580 runs before the characters, and the over pass FUN_003346e8 after
// the effects list; a movie's draw (FUN_00340404) does the same with its
// overlay (+0xf9a) instead of the map's weather. Each pass runs the kinds
// whose test (FUN_00334120) holds for the scene, or the one kind named by
// the overlay.
package weather

import (
	"image"
	"math"
	"time"

	"wonderland-go/client/wlo/picdb"
	"wonderland-go/client/wlo/surface"
)

// Kind is a weather kind: the category FUN_00334120 tests and the
// overlay values a movie sets (+0xf9a).
type Kind byte

const (
	None    Kind = 0
	Leaves  Kind = 1  // icon_leaves (FUN_00333d80)
	Snow    Kind = 2  // icon_Snow1, icon_Snow2 (FUN_0033575c)
	Rain    Kind = 3  // icon_Rains (FUN_00336558), a dim fill and lightning
	Stars   Kind = 5  // icon_Star1 (FUN_00336afc), scene 11009 only
	Steam   Kind = 10 // icon_steam_1..4 (FUN_00335ed0)
	Leaves2 Kind = 11 // icon_leaves_2 (FUN_00337840)
	Bubbles Kind = 12 // Icon_Bubble_1, Icon_Bubble_2 (FUN_00337f04)
	Ribbons Kind = 13 // Icon_Ribbon1..6 (FUN_0033867c), while +0x57f2 is set
)

// SceneKind is FUN_00334120's test of the scene record's weather byte
// (SceneData.dat +0x29): 1 leaves, 2 snow, 3 steam, 4 rain, 5 the second
// leaves, 6 bubbles, 7 ribbons.
func SceneKind(b byte) Kind {
	switch b {
	case 1:
		return Leaves
	case 2:
		return Snow
	case 3:
		return Steam
	case 4:
		return Rain
	case 5:
		return Leaves2
	case 6:
		return Bubbles
	case 7:
		return Ribbons
	}
	return None
}

// The rain's fills (ro_ARGB through ro_Clipper_Alpha_Fill over the whole
// 800×600 screen): the under pass dims the scene, and the over pass
// flashes it when Random(400) < 10.
const (
	rainDimAlpha   = 0xc8
	rainDimRGB     = 0x28
	flashAlpha     = 0xc8
	flashRGB       = 0xc8
	flashOdds      = 400
	flashThreshold = 10
)

// Spawn area: a particle starts within the 800-pixel screen width (1000
// for bubbles), at the top (or 500 and 800 below it for steam and
// bubbles), and heads 600 pixels down (or up) and a few 50-pixel columns
// aside.
const (
	spawnWidth       = 800
	bubbleSpawnWidth = 1000
	fallDepth        = 600
	driftColumns     = 5
	driftColumn      = 0x32
	steamRise        = 500
	steamSpread      = 100
	bubbleRise       = 800
	bubbleSpread     = 100
	bubbleTop        = 300
)

// Movement (FUN_003339e0 and its siblings): each frame the particle moves
// round(elapsed × speed) pixels along its longer axis and proportionally
// along the other, where elapsed counts from its last animation frame;
// once the step passes both distances it lands on the target.
const moveEpsilon = 0.001 // the tbyte at 0x333c18

// spec is one kind's pool (the weather object's arrays) and parameters.
type spec struct {
	slots    int           // pool size (1-based arrays in the original)
	interval time.Duration // between spawns (one per interval)
	frames   int           // frames stacked vertically in each picture
	speeds   [4]float64    // px/ms by class 1..3, [0] for the others
	swayStep time.Duration // the sway's tick (0 every frame)
	swayHold time.Duration // the sway's pause at its extremes
	light    int           // light level for the additive blit, 0 for the colour-key blit
}

var specs = map[Kind]spec{
	Leaves:  {slots: 8, interval: 900 * time.Millisecond, frames: 5, speeds: [4]float64{0.015, 0.01, 0.015, 0.02}, swayStep: 50 * time.Millisecond, swayHold: 200 * time.Millisecond},
	Leaves2: {slots: 8, interval: 900 * time.Millisecond, frames: 5, speeds: [4]float64{0.015, 0.01, 0.015, 0.02}, swayStep: 50 * time.Millisecond, swayHold: 200 * time.Millisecond},
	Snow:    {slots: 100, interval: 500 * time.Millisecond, frames: 4, speeds: [4]float64{0.015, 0.01, 0.015, 0.02}},
	Steam:   {slots: 200, interval: 200 * time.Millisecond, frames: 10, speeds: [4]float64{0.007, 0.005, 0.007, 0.009}, light: 10},
	Bubbles: {slots: 10, interval: 200 * time.Millisecond, frames: 4, speeds: [4]float64{0.02, 0.02, 0.015, 0.02}},
}

// Picture names by kind; a particle's picture is one of them.
var pictures = map[Kind][]string{
	Leaves:  {"icon_leaves"},
	Leaves2: {"icon_leaves_2"},
	Snow:    {"icon_Snow1", "icon_Snow2", "icon_Snow3"},
	Steam:   {"icon_steam_1", "icon_steam_2", "icon_steam_3", "icon_steam_4"},
	Bubbles: {"Icon_Bubble_1", "Icon_Bubble_2"},
}

// particle is one record of a pool.
type particle struct {
	live   bool // +4
	landed bool
	pic    int // picture index in the DB, -1 when absent
	w, fh  int // width and frame height
	frame  int
	frames int
	every  time.Duration // between frames
	speed  float64
	x, y   int // map pixels
	tx, ty int
	at     time.Time // the last frame's time, the movement's base
	// The sway (FUN_00333c24): an offset added to x, growing by a quarter
	// of its amplitude per tick to the amplitude, held, and back to 0,
	// then the same to the other side.
	sway, amp int
	swayRight bool
	swayPhase int
}

// Layer is the weather object.
type Layer struct {
	Pics *picdb.DB
	// Random is the client's Random (System.Random), shared with the rest
	// of the client in the original.
	Random func(n int) int

	pools   map[Kind][]particle
	spawned map[Kind]time.Time
	frameAt map[Kind]time.Time
}

// New makes an empty layer.
func New(pics *picdb.DB, random func(int) int) *Layer {
	return &Layer{Pics: pics, Random: random, pools: map[Kind][]particle{}, spawned: map[Kind]time.Time{}, frameAt: map[Kind]time.Time{}}
}

// Frame is the original's paint period (the 30 ms multimedia timer): the
// weather moves once per painted frame.
const Frame = 30 * time.Millisecond

const maxCatchUp = 10

// Under is FUN_00334580 for one kind: the frames due by now are run, then
// what lies under the characters is drawn (the rain's dim fill; landed
// particles are removed in the frame they land, so none are drawn).
func (l *Layer) Under(dst *surface.Surface, k Kind, cam image.Point, now time.Time) {
	if k == None {
		return
	}
	l.run(k, cam, now)
	if k == Rain {
		dst.FillAlpha(image.Rect(0, 0, dst.W, dst.H), rgb(rainDimRGB), rainDimAlpha)
	}
}

// Over is FUN_003346e8 for one kind: the falling particles and the rain's
// lightning.
func (l *Layer) Over(dst *surface.Surface, k Kind, cam image.Point) {
	if k == Rain && l.Random(flashOdds) < flashThreshold {
		dst.FillAlpha(image.Rect(0, 0, dst.W, dst.H), rgb(flashRGB), flashAlpha)
	}
	sp, ok := specs[k]
	if !ok {
		return
	}
	for i := range l.pools[k] {
		p := &l.pools[k][i]
		if !p.live || p.landed || p.pic < 0 {
			continue
		}
		x, y := p.x+p.sway-cam.X, p.y-cam.Y
		r := image.Rect(0, p.frame*p.fh, p.w, (p.frame+1)*p.fh)
		if sp.light > 0 {
			l.Pics.DrawLight(dst, p.pic, x, y, r, sp.light)
		} else {
			l.Pics.DrawRect(dst, p.pic, x, y, r, true)
		}
	}
}

func rgb(v uint32) uint32 { return v | v<<8 | v<<16 }

// run runs the frames due by now.
func (l *Layer) run(k Kind, cam image.Point, now time.Time) {
	at := l.frameAt[k]
	if at.IsZero() || now.Sub(at) > maxCatchUp*Frame {
		at = now.Add(-Frame)
	}
	for now.Sub(at) >= Frame {
		at = at.Add(Frame)
		l.step(k, cam, at)
	}
	l.frameAt[k] = at
}

// step is one frame of a kind (FUN_00334ebc and its siblings): a spawn
// when the interval has passed, then every live particle moves; landed
// ones are freed.
func (l *Layer) step(k Kind, cam image.Point, now time.Time) {
	sp, ok := specs[k]
	if !ok {
		return
	}
	pool := l.pools[k]
	if pool == nil {
		pool = make([]particle, sp.slots)
		l.pools[k] = pool
	}
	if now.Sub(l.spawned[k]) >= sp.interval {
		for i := range pool {
			if !pool[i].live {
				pool[i] = l.spawn(k, sp, cam, now)
				l.spawned[k] = now
				break
			}
		}
	}
	for i := range pool {
		p := &pool[i]
		if !p.live {
			continue
		}
		p.move(now)
		if p.landed {
			p.live = false
			continue
		}
		p.swing(sp, now)
	}
}

// spawn sets up a particle (FUN_00333d80, FUN_0033575c, FUN_00335ed0,
// FUN_00337840, FUN_00337f04).
func (l *Layer) spawn(k Kind, sp spec, cam image.Point, now time.Time) particle {
	rnd := l.Random
	p := particle{live: true, at: now, swayRight: true, frames: sp.frames}
	// Bubbles change frames every 200..220 ms, the others 200..400.
	scale := 100
	if k == Bubbles {
		scale = 10
	}
	p.every = time.Duration(rnd(3)*scale+200) * time.Millisecond
	class := rnd(3) + 1
	pic := 0
	switch k {
	case Leaves, Leaves2:
		p.x = rnd(spawnWidth) + cam.X
		p.y = cam.Y
		p.ty = cam.Y + fallDepth
		p.tx = drift(rnd, p.x)
		p.amp = rnd(0x1e)
	case Snow:
		p.x = rnd(spawnWidth) + cam.X
		p.y = cam.Y
		p.ty = cam.Y + fallDepth
		// Random(100) < 60 picks the first flake (the < 90 test inside
		// is always true, so icon_Snow3 is never used), else the second.
		if rnd(100) >= 60 {
			pic = 1
		}
		p.tx = drift(rnd, p.x)
		p.amp = rnd(0x14)
	case Steam:
		p.x = rnd(spawnWidth) + cam.X
		p.y = rnd(steamSpread) + cam.Y + steamRise
		p.ty = cam.Y
		pic = rnd(4)
		p.tx = drift(rnd, p.x)
		p.amp = rnd(10)
	case Bubbles:
		p.x = rnd(bubbleSpawnWidth) + cam.X
		p.y = rnd(bubbleSpread) + cam.Y + bubbleRise
		p.ty = cam.Y - rnd(bubbleTop)
		p.tx = p.x
		// The speed follows the picture (+0x10), not the class.
		pic = 1
		if rnd(100) < 60 {
			pic = 0
		}
		class = pic + 1
		rnd(0x14) // +0x3c, unused: bubbles do not sway
	}
	p.speed = sp.speeds[0]
	if class < len(sp.speeds) {
		p.speed = sp.speeds[class]
	}
	names := pictures[k]
	p.pic = l.Pics.Find(names[pic])
	if p.pic >= 0 {
		w, h := l.Pics.Size(p.pic)
		p.w, p.fh = w, h/sp.frames
	}
	return p
}

// drift is the target column: Random(2) picks a side, Random(5) columns
// of 50 pixels.
func drift(rnd func(int) int, x int) int {
	if rnd(2) == 0 {
		return x + rnd(driftColumns)*driftColumn
	}
	return x - rnd(driftColumns)*driftColumn
}

// move is FUN_003339e0 (and FUN_003353c4, FUN_00335b38, FUN_00337b94).
func (p *particle) move(now time.Time) {
	step := float64(roundInt(float64(now.Sub(p.at).Milliseconds()) * p.speed))
	dx, dy := float64(abs(p.tx-p.x)), float64(abs(p.ty-p.y))
	if step > dx && step > dy {
		p.x, p.y, p.landed = p.tx, p.ty, true
		return
	}
	var rx, ry float64
	if dy > dx {
		rx = dy / (moveEpsilon + dx)
	} else {
		ry = dx / (moveEpsilon + dy)
	}
	sx := min(step/(moveEpsilon+rx), step)
	sy := min(step/(moveEpsilon+ry), step)
	p.x += sign(p.tx-p.x) * roundInt(sx)
	p.y += sign(p.ty-p.y) * roundInt(sy)
	if now.Sub(p.at) >= p.every {
		p.frame++
		if p.frame >= p.frames {
			p.frame = 0
		}
		p.at = now
	}
}

// swing is FUN_00333c24 (and FUN_00335608, FUN_00335d7c): leaves tick every
// 50 ms of the frame and hold 200 ms; snow and steam tick every frame and
// do not hold.
func (p *particle) swing(sp spec, now time.Time) {
	if now.Sub(p.at) < sp.swayStep {
		return
	}
	q := p.amp / 4
	switch p.swayPhase {
	case 0:
		if p.swayRight {
			p.sway += q
			if p.sway > p.amp {
				p.swayPhase, p.sway = 1, p.amp
			}
		} else {
			p.sway -= q
			if p.sway < -p.amp {
				p.swayPhase, p.sway = 1, -p.amp
			}
		}
	case 1:
		if now.Sub(p.at) >= sp.swayHold {
			p.swayPhase = 2
		}
	case 2:
		if p.swayRight {
			p.sway -= q
			if p.sway < 0 {
				p.sway, p.swayRight, p.swayPhase = 0, false, 0
			}
		} else {
			p.sway += q
			if p.sway >= 0 {
				p.sway, p.swayRight, p.swayPhase = 0, true, 0
			}
		}
	}
}

func roundInt(v float64) int { return int(math.RoundToEven(v)) }

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
