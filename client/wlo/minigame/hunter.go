package minigame

import (
	"image"
	"time"

	"wonderland-go/client/wlo/picdb"
	"wonderland-go/client/wlo/surface"
)

// Hunter is minigame 4, hunting (TSport_Hunter, constructor FUN_0017e2c8,
// PTR_DAT_004c99e4): monsters (NPC roles) spawn on a forest picture and
// wander; every so often one walks to the player at the bottom of the
// screen and attacks, costing a life. Clicks shoot everything under the
// pointer. The game ends when the player has no lives left, the clock runs
// out (a win) or 50 monsters are down.
//
// The original runs the update (FUN_0017e408) and the draw (FUN_0017e528)
// once each per 30 ms game frame, and part of the game's logic lives in the
// draw: the monsters' steps and attacks (FUN_0017e9e0) and the shot's
// animation (FUN_0017fd14). Step runs all of it per game frame, so Draw
// only draws and the client keeps its display rate.
type Hunter struct {
	Pics *picdb.DB
	// NewMonster creates a monster's role from its NPC template
	// (FUN_00410e04 + FUN_004265a4); nil when the template is missing.
	NewMonster func(id uint16) Monster
	// Body is the player's body type (+8), which picks the hurt sound.
	Body    byte
	Rand    func(n int) int
	Sound   func(path string)
	Music   func(path string)
	Result  func(win bool)
	Started bool // +0x14c

	monsters []*monster // +8.., +0x100 the count spawned
	frame    int        // +0x104
	frameAt  time.Time
	kills    int // +0x108
	byKind   [hunterKinds]int
	seconds  int  // +0x130
	lives    int  // +0x140
	ammo     int  // +300, never spent
	score    int  // +0x134
	ended    bool // +0x13c
	endWait  int  // +0x138
	shaking  bool // +0x159
	shake    byte // +0x158
	shakeX   int  // +0x150
	shakeY   int  // +0x154
	sightX   int  // +0xe8
	sightY   int  // +0xec
	shotX    int  // +0xf0
	shotY    int  // +0xf4
	shot     int  // +0xf8, 0 when no shot shows (+0xfc)
	done     bool
}

// Monster draws a hunted role: role.NPC with its ground shadow.
type Monster interface {
	Draw(dst *surface.Surface, x, y, action int)
	Bounds(x, y, action int) image.Rectangle
	FrameSize(action int) (w, h int)
	Hold(frame int, wrap bool)
}

// monster is one role and the hunt's fields on it.
type monster struct {
	role       Monster
	id         uint16 // +4
	x, y       int    // +0x20, +0x24 (+0x8c, +0x90 the drawn point)
	tx, ty     int    // +0x84, +0x88
	stepAt     time.Time
	action     int  // +0x121
	arrived    bool // +0x123
	hp, maxHP  int  // +0x21c4, +0x21c0
	dead       bool // +0x21b8
	deadFor    int  // +0x21bc, in 50-frame units
	deadAt     time.Time
	countdown  int  // +0x21c8, wanders before the next attack
	attacking  bool // +0x21cc
	attackLeft int  // +0x21d0
	hovered    bool // +0x80, from the last draw
}

// The game frame (DXTimer1Timer's 30 ms multimedia timer).
const hunterFrame = 30 * time.Millisecond

const (
	hunterSeconds    = 0x1e
	hunterLives      = 3
	hunterAmmo       = 200
	hunterEndWait    = 100 // frames before the result
	hunterTick       = 0x32
	hunterWinKills   = 0x32 // more than 49 down ends the game
	hunterMaxSpawn   = 0x32
	hunterSpawnBase  = 0x41
	hunterSpawnMin   = 0x28
	hunterReroll     = 0x96 // frames between a standing monster's walks
	hunterRerollTopY = 0x7d
	hunterCountdown  = 10
	hunterAttacks    = 5
	hunterAttackStep = 10
	hunterDeadFor    = 3
	hunterShotDamage = 0x32
	hunterScorePer   = 5
	hunterWanderW    = 800
	hunterWanderH    = 300
	hunterWanderTop  = 300
	hunterFlyerH     = 0x1db
	hunterFlyerTop   = 0x7d
	hunterPlayerX    = 400
	hunterPlayerY    = 0x230
	hunterGroundTop  = 0x12d // walkers stop at y 300
	hunterGroundY    = 300

	actionStand = 8
	actionDie   = 0x1a
	deathFrame  = 100 * time.Millisecond
)

// The monsters (DAT_004bce04, picked with Random(7)) and their counters
// (+0x110..+0x124).
const (
	MonsterA    = 0x42da
	MonsterB    = 0x4316
	MonsterC    = 0x4322
	MonsterD    = 0x428c // flies above the ground line
	MonsterE    = 0x42a8 // wanders higher
	MonsterBoss = 0x432f
	hunterKinds = 6
)

var hunterMonsters = [...]uint16{MonsterA, MonsterB, MonsterC, MonsterC, MonsterD, MonsterE, MonsterBoss}

// Monster hit points: 250 for the big one, 50 otherwise.
const (
	hunterHP     = 0x32
	hunterBossHP = 0xfa
)

// kind is the counter a kill adds to (FUN_0017eda8), -1 for none.
func kind(id uint16) int {
	switch {
	case id == MonsterA:
		return 0
	case id == MonsterB:
		return 1
	case id == MonsterC:
		return 2
	case id == MonsterD:
		return 3
	case id == MonsterE:
		return 4
	case id == 0x432e || id == MonsterBoss:
		return 5
	}
	return -1
}

// speed is a monster's pace in pixels per millisecond (FUN_0017df40).
func speed(id uint16) float64 {
	switch {
	case id == MonsterB:
		return 0.12
	case id == MonsterD:
		return 0.14
	case id == MonsterC:
		return 0.18
	case id == 0x432e || id == MonsterBoss:
		return 0.2
	case id == MonsterE:
		return 0.22
	}
	return 0.1
}

// flyer reports a monster that may go above the ground line.
func flyer(id uint16) bool { return id >= 0x428b && id <= 0x428d }

// Pictures (FUN_0017f34c). AnimAtt (pic\animAtt) is missing from this
// build, so the original draws nothing there either.
const (
	HunterGround     = "59092" // map.JMG
	HunterTrees      = "59091"
	HunterBushes     = "59093"
	HunterHeart      = "icon_heart"
	HunterSight      = "sight"
	HunterShot       = "S10151"
	HunterScoreLabel = "MiniGame_Score_1"
	HunterTimeLabel  = "MiniGame_Time_1"
	HunterLifeLabel  = "MiniGame_Life_1"
	HunterHPBar      = "Color_R"
)

// Pictures lists the pictures the game draws, for loading.
func (h *Hunter) Pictures() []string {
	return []string{HunterGround, HunterTrees, HunterBushes, HunterHeart, HunterSight, HunterShot,
		HunterScoreLabel, HunterTimeLabel, HunterLifeLabel, HunterHPBar, MoleDigits,
		letterFonts[FontRed], letterFonts[FontRed+1]}
}

// Sounds.
const (
	hunterMusic = `sound\BGM0019.wav`
	soundShot   = `sound\SEB0057.wav`
	soundEmpty  = `sound\SEB0090.wav`
)

// hurtSounds are the player's cry per body type.
var hurtSounds = [...]string{1: `Sound\SEB0173.wav`, 2: `Sound\SEB0221.wav`, 3: `Sound\SEB0156.wav`, 4: `Sound\SEB0199.wav`}

// NewHunter is FUN_0017e2c8.
func NewHunter(pics *picdb.DB, now time.Time) *Hunter {
	return &Hunter{Pics: pics, lives: hunterLives, ammo: hunterAmmo, seconds: hunterSeconds,
		endWait: hunterEndWait, frameAt: now}
}

// Start is the start form's Start: +0x14c and FUN_0017f8b8 (the music and
// the first monster).
func (h *Hunter) Start(now time.Time) {
	h.Started = true
	h.frameAt = now
	if h.Music != nil {
		h.Music(hunterMusic)
	}
	h.spawn(now)
}

// End is the Exit button and Leave (FUN_0017f884): a loss.
func (h *Hunter) End(win bool) {
	h.ended = false
	h.report(win)
}

func (h *Hunter) report(win bool) {
	h.done = true
	if h.Result != nil {
		h.Result(win)
		h.Result = nil
	}
}

// Done reports whether the game has reported its result.
func (h *Hunter) Done() bool { return h.done }

// Score, Seconds and Lives are the shown counters.
func (h *Hunter) Score() int   { return h.score }
func (h *Hunter) Seconds() int { return h.seconds }
func (h *Hunter) Lives() int   { return h.lives }

// Aim is the mouse move (vmt +0x18, FUN_0017fcfc): the sight follows.
func (h *Hunter) Aim(x, y int) { h.sightX, h.sightY = x, y }

// Click is the left button (vmt +0x14, FUN_0017f714): a shot at (x, y)
// hurts every monster under the pointer.
func (h *Hunter) Click(x, y int) {
	if h.ended || !h.Started || h.done {
		return
	}
	if h.ammo < 1 {
		h.sound(soundEmpty)
		h.ended = true
		return
	}
	h.sound(soundShot)
	h.sightX, h.sightY = x, y
	h.shotX, h.shotY = x, y
	h.shot = 1
	for _, m := range h.monsters {
		if m != nil && m.hovered {
			m.hp -= hunterShotDamage
		}
	}
}

func (h *Hunter) sound(path string) {
	if h.Sound != nil {
		h.Sound(path)
	}
}

// Step runs the game frames due by now.
func (h *Hunter) Step(now time.Time) {
	for now.Sub(h.frameAt) >= hunterFrame {
		h.frameAt = h.frameAt.Add(hunterFrame)
		h.gameFrame(h.frameAt)
	}
}

// gameFrame is one frame of FUN_0017e408 followed by the logic of the draw
// (FUN_0017e528).
func (h *Hunter) gameFrame(now time.Time) {
	if h.done {
		return
	}
	for _, m := range h.monsters {
		if m != nil && m.arrived && m.attacking {
			h.shakeStep()
		}
	}
	if h.Started {
		if h.lives < 1 {
			h.ended = true
		}
		if h.kills >= hunterWinKills || h.seconds < 1 {
			h.ended = true
		}
		if h.ended {
			h.endWait--
			if h.endWait < 1 {
				h.report(h.seconds < 1)
				return
			}
		}
		h.frame++
		h.logic(now)
		if h.frame%hunterTick == 0 && !h.ended {
			h.seconds--
		}
	}
	h.monsterFrame(now)
	if h.shot > 0 {
		h.shot++
		if h.shot == 5 {
			h.shot = 0
		}
	}
}

// shakeStep is FUN_0017fc08: the screen's offsets while a monster attacks.
func (h *Hunter) shakeStep() {
	steps := [...][2]int{{0xf, -0xf}, {-0xf, 0xf}, {0xf, 6}, {-0xf, -0xf}, {0, 0}, {0xf, 0xf}, {0, -6}}
	s := steps[h.shake]
	h.shakeX, h.shakeY = s[0], s[1]
	h.shake = (h.shake + 1) % byte(len(steps))
}

// logic is FUN_0017eda8.
func (h *Hunter) logic(now time.Time) {
	if h.ended {
		return
	}
	for _, m := range h.monsters {
		if m == nil || m.dead || m.hp >= 1 {
			continue
		}
		// FUN_00430474(…, 0x1a): the two-sided death, by the facing.
		die := actionDie
		if facingOf(m.action) >= 4 {
			die++
		}
		m.dead, m.deadFor, m.deadAt, m.action = true, hunterDeadFor, now, die
		h.kills++
		if k := kind(m.id); k >= 0 {
			h.byKind[k]++
		}
	}
	for _, m := range h.monsters {
		if m == nil || !(m.arrived || h.frame%hunterReroll == 0 || m.ty < hunterRerollTopY) {
			continue
		}
		if !m.attacking {
			m.countdown--
		}
		if m.countdown == 0 {
			m.attacking = true
			m.tx, m.ty = hunterPlayerX, hunterPlayerY
			m.arrived, m.stepAt = false, now
			m.countdown = hunterCountdown
			h.shaking = true
		}
		if !m.attacking {
			h.wander(m)
			m.arrived, m.stepAt = false, now
		}
	}
	if h.frame%hunterTick == 0 {
		for i, m := range h.monsters {
			if m != nil && m.dead {
				m.deadFor--
				if m.deadFor == 0 {
					h.monsters[i] = nil
				}
			}
		}
	}
	every := max(hunterSpawnBase-len(h.monsters), hunterSpawnMin)
	if h.frame%every == 0 && len(h.monsters) < hunterMaxSpawn {
		h.spawn(now)
	}
}

// wander picks a monster's next walk.
func (h *Hunter) wander(m *monster) {
	m.tx, m.ty = h.Rand(hunterWanderW), h.Rand(hunterWanderH)+hunterWanderTop
	if m.id == MonsterE {
		m.tx, m.ty = h.Rand(hunterWanderW), h.Rand(hunterFlyerH)+hunterFlyerTop
	}
}

// spawn adds a monster (FUN_0017f8b8, FUN_0017eda8): a random kind at a
// random point of the ground, walking to another. A slot is taken even
// when the template is missing, as the count paces the spawning.
func (h *Hunter) spawn(now time.Time) {
	id := hunterMonsters[h.Rand(len(hunterMonsters))]
	m := &monster{id: id, countdown: hunterCountdown, attackLeft: hunterAttacks, hp: hunterHP, stepAt: now,
		action: actionStand}
	if id == 0x432e || id == MonsterBoss {
		m.hp = hunterBossHP
	}
	m.maxHP = m.hp
	m.x, m.y = h.Rand(hunterWanderW), h.Rand(hunterWanderH)+hunterWanderTop
	m.tx, m.ty = h.Rand(hunterWanderW), h.Rand(hunterWanderH)+hunterWanderTop
	if h.NewMonster != nil {
		m.role = h.NewMonster(id)
	}
	if m.role == nil {
		h.monsters = append(h.monsters, nil)
		return
	}
	h.monsters = append(h.monsters, m)
}

// facingOf is FUN_0042646c for walking (0..7) and standing (8..15).
func facingOf(action int) int {
	if action >= actionStand && action < actionStand+8 {
		return action - actionStand
	}
	return action & 7
}

// monsterFrame is FUN_0017e9e0's logic: living monsters face their target
// and step (FUN_00411fd8, FUN_0017df40); attackers hurt the player every
// ten frames, five times, then go back to wandering.
func (h *Hunter) monsterFrame(now time.Time) {
	for _, m := range h.monsters {
		if m == nil {
			continue
		}
		if !m.dead && !h.ended {
			m.face()
			m.step(now)
		}
		if !m.arrived || !m.attacking {
			continue
		}
		if h.frame%hunterAttackStep != 0 {
			continue
		}
		if m.attackLeft == hunterAttacks {
			h.lives--
			if int(h.Body) < len(hurtSounds) && hurtSounds[h.Body] != "" {
				h.sound(hurtSounds[h.Body])
			}
		}
		m.attackLeft--
		if m.attackLeft == 0 {
			m.attacking = false
			m.attackLeft, m.countdown = hunterAttacks, hunterCountdown
			h.shaking = false
		}
	}
	h.score = 0
	for _, n := range h.byKind {
		h.score += n * hunterScorePer
	}
}

// face is FUN_00411fd8: the walking direction toward the target (+0x121),
// diagonal while |dy| / (|dx| + 0.001) is between 0.25 and 2.
func (m *monster) face() {
	dx, dy := m.tx-m.x, m.ty-m.y
	r := float64(abs(dy)) / (0.001 + float64(abs(dx)))
	diag := r > 0.25 && r < 2
	switch {
	case diag && dx < 0 && dy > 0:
		m.action = 3
	case diag && dx < 0 && dy < 0:
		m.action = 1
	case diag && dx > 0 && dy > 0:
		m.action = 5
	case diag && dx > 0 && dy < 0:
		m.action = 7
	case dx > 0 && r <= 0.25:
		m.action = 6
	case dx < 0 && r <= 0.25:
		m.action = 2
	case dy > 0:
		m.action = 4
	case dy < 0:
		m.action = 0
	}
}

// step is FUN_0017df40: both axes move by the monster's pace times the
// time since its last step, until it stands at its target; walkers stop
// at the ground line.
func (m *monster) step(now time.Time) {
	if (m.tx == 0 && m.ty == 0) || (m.x == m.tx && m.y == m.ty) {
		m.action = actionStand + facingOf(m.action)
		m.arrived = true
		return
	}
	n := int(float64(now.Sub(m.stepAt).Milliseconds())*speed(m.id) + 0.5)
	m.stepAt = now
	dx, dy := m.tx-m.x, m.ty-m.y
	if n <= abs(dx) || n <= abs(dy) {
		m.x += sign(dx) * n
		m.y += sign(dy) * n
	} else {
		m.x, m.y = m.tx, m.ty
	}
	if m.y < hunterGroundTop && !flyer(m.id) {
		m.y, m.ty = hunterGroundY, hunterGroundY
		m.arrived = true
	}
}

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

// Draw is FUN_0017e528 without its logic: the ground and its layers, the
// counters (FUN_0017e5c0), the monsters with their health bars
// (FUN_0017e9e0), the sight and the shot (FUN_0017fd14), and the outcome.
// mx, my is the pointer, for the monsters under it. The game keeps
// drawing after its result until the server's 57/2 frees it.
func (h *Hunter) Draw(dst *surface.Surface, mx, my int, now time.Time) {
	pics := h.Pics
	sx, sy := 0, 0
	if h.shaking {
		sx, sy = h.shakeX, h.shakeY
	}
	draw := func(name string, x, y int, transparent bool) {
		if i := pics.Find(name); i >= 0 {
			pics.Draw(dst, i, x, y, transparent)
		}
	}
	draw(HunterGround, sx-0x32, sy-0x96, false)
	draw(HunterTrees, sx-0x32, sy-0x96, true)
	draw(HunterBushes, sx-0x32, sy+0xe6, true)
	if i := pics.Find(HunterHeart); i >= 0 {
		for k := 1; k <= h.lives; k++ {
			pics.DrawStretch(dst, i, image.Rect(k*0x28+0x32, 0x19, k*0x28+0x50, 0x32), true)
		}
	}
	if h.ammo < 1 {
		DrawWord(dst, pics, centreX, 200, "You Lose", true, 0x41, 0, FontRed)
	}
	draw(HunterScoreLabel, 0x276, 0x19, true)
	draw(HunterTimeLabel, 0x159, 0x19, true)
	draw(HunterLifeLabel, 0xf, 0x19, true)
	h.drawDigits(dst, 0x2da, 0x19, h.score)
	h.drawDigits(dst, 0x1bd, 0x19, max(h.seconds, 0))
	h.drawMonsters(dst, sx, sy, mx, my, now)
	draw(HunterSight, h.sightX-0xe, h.sightY-0xe, true)
	if h.shot > 0 {
		if i := pics.Find(HunterShot); i >= 0 {
			w, ph := pics.Size(i)
			pics.DrawLight(dst, i, h.shotX-w/2+sx, h.shotY-ph/10+sy, image.Rect(0, (h.shot-1)*100, 100, h.shot*100), 0x19)
		}
	}
	if h.seconds < 1 {
		DrawWord(dst, pics, centreX, 200, "You Win", true, 0x41, 0, FontRed)
	}
	if h.lives < 1 {
		DrawWord(dst, pics, centreX, 200, "You Lose", true, 0x41, 0, FontRed)
	}
}

// drawDigits draws a number in Num_White5_1, a glyph's width apart.
func (h *Hunter) drawDigits(dst *surface.Surface, x, y, v int) {
	i := h.Pics.Find(MoleDigits)
	if i < 0 {
		return
	}
	w, ph := h.Pics.Size(i)
	gh := ph / numberGlyphs
	for k, d := range itoa(v) {
		row := int(d - '0')
		h.Pics.DrawRect(dst, i, x+k*w, y, rectRow(w, gh, row), true)
	}
}

// drawMonsters draws each monster at its point (shaken), and its health
// bar: Color_R's width less 80 at full health, 5 high, centred 30 above
// the sprite's top.
func (h *Hunter) drawMonsters(dst *surface.Surface, sx, sy, mx, my int, now time.Time) {
	bar := h.Pics.Find(HunterHPBar)
	barW, _ := h.Pics.Size(bar)
	for _, m := range h.monsters {
		if m == nil {
			continue
		}
		if m.dead {
			m.role.Hold(int(now.Sub(m.deadAt)/deathFrame), false)
		}
		_, fh := m.role.FrameSize(m.action)
		if bar >= 0 && m.maxHP > 0 {
			w := m.hp * (barW - 0x50) / m.maxHP
			if w > 0 {
				h.Pics.DrawRect(dst, bar, m.x-w/2+sx, m.y-fh-0x1e+sy, image.Rect(0, 0, w, 5), true)
			}
		}
		x, y := m.x+sx, m.y+sy
		m.role.Draw(dst, x, y, m.action)
		m.hovered = image.Pt(mx, my).In(m.role.Bounds(x, y, m.action))
	}
}
