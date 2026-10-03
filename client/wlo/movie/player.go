package movie

import (
	"image"
	"math"
	"time"
)

// speeds are FUN_00339598's speed classes in pixels per millisecond
// (keyframe +0x0d, actor +0x2dd8); class 0 keeps the previous rate.
var speeds = [...]float64{0, 0.0008, 0.008, 0.08, 0.12, 0.4, 4, 40}

// cameraHalfW and cameraHalfH turn a camera keyframe (the screen's centre)
// into the camera's top-left (FUN_003406c4: x − 400, y − 300).
const (
	cameraHalfW = 400
	cameraHalfH = 300
	// NormalPose is the pose that walks and stands by the movement.
	NormalPose  = 8
	moveEpsilon = 0.001
)

// ActorState is an actor's place in the movie.
type ActorState struct {
	Actor  *Actor
	Key    int     // +0x2dc8
	X, Y   float64 // +0x7c, +0x80
	TX, TY int     // +0x8c, +0x90
	Speed  float64
	Pose   int // +0x6c
	Facing int // +0x2dd0: the direction
	// keyFacing is the current keyframe's facing (Keep: by the movement).
	keyFacing int
	// The animation frame (+0x68), stepped at the speed class's interval
	// (+0x2dd8, FUN_00343068) by FUN_0033984c.
	frame   int
	class   int
	frameAt time.Time
	Moving  bool
	at      time.Time
}

// Active reports whether the actor takes part in stage s.
func (a *ActorState) Active(s int) bool {
	return a.Actor.Start <= s && s <= a.Actor.Start+a.Actor.Count
}

// Point is the actor's position.
func (a *ActorState) Point() image.Point {
	return image.Pt(int(math.Round(a.X)), int(math.Round(a.Y)))
}

// step is FUN_00339598: toward the target at the speed, the longer axis
// moving the full step and the other in proportion; it reports arrival.
func (a *ActorState) step(now time.Time) bool {
	dt := float64(now.Sub(a.at).Milliseconds())
	a.at = now
	dx, dy := float64(a.TX)-a.X, float64(a.TY)-a.Y
	d := math.Round(dt * a.Speed)
	if d >= math.Abs(dx) && d >= math.Abs(dy) {
		a.X, a.Y = float64(a.TX), float64(a.TY)
		a.Moving = false
		return true
	}
	ax, ay := math.Abs(dx), math.Abs(dy)
	sx, sy := d, d
	if ax > ay {
		sy = d * ay / (ax + moveEpsilon)
	} else {
		sx = d * ax / (ay + moveEpsilon)
	}
	a.X += math.Copysign(math.Min(sx, ax), dx)
	a.Y += math.Copysign(math.Min(sy, ay), dy)
	a.Moving = true
	return false
}

// Player runs a movie (TMovie's update FUN_003406c4).
type Player struct {
	M   *Movie
	Now func() time.Time
	// Say shows a line (FUN_003430c0); Talking reports whether the talk
	// window still shows it.
	Say     func(Line)
	Talking func() bool
	// Music plays a sound table entry as the music, MapMusic the map's
	// track; Sound plays a keyframe's sound table entry once.
	Music    func(index int)
	MapMusic func()
	Sound    func(index int, loop bool)
	// Facing turns a movement into a direction (FUN_0041218c).
	Facing func(x, y, tx, ty, current int) int

	Stage    int // +0xf90
	Actors   []*ActorState
	Pictures []*PictureState
	Camera   image.Point // the camera's top-left
	cam      ActorState
	camKey   int // +0x2140
	stageAt  time.Time
	line     *Line // the line being said (+0xf98)
	lineAt   time.Time
	said     map[int]bool
	ended    bool
	effects
}

// Start begins the movie at stage 0 (FUN_0033e18c's set-up).
func (p *Player) Start() {
	now := p.Now()
	p.said = map[int]bool{}
	add := func(a *Actor) {
		if a == nil || len(a.Keys) == 0 {
			return
		}
		k := a.Keys[0]
		s := &ActorState{Actor: a, X: float64(k.X), Y: float64(k.Y), TX: k.X, TY: k.Y, Speed: speeds[defaultClass], Pose: NormalPose, Facing: a.Facing, keyFacing: Keep, at: now,
			class: defaultClass, frameAt: now}
		if k.Pose != Keep {
			s.Pose = k.Pose
		}
		p.Actors = append(p.Actors, s)
	}
	add(p.M.Player)
	for i := range p.M.NPCs {
		add(&p.M.NPCs[i])
	}
	for i := range p.M.Images {
		p.Pictures = append(p.Pictures, newPicture(&p.M.Images[i], now))
	}
	c := p.M.Camera
	if len(c.Keys) > 0 {
		p.cam = ActorState{X: float64(c.Keys[0].X - cameraHalfW), Y: float64(c.Keys[0].Y - cameraHalfH), Speed: speeds[4], at: now}
		p.cam.TX, p.cam.TY = int(p.cam.X), int(p.cam.Y)
		if len(c.Keys) > 1 && c.Start == 0 {
			p.cam.TX, p.cam.TY = c.Keys[1].X-cameraHalfW, c.Keys[1].Y-cameraHalfH
		}
	}
	p.Camera = p.cam.Point()
	p.stageAt = now
	p.Light = lightFull
	p.enterStage(now)
}

// Ended reports whether the movie has finished.
func (p *Player) Ended() bool { return p.ended }

// PlayerActor is the actor standing for the player, nil when absent.
func (p *Player) PlayerActor() *ActorState {
	if p.M.Player != nil && len(p.Actors) > 0 && p.Actors[0].Actor == p.M.Player {
		return p.Actors[0]
	}
	return nil
}

func (p *Player) cameraActive() bool {
	c := p.M.Camera
	return len(c.Keys) > 0 && c.Start <= p.Stage && p.Stage <= c.Start+c.Count
}

// Tick is one frame of FUN_003406c4.
func (p *Player) Tick() {
	if p.ended {
		return
	}
	now := p.Now()
	// The fade and the shakes step once per game frame; a fade holds the
	// movie until it completes (FUN_003406c4).
	if p.gameFrames(now) {
		return
	}
	for _, a := range p.Actors {
		a.stepFrame(now)
	}
	for _, s := range p.Pictures {
		if s.Drawn(p.Stage) {
			s.stepFrame(now)
		}
	}
	if p.line != nil {
		// A line waits its delay, then for the talk window to close.
		if now.Sub(p.lineAt) < time.Duration(p.line.Delay)*time.Millisecond || p.Talking() {
			p.moveAll(now)
			return
		}
		p.said[p.lineIndex()] = true
		p.line = nil
	}
	active, done := 1, 0
	if p.waitDone(now) {
		done++
	}
	if p.cameraActive() {
		active++
		if p.cam.step(now) {
			done++
		}
		p.Camera = p.cam.Point()
	}
	for _, s := range p.Pictures {
		if !s.active(p.Stage) {
			continue
		}
		active++
		if s.mover.step(now) {
			done++
		}
	}
	for _, a := range p.Actors {
		if !a.Active(p.Stage) {
			continue
		}
		active++
		x, y := a.Point().X, a.Point().Y
		if a.step(now) {
			done++
		} else {
			// FUN_00342948 while moving: the keyframe's facing, or the
			// movement's direction.
			if a.keyFacing != Keep {
				a.Facing = a.keyFacing
			} else if p.Facing != nil {
				a.Facing = p.Facing(x, y, a.TX, a.TY, a.Facing)
			}
		}
	}
	if active != done {
		return
	}
	for i := range p.M.Lines {
		l := &p.M.Lines[i]
		if l.Stage == p.Stage && !p.said[i] {
			p.line, p.lineAt = l, now
			if p.Say != nil {
				p.Say(*l)
			}
			return
		}
	}
	p.advance(now)
}

func (p *Player) lineIndex() int {
	for i := range p.M.Lines {
		if &p.M.Lines[i] == p.line {
			return i
		}
	}
	return -1
}

func (p *Player) moveAll(now time.Time) {
	for _, a := range p.Actors {
		if a.Active(p.Stage) {
			a.step(now)
		}
	}
	for _, s := range p.Pictures {
		if s.active(p.Stage) {
			s.mover.step(now)
		}
	}
	if p.cameraActive() {
		p.cam.step(now)
		p.Camera = p.cam.Point()
	}
}

// waitDone is FUN_0034397c: the stage's wait has passed.
func (p *Player) waitDone(now time.Time) bool {
	if p.Stage >= len(p.M.Stages) {
		return true
	}
	return now.Sub(p.stageAt) >= time.Duration(p.M.Stages[p.Stage].Wait)*time.Millisecond
}

// advance moves every active part to its next keyframe and the movie to
// the next stage.
func (p *Player) advance(now time.Time) {
	if p.cameraActive() {
		// The camera's own counter: keyframe 1 during its first stage,
		// then one more each stage.
		c := p.M.Camera
		p.camKey++
		if k := p.camKey; k < len(c.Keys) {
			p.cam.TX, p.cam.TY = c.Keys[k].X-cameraHalfW, c.Keys[k].Y-cameraHalfH
			p.cam.Speed = speedOf(c.Keys[k].Speed, p.cam.Speed)
		}
	}
	for _, s := range p.Pictures {
		if s.active(p.Stage) {
			s.next()
		}
	}
	for _, a := range p.Actors {
		if !a.Active(p.Stage) {
			continue
		}
		a.Key++
		if a.Key >= a.Actor.Count {
			a.TX, a.TY = int(a.X), int(a.Y)
			continue
		}
		k := a.Actor.Keys[a.Key]
		a.TX, a.TY = k.X, k.Y
		a.Speed = speedOf(k.Speed, a.Speed)
		if k.Speed > 0 && k.Speed < len(speeds) {
			a.class = k.Speed
		}
		if k.Pose != Keep {
			a.Pose = k.Pose
		}
		a.keyFacing = k.Facing
		if k.Sound > 0 && p.Sound != nil {
			p.Sound(k.Sound, k.SoundLoop)
		}
	}
	p.Stage++
	p.stageAt = now
	if p.Stage > p.M.Last {
		p.ended = true
		if p.MusicChanged && p.MapMusic != nil {
			p.MapMusic()
		}
		return
	}
	p.enterStage(now)
}

// enterStage applies the stage's timeline (effects.go).
func (p *Player) enterStage(now time.Time) {
	p.applyEffects(now)
}

// Shown reports whether the actor is drawn in the current stage
// (FUN_00340404: from stage Start + 1 to Start + Count).
func (p *Player) Shown(a *ActorState) bool {
	return a.Actor.Start+1 <= p.Stage && p.Stage <= a.Actor.Start+a.Actor.Count
}

func speedOf(class int, current float64) float64 {
	if class > 0 && class < len(speeds) {
		return speeds[class]
	}
	return current
}

// twoWay are the pose groups with two directions, left (0) and right (1)
// (FUN_00342af0).
var twoWay = map[int]bool{0x10: true, 0x1a: true, 0x1c: true, 0x1e: true, 0x20: true, 0x22: true,
	0x24: true, 0x26: true, 0x28: true, 0x2a: true, 0x2c: true, 0x3e: true, 0x40: true}

const facingsRight = 3 // directions above it face right in two-way groups

// Action is FUN_00339930's role action: the pose's group plus the
// direction, which two-way groups reduce to 0 or 1 (FUN_00342af0).
func (a *ActorState) Action() int {
	d := a.Facing
	if twoWay[a.Pose] {
		if d > facingsRight {
			d = 1
		} else {
			d = 0
		}
	}
	return a.Pose + d
}

// frameIntervals are FUN_00343068's frame times per speed class.
var frameIntervals = [...]time.Duration{0, 500 * time.Millisecond, 400 * time.Millisecond, 300 * time.Millisecond,
	230 * time.Millisecond, 100 * time.Millisecond, 50 * time.Millisecond, time.Millisecond}

const defaultClass = 4

// stepFrame advances an animating actor's frame (FUN_00339930).
func (a *ActorState) stepFrame(now time.Time) {
	if _, fixed := a.Frame(); fixed {
		return
	}
	if now.Sub(a.frameAt) > frameIntervals[a.class] {
		a.frameAt = now
		a.frame++
	}
}

// Frame is the frame to draw: the keyframe's fixed frame (clamped to the
// action's last by the painter), or the animation counter, which wraps.
func (a *ActorState) Frame() (frame int, fixed bool) {
	if a.Key < len(a.Actor.Keys) {
		if f := a.Actor.Keys[a.Key].Frame; f > 0 {
			return f - 1, true
		}
	}
	return a.frame, false
}
