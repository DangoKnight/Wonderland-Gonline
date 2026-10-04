package minigame

import (
	"time"

	"wonderland-go/client/wlo/cursor"
	"wonderland-go/client/wlo/picdb"
	"wonderland-go/client/wlo/surface"
)

// Mole is minigame 3, hitting moles (the object FUN_001b3090 creates at
// PTR_DAT_004c9b10). Seven holes on the "HitMouse" picture; after a
// countdown the player has 40 seconds. Moles (or rabbits, or turtles) and
// bombs rise from the holes, stay up, and sink again; hitting a mole scores
// one, hitting a bomb costs three. 30 or more wins. The game runs only
// after the start form's Start (+0x288), and each frame (FUN_001b4820) runs
// the clock, the difficulty, the spawning, the holes, the hammer cursor,
// the countdown and the end in that order.
type Mole struct {
	Pics    *picdb.DB
	Variant int // +0x2a0: VariantMouse, VariantRabbit, VariantTurtle
	Started bool
	// Rand is Delphi's Random(n), 0 ≤ r < n.
	Rand func(n int) int
	// Sound plays a sound file, Music the music; Result reports the
	// outcome (FUN_001b3ca8 → FUN_003bdfa4).
	Sound  func(path string)
	Music  func(path string)
	Result func(win bool)
	// Cursor is the cursor the game asks for (FUN_003ba9d8).
	Cursor cursor.Shape

	state   byte      // +0x18
	count   int       // +0x248 the countdown
	seconds int       // +8 seconds left
	score   int       // +0x30
	tick    time.Time // +0x10
	holes   [moleHoles + 1]moleHole
	hammer  byte      // +0x19
	hamAt   time.Time // +0x20
	effect  Effect    // +0x2c
}

// moleHole is one hole (index 1..7).
type moleHole struct {
	x, y     int  // +0x1b8, +0x1bc: the hole picture's top-left
	kind     byte // +0x1b0: moleKindBomb or moleKindMole
	state    byte // +0x84
	frame    byte // +0x1f8: 1..5 risen
	hitFrame byte // +0x200
	at       time.Time
	up, step time.Duration // +0xac, +0xcc
}

// Variants by the start packet's parameter (FUN_001b3090).
const (
	VariantMouse  = 0
	VariantRabbit = 1
	VariantTurtle = 2
	// MouseParam and MouseParamAlt choose the mice; TurtleParam the turtles;
	// any other parameter the rabbits.
	MouseParam    = 11000
	MouseParamAlt = 0x4a3f
	TurtleParam   = 0x4608
)

const (
	moleHoles    = 7
	moleSeconds  = 0x28
	moleCount    = 4
	moleWinScore = 30
	moleBombCost = 3
	moleFrames   = 5 // rows of the mole, bomb and hit pictures
	moleKindBomb = 0
	moleKindMole = 1
	moleMolesPct = 0x4c // Random(101) below it rises a mole, else a bomb

	moleStateCountdown = 0
	moleStateRunning   = 1
	moleStateOver      = 2
	moleStateDone      = 3

	holeEmpty   = 0
	holeRising  = 1
	holeSinking = 2
	holeUp      = 3
	holeHit     = 4

	moleSecond    = time.Second // _DAT_001b308c
	moleEndWait   = 3000 * time.Millisecond
	moleHitStep   = 200 * time.Millisecond
	moleHammerHit = 50 * time.Millisecond
)

// holeCentres are FUN_001b3fd4's points; a hole's picture starts 0x32 to
// the left.
var holeCentres = [moleHoles + 1][2]int{{}, {300, 150}, {500, 150}, {200, 300}, {400, 300}, {600, 300}, {300, 450}, {500, 450}}

const holeLeft = 0x32

// The pictures (pic\images.BMg, map.JMG for the background, images3 for the
// hit sparkle) and the menu skin's labels and fonts.
const (
	MoleBackground  = "HitMouse"
	MoleHole        = "Hole"
	MoleExplosion   = "Bomb_2"
	MoleSparkle     = "S10416"
	MoleTimeLabel   = "MiniGame_Time_1"
	MoleScoreLabel  = "MiniGame_Score_1"
	MoleDigits      = "Num_White5_1"
	MoleCountDigits = "Num_YellowRed10_2"
	MoleGo          = "Go"
)

// variantPictures are each variant's rising, hit and bomb pictures.
var variantPictures = [...][3]string{
	VariantMouse:  {"Mouse_1", "Mouse_2", "Bomb_1"},
	VariantRabbit: {"Rabit_1", "Rabit_2", "RabitBomb"},
	VariantTurtle: {"HiTurtle_1", "HiTurtle_2", "TurtleBomb"},
}

// Pictures lists every picture the game draws, for loading.
func (m *Mole) Pictures() []string {
	p := variantPictures[m.Variant]
	return []string{MoleBackground, MoleHole, MoleExplosion, MoleSparkle, MoleTimeLabel, MoleScoreLabel,
		MoleDigits, MoleCountDigits, p[0], p[1], p[2], letterFonts[FontRed], letterFonts[FontRed+1]}
}

// The pictures' offsets from a hole's top-left (FUN_001b4484): rising,
// bomb, hit, per variant.
var (
	riseOffset = [...][2]int{{0x10, 0x2a}, {0x1e, 0x2a}, {0xe, 0x31}}
	bombOffset = [...][2]int{{0x1e, 0x33}, {0x1e, 0x33}, {0x1e, 0x35}}
	hitOffset  = [...][2]int{{0x10, 0x2d}, {0x19, 0x1b}, {0xe, 0x2e}}
)

// The HUD of the game (FUN_001b4484).
const (
	timeLabelX, timeDigitsX   = 0x14a, 0x1ae
	scoreLabelX, scoreDigitsX = 0x276, 0x2e4
	labelY, digitsY           = 10, 0xd
	centreX, centreY          = 400, 300
	goAdvance                 = 0x50
)

// VariantOf is FUN_001b3090's choice of animals.
func VariantOf(param int) int {
	switch param {
	case MouseParam, MouseParamAlt:
		return VariantMouse
	case TurtleParam:
		return VariantTurtle
	}
	return VariantRabbit
}

// NewMole is FUN_001b3090.
func NewMole(pics *picdb.DB, param int, now time.Time) *Mole {
	m := &Mole{Pics: pics, Variant: VariantOf(param), count: moleCount, seconds: moleSeconds, tick: now,
		Cursor: cursor.ShapeNormal}
	for i := 1; i <= moleHoles; i++ {
		c := holeCentres[i]
		m.holes[i] = moleHole{x: c[0] - holeLeft, y: c[1], kind: moleKindMole, at: now}
	}
	return m
}

// Start is the start form's Start (+0x288).
func (m *Mole) Start() { m.Started = true }

// Score and Seconds are the shown score and time.
func (m *Mole) Score() int   { return m.score }
func (m *Mole) Seconds() int { return m.seconds }

// Update is FUN_001b4820.
func (m *Mole) Update(now time.Time) {
	if !m.Started {
		return
	}
	m.clock(now)
	m.difficulty()
	m.spawn()
	m.animate(now)
	m.hammerCursor(now)
	m.countdown(now)
	m.finish(now)
}

// clock is FUN_001b3038.
func (m *Mole) clock(now time.Time) {
	if m.state != moleStateRunning || now.Sub(m.tick) < moleSecond {
		return
	}
	m.seconds--
	m.tick = now
	if m.seconds == 0 {
		m.state = moleStateOver
	}
}

// difficulty is FUN_001b4908: by the seconds left, how long a target stays
// up and how fast it rises and sinks (faster as the clock runs down).
func (m *Mole) difficulty() {
	var up, step int
	switch t := m.seconds; {
	case t < 11:
		up, step = 300, 0x3c
	case t < 21:
		up, step = 400, 0x50
	case t < 31:
		up, step = 500, 100
	case t < 41:
		up, step = 600, 0x78
	default:
		return
	}
	for i := 1; i <= moleHoles; i++ {
		m.holes[i].up, m.holes[i].step = time.Duration(up)*time.Millisecond, time.Duration(step)*time.Millisecond
	}
}

// spawn is FUN_001b4b78: with fewer than limit holes busy, up to n empty
// holes (seven random picks) get a mole or a bomb.
func (m *Mole) spawn() {
	if m.state != moleStateRunning {
		return
	}
	var limit, n int
	switch t := m.seconds; {
	case t < 20:
		limit, n = 4, 3
	case t < 40:
		limit, n = 3, 2
	case t < 61:
		limit, n = 2, 1
	default:
		return
	}
	busy := 0
	for i := 1; i <= moleHoles; i++ {
		if m.holes[i].state != holeEmpty {
			busy++
		}
	}
	if busy >= limit {
		return
	}
	for tries, made := moleHoles, 0; tries > 0 && made < n; tries-- {
		h := &m.holes[m.Rand(moleHoles)+1]
		if h.state != holeEmpty {
			continue
		}
		h.kind = moleKindBomb
		if m.Rand(101) < moleMolesPct {
			h.kind = moleKindMole
		}
		h.state = holeRising
		made++
	}
}

// animate is FUN_001b49ac.
func (m *Mole) animate(now time.Time) {
	for i := 1; i <= moleHoles; i++ {
		h := &m.holes[i]
		switch h.state {
		case holeRising:
			if now.Sub(h.at) >= h.step {
				h.frame++
				if h.frame > moleFrames-1 {
					h.frame, h.state = moleFrames, holeUp
				}
				h.at = now
			}
		case holeSinking:
			if now.Sub(h.at) >= h.step {
				h.frame--
				if h.frame == 0 {
					h.state = holeEmpty
				}
				h.at = now
			}
		case holeUp:
			if now.Sub(h.at) >= h.up {
				h.state = holeSinking
			}
		case holeHit:
			if h.kind == moleKindBomb {
				h.hitFrame, h.state, h.at = 0, holeEmpty, now
			} else if now.Sub(h.at) >= moleHitStep {
				h.hitFrame++
				if h.hitFrame > moleFrames-1 {
					h.hitFrame, h.state = 0, holeEmpty
				}
				h.at = now
			}
		}
	}
}

// Hammer states (+0x19) and their cursors (FUN_001b4874).
const (
	hammerReady = 0
	hammerBack  = 1
	hammerDown  = 2
)

func (m *Mole) hammerCursor(now time.Time) {
	switch m.hammer {
	case hammerReady:
		m.Cursor = cursor.ShapeHammer
	case hammerBack:
		m.Cursor = cursor.ShapeHammerSide
		if now.Sub(m.hamAt) >= moleHammerHit {
			m.hammer = hammerReady
		}
	case hammerDown:
		m.Cursor = cursor.ShapeHammerTilted
		if now.Sub(m.hamAt) >= moleHammerHit {
			m.hamAt, m.hammer = now, hammerBack
		}
	}
}

// Countdown sounds.
const (
	soundBeep = `Sound\Wav1605.wav`
	soundGo   = `Sound\Wav1610.wav`
	soundHit  = `Sound\Wav1611.wav`
	soundBomb = `Sound\SEB0008.wav`
	moleMusic = `Sound\BGM0019.wav`
)

// countdown is FUN_001b3b04: a beep a second from 3 down, "Go" at 0, and
// the game and its music a second after.
func (m *Mole) countdown(now time.Time) {
	if m.state != moleStateCountdown || now.Sub(m.tick) < moleSecond {
		return
	}
	if m.count < 0 {
		m.state = moleStateRunning
		if m.Music != nil {
			m.Music(moleMusic)
		}
		return
	}
	m.count--
	m.tick = now
	switch {
	case m.count == 0:
		m.sound(soundGo)
	case m.count > 0:
		m.sound(soundBeep)
	}
}

// finish is FUN_001b3acc: three seconds after the clock runs out the game
// reports its result.
func (m *Mole) finish(now time.Time) {
	if m.state == moleStateOver && now.Sub(m.tick) >= moleEndWait {
		m.End(m.score >= moleWinScore)
		m.state = moleStateDone
	}
}

// End is FUN_001b3ca8: the result goes to the server and the cursor
// returns.
func (m *Mole) End(win bool) {
	m.Cursor = cursor.ShapeNormal
	m.state = moleStateDone
	if m.Result != nil {
		m.Result(win)
		m.Result = nil
	}
}

// Done reports whether the game has reported its result.
func (m *Mole) Done() bool { return m.state == moleStateDone }

func (m *Mole) sound(path string) {
	if m.Sound != nil {
		m.Sound(path)
	}
}

// Click is FUN_001b3ce8: a hit while the hammer is ready swings it and
// hits the first risen bomb or mole under the point.
func (m *Mole) Click(x, y int, now time.Time) {
	if m.state != moleStateRunning || m.hammer != hammerReady {
		return
	}
	m.hamAt, m.hammer = now, hammerDown
	for i := 1; i <= moleHoles; i++ {
		h := &m.holes[i]
		if h.state < holeRising || h.state > holeUp || h.frame == 0 || h.frame > moleFrames || !m.hit(h, x, y) {
			continue
		}
		if h.kind == moleKindBomb {
			h.state, h.frame, h.at = holeEmpty, 0, now
			m.effect.Play(MoleExplosion, 100*time.Millisecond, LevelKeyed, h.x+0x3c, h.y+0x32, 4, 4, 1, now)
			m.score = max(m.score-moleBombCost, 0)
			m.sound(soundBomb)
			return
		}
		h.state = holeHit
		h.hitFrame = moleFrames + 1 - h.frame
		h.frame = 0
		m.effect.Play(MoleSparkle, 70*time.Millisecond, sparkleLevel, x, y+0x3c, 9, 9, 4, now)
		m.score++
		m.sound(soundHit)
		return
	}
}

const sparkleLevel = 0x1e

// hit is FUN_001b2b44: the target's box for its risen frame.
func (m *Mole) hit(h *moleHole, x, y int) bool {
	f := int(h.frame)
	if h.kind == moleKindBomb {
		top := [...]int{0, 0x20, 7, -0xc, -0x12, -0x17}[f]
		left, right := 0x1e, 0x58
		if f == 1 {
			left, right = 0x26, 0x4f
		}
		return h.x+left <= x && x <= h.x+right && h.y+top <= y && y <= h.y+0x30
	}
	lift := 0
	if m.Variant == VariantTurtle {
		lift = 0x14
	}
	top := [...]int{0, 0x16, 0xe, 8, -0xe, -0x1c}[f]
	left, right := 0x21, 0x4b
	if f >= 4 {
		left, right = 0x1a, 0x53
	}
	return h.x+left <= x && x <= h.x+right && h.y+top-lift <= y && y <= h.y+0x30-lift
}

// Draw is FUN_001b4484.
func (m *Mole) Draw(dst *surface.Surface, now time.Time) {
	if m.state == moleStateDone {
		return
	}
	pics := m.Pics
	if i := pics.Find(MoleBackground); i >= 0 {
		pics.Draw(dst, i, 0, 0, false)
	}
	if i := pics.Find(MoleHole); i >= 0 {
		for k := 1; k <= moleHoles; k++ {
			pics.Draw(dst, i, m.holes[k].x, m.holes[k].y, true)
		}
	}
	p := variantPictures[m.Variant]
	for k := 1; k <= moleHoles; k++ {
		h := &m.holes[k]
		switch {
		case h.state >= holeRising && h.state <= holeUp && h.frame != 0:
			if h.kind == moleKindBomb {
				m.drawFrame(dst, p[2], h, bombOffset[m.Variant], int(h.frame))
			} else {
				m.drawFrame(dst, p[0], h, riseOffset[m.Variant], int(h.frame))
			}
		case h.state == holeHit && h.hitFrame != 0:
			m.drawFrame(dst, p[1], h, hitOffset[m.Variant], int(h.hitFrame))
		}
	}
	m.effect.Draw(dst, pics, now)
	if i := pics.Find(MoleTimeLabel); i >= 0 {
		pics.Draw(dst, i, timeLabelX, labelY, true)
	}
	DrawNumber(dst, pics, timeDigitsX, digitsY, m.seconds, true, NumberSmall, MoleDigits)
	if i := pics.Find(MoleScoreLabel); i >= 0 {
		pics.Draw(dst, i, scoreLabelX, labelY, true)
	}
	DrawNumber(dst, pics, scoreDigitsX, digitsY, m.score, true, NumberSmall, MoleDigits)
	if m.state == moleStateCountdown {
		switch {
		case m.count == 0:
			DrawWord(dst, pics, centreX, centreY, MoleGo, true, goAdvance, 0, FontRed)
		case m.count >= 1 && m.count <= 3:
			DrawNumber(dst, pics, centreX, centreY, m.count, true, NumberLarge, MoleCountDigits)
		}
	}
}

// drawFrame draws row frame (1-based) of a five-row picture at the hole.
func (m *Mole) drawFrame(dst *surface.Surface, name string, h *moleHole, off [2]int, frame int) {
	i := m.Pics.Find(name)
	if i < 0 {
		return
	}
	w, ph := m.Pics.Size(i)
	fh := ph / moleFrames
	m.Pics.DrawRect(dst, i, h.x+off[0], h.y-off[1], rectRow(w, fh, frame-1), true)
}
