package battle

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"math"
	"time"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/protocol"
)

const (
	nativeMotionMaximumFrames      = 50
	nativeMotionFixedPrefixBytes   = 20
	nativeMotionMaximumHold        = 30000
	nativeBattleSceneOffset        = 0x1988
	NativeDefaultBattleScene       = 59081 // FUN_0036f448's e6c9 fallback.
	nativeBattleSceneRecordBytes   = 131
	nativeBattleSceneLayersOffset  = 82 // TFSceneData +0x58, after four-byte prefix and two-byte index.
	nativeBattleSceneHeightsOffset = 88
	nativeMotionLastFrameTrigger   = 100
	nativeMotionWalkRight          = 2
	nativeMotionWalkLeft           = 6
	nativeMotionStandRight         = 10
	nativeMotionStandLeft          = 14
	nativeMotionPairedActions      = 16
	// Approach and return poses (FUN_003747f4, FUN_003753ac): mode 0 runs
	// with the sprite animating, mode 1 holds a leap pose. Move kind 1
	// (Npc.dat +0x57) walks instead of running.
	motionRunRight          = 0x41
	motionRunLeft           = 0x40
	motionWalkRight         = 6
	motionWalkLeft          = 2
	motionLeapRightSide     = 0x24 // fighters in columns 3-4 (+0x21f4 == 1)
	motionLeapLeftSide      = 0x25
	motionLeapWalkRightSide = 0x10 // move kind 1's leap poses
	motionLeapWalkLeftSide  = 0x11
	MoveKindWalk            = 1
)

// TargetReferenceDistance is a compatibility constant: the horizontal
// distance from column 4 to column 1 in row 3, which the authored approach
// offsets (about 509 for the basic attack) are measured against. The
// native re-targeting is not traced; the battle captures show attackers
// stopping short of targets at any distance, which scaling by the actual
// distance over this reference reproduces.
var TargetReferenceDistance = Cell{4, 3}.Position().X - Cell{1, 3}.Position().X

type MotionFrame struct {
	Action, Frame byte
	Speed         float64
	Hold          time.Duration
	Offset        image.Point
}
type Motion struct {
	Approach, Return byte
	// Reaction is the motion its targets play when hit (record +0x2c,
	// FUN_003790d4 keeps it at the action's +0xc/+0xe): 60011 for the basic
	// attack, a short knock-back in the hurt pose.
	Reaction uint16
	// Effects are the visual effects the action shows (FUN_00386f60).
	Effects []MotionEffect
	// LeapScale and ReturnLeapScale multiply the leap arc (approach and
	// return mode 1, FUN_00373e58).
	LeapScale, ReturnLeapScale float64
	Frames                     []MotionFrame
	DamageMode                 byte
	Events                     []MotionEvent
	Sounds                     []SoundCue
	spans                      []time.Duration
}

// MotionEffect is one visual effect of a motion record (FUN_00377060's
// effect section, set up by FUN_00386f60 and FUN_003725f8): the picture
// S<ID> (with a direction suffix), a path of attacker-relative points, and
// its animation and timing.
type MotionEffect struct {
	ID     uint32
	Points []image.Point // +0x57
	Speed  float64       // +0x5f, pixels per millisecond along the points
	// Interval (+0x63) is the frame time; Frames (+0x67) the strip's frame
	// count; First and Last (+0x6b, +0x6f) the frames played, in either
	// direction.
	Interval            time.Duration
	Frames, First, Last byte
	Unresolved73        byte   // +0x73, the effect's +0x5c
	Trigger             uint32 // +0x77: 1..100 a keyframe, otherwise milliseconds
	Animate             byte   // +0x7b: 1 steps the frames (FUN_00372be4)
	Move                byte   // +0x7f: 0 stays on a point, 1 and 2 travel the points
	Repeat              byte   // +0x83: 0 loops, n plays n times
	Unresolved87        byte   // +0x87, the effect's +0x748
	Direction           byte   // +0x8b: picture suffix (none, L, R, F, B), L/R swapped on the right
	Unresolved8f        uint32 // +0x8f, the effect's +0x740
}

type MotionEvent struct {
	Trigger uint32 // 1..100: one-based motion keyframe; otherwise elapsed milliseconds.
	Percent byte
}

// SoundCue is the native seven-byte CH_TBattleSound entry. The trailing
// flag is retained as an unresolved compatibility field; the native setup
// FUN_003800a4 reads only the sound ID and trigger.
type SoundCue struct {
	ID                uint16
	Trigger           uint32
	CompatibilityFlag byte
}
type Background struct {
	Pictures [3]uint16
	Heights  [3]uint16
}
type Assets struct {
	Motions     map[uint16]Motion
	Backgrounds map[uint16]Background
}

// ParseMotion reads the variable keyframes in MBTM, FUN_00377060. Preserve the
// native action/frame numbers; speeds are pixels/ms and holds are milliseconds.
func ParseMotion(b []byte) (Motion, error) {
	var m Motion
	r := protocol.NewReader(b)
	r.Bytes(3)
	m.Approach = r.U8()
	m.Return = r.U8()
	// +0x1e, then the leap scales +0x23/+0x27 (FUN_003a1a9c keeps them at
	// +0xc48/+0xc4c for FUN_00373e58), +0x2b, the reaction motion (+0x2c)
	// and +0x2e.
	r.Bytes(4)
	m.LeapScale = float64(math.Float32frombits(r.U32()))
	m.ReturnLeapScale = float64(math.Float32frombits(r.U32()))
	r.Bytes(1)
	m.Reaction = r.U16()
	r.Bytes(4) // +0x2e
	m.DamageMode = r.U8()
	count := int(r.U8())
	if count > nativeMotionMaximumFrames {
		return m, fmt.Errorf("too many motion events")
	}
	for i := 0; i < count; i++ {
		m.Events = append(m.Events, MotionEvent{r.U32(), r.U8()})
	}
	count = int(r.U8())
	if count > nativeMotionMaximumFrames {
		return m, fmt.Errorf("invalid frame count")
	}
	for i := 0; i < count; i++ {
		f := MotionFrame{Action: r.U8(), Frame: r.U8(), Speed: r.F64()}
		hold := r.U32()
		f.Hold = time.Duration(hold) * time.Millisecond
		if math.IsNaN(f.Speed) || math.IsInf(f.Speed, 0) || f.Speed < 0 || hold > nativeMotionMaximumHold {
			return m, fmt.Errorf("invalid motion timing")
		}
		m.Frames = append(m.Frames, f)
	}
	// The native allocation hint (+0x1ea) may exceed the frame count.
	// FUN_00377060 still reads exactly one coordinate pair per frame.
	coordinates := int(r.U8())
	if coordinates > nativeMotionMaximumFrames {
		return m, fmt.Errorf("invalid coordinate allocation")
	}
	for i := range m.Frames {
		m.Frames[i].Offset = image.Pt(int(int32(r.U32())), int(int32(r.U32())))
	}
	// Walk the variable visual-effect section to reach the sound track.
	// Its coordinates and 29-byte parameter block are independent of fighter
	// frames. FUN_00377060 then reads effectCount+1 anchor bytes.
	effects := int(r.U8())
	if effects > nativeMotionMaximumFrames {
		return m, fmt.Errorf("too many visual effects")
	}
	for i := 0; i < effects; i++ {
		e := MotionEffect{ID: r.U32()}
		points := int(r.U8())
		for j := 0; j < points; j++ {
			e.Points = append(e.Points, image.Pt(int(int32(r.U32())), int(int32(r.U32()))))
		}
		e.Speed = r.F64()
		e.Interval = time.Duration(r.U32()) * time.Millisecond
		e.Frames, e.First, e.Last, e.Unresolved73 = r.U8(), r.U8(), r.U8(), r.U8()
		e.Trigger = r.U32()
		e.Animate, e.Move, e.Repeat, e.Unresolved87, e.Direction = r.U8(), r.U8(), r.U8(), r.U8(), r.U8()
		e.Unresolved8f = r.U32()
		if math.IsNaN(e.Speed) || math.IsInf(e.Speed, 0) {
			return m, fmt.Errorf("invalid effect speed")
		}
		m.Effects = append(m.Effects, e)
	}
	r.Bytes(effects + 1)
	sounds := int(r.U8())
	if sounds > nativeMotionMaximumFrames {
		return m, fmt.Errorf("too many sound cues")
	}
	for i := 0; i < sounds; i++ {
		m.Sounds = append(m.Sounds, SoundCue{r.U16(), r.U32(), r.U8()})
	}
	if r.Err() != nil {
		return m, r.Err()
	}
	m.spans = m.frameSpans()
	return m, nil
}
func Load(a login.Assets) (*Assets, error) {
	out := &Assets{Motions: map[uint16]Motion{}, Backgrounds: map[uint16]Background{}}
	raw, err := clientfs.ReadFile(a.DataPath("animation_data.json"))
	if err != nil {
		return nil, err
	}
	var motions struct {
		Records []struct {
			ID  uint16 `json:"id"`
			Hex string `json:"decoded_hex"`
		}
	}
	if err = json.Unmarshal(raw, &motions); err != nil {
		return nil, err
	}
	for _, record := range motions.Records {
		b, err := hex.DecodeString(record.Hex)
		if err != nil {
			return nil, err
		}
		m, err := ParseMotion(b)
		if err != nil {
			return nil, fmt.Errorf("animation %d: %w", record.ID, err)
		}
		out.Motions[record.ID] = m
	}
	raw, err = clientfs.ReadFile(a.DataPath("scene_data.json"))
	if err != nil {
		return nil, err
	}
	var scenes struct {
		Records []struct {
			Hex string `json:"decoded_hex"`
		}
	}
	if err = json.Unmarshal(raw, &scenes); err != nil {
		return nil, err
	}
	for _, record := range scenes.Records {
		b, err := hex.DecodeString(record.Hex)
		if err != nil || len(b) != nativeBattleSceneRecordBytes {
			return nil, fmt.Errorf("invalid scene record")
		}
		r := protocol.NewReader(b)
		id := r.U16()
		r.Bytes(nativeBattleSceneLayersOffset - 2)
		var bg Background
		for i := range bg.Pictures {
			bg.Pictures[i] = r.U16()
		}
		for i := range bg.Heights {
			bg.Heights[i] = r.U16()
		}
		if bg.Pictures[2] != 0 {
			out.Backgrounds[id] = bg
		}
	}
	return out, nil
}

// Scene is the scene record a battle background ID selects (FUN_0036f448):
// the formation's background less 0x1988, or the 59081 fallback. Its music
// is the battle music (battle scenes name BGM0014).
func (a *Assets) Scene(id uint16) uint16 {
	if a != nil {
		if _, ok := a.Backgrounds[id-nativeBattleSceneOffset]; ok {
			return id - nativeBattleSceneOffset
		}
	}
	return NativeDefaultBattleScene
}

func (a *Assets) Background(id uint16) Background {
	if a == nil {
		return Background{}
	}
	if bg, ok := a.Backgrounds[id-nativeBattleSceneOffset]; ok {
		return bg
	}
	return a.Backgrounds[NativeDefaultBattleScene]
}

// Timeline uses the authored Chebyshev-distance travel, pixels/ms speeds and
// holds also used by the server's MBTM duration estimator. A compatibility
// scheduling margin keeps a zero-hold frame visible at least one game tick.
const motionTick = 30 * time.Millisecond
const compatibilityMotionSpeed = .6

func (m Motion) frameSpans() []time.Duration {
	if m.spans != nil {
		return m.spans
	}
	spans := make([]time.Duration, len(m.Frames))
	previous := image.Point{}
	for i, f := range m.Frames {
		speed := f.Speed
		if i == 0 {
			speed = compatibilityMotionSpeed
			if m.Approach == 1 {
				speed = .8
			}
		}
		if speed <= 0 {
			speed = compatibilityMotionSpeed
		}
		distance := max(math.Abs(float64(f.Offset.X-previous.X)), math.Abs(float64(f.Offset.Y-previous.Y)))
		spans[i] = time.Duration(distance/speed*float64(time.Millisecond)) + f.Hold + motionTick
		previous = f.Offset
	}
	return spans
}
func (m Motion) Duration() time.Duration {
	var d time.Duration
	for _, span := range m.frameSpans() {
		d += span
	}
	if len(m.Frames) > 0 {
		d += m.returnTravel() + motionTick
	}
	// Millisecond cues can outlive the last keyframe. Keep their track alive
	// rather than discarding a late sound or forcing a damage cue early.
	for _, event := range m.Events {
		d = max(d, m.EventAt(event)+motionTick)
	}
	for _, sound := range m.Sounds {
		d = max(d, m.EventAt(MotionEvent{Trigger: sound.Trigger})+motionTick)
	}
	// Effects that end by themselves keep the action alive until they do.
	for _, e := range m.Effects {
		if end, ok := e.end(); ok {
			start := time.Duration(e.Trigger) * time.Millisecond
			if e.Trigger >= 1 && e.Trigger <= nativeMotionLastFrameTrigger {
				start = m.EventAt(MotionEvent{Trigger: e.Trigger})
			}
			d = max(d, start+end+motionTick)
		}
	}
	return d
}

// PositionAt applies FUN_003a1a9c's actor-relative coordinates: fighters on
// the right subtract X, while fighters on the left add X; both add Y.
// FUN_003741f0 advances both axes proportionally to the longer axis. The
// timeline retains frameSpans' documented scheduling compatibility margin.
// When target is a fighter of the other half, the path is scaled to it (see
// TargetReferenceDistance).
func (m Motion) PositionAt(cell, target Cell, elapsed time.Duration) image.Point {
	previous := image.Point{}
	spans := m.frameSpans()
	for i, frame := range m.Frames {
		if elapsed < spans[i] {
			travel := max(time.Duration(0), spans[i]-frame.Hold-motionTick)
			return motionPoint(cell, target, interpolateMotion(previous, frame.Offset, elapsed, travel))
		}
		elapsed -= spans[i]
		previous = frame.Offset
	}
	return motionPoint(cell, target, interpolateMotion(previous, image.Point{}, elapsed, m.returnTravel()))
}

func (m Motion) returnTravel() time.Duration {
	if len(m.Frames) == 0 {
		return 0
	}
	p := m.Frames[len(m.Frames)-1].Offset
	speed := compatibilityMotionSpeed
	if m.Return == 1 {
		speed = .8
	}
	return time.Duration(max(math.Abs(float64(p.X)), math.Abs(float64(p.Y))) / speed * float64(time.Millisecond))
}

// motionPoint places an authored offset: toward the target when it stands
// on the other half, otherwise actor-relative.
func motionPoint(cell, target Cell, offset image.Point) image.Point {
	if !target.Valid() || (cell.X > 2) == (target.X > 2) || TargetReferenceDistance == 0 {
		return motionPosition(cell, offset)
	}
	from, to := cell.Position(), target.Position()
	f := float64(offset.X) / float64(TargetReferenceDistance)
	return image.Pt(from.X+int(math.Round(float64(to.X-from.X)*f)), from.Y+int(math.Round(float64(to.Y-from.Y)*f))+offset.Y)
}

// PoseAt is the drawn action and frame (-1 animates): the run or leap pose
// while approaching the first keyframe and returning (FUN_003747f4,
// FUN_003753ac), and the keyframes' fixed poses in between.
func (m Motion) PoseAt(cell, target Cell, elapsed time.Duration, moveKind byte) (action, frame int) {
	if len(m.Frames) == 0 {
		return 0, -1
	}
	spans := m.frameSpans()
	first := m.Frames[0]
	approach := spans[0] - first.Hold - motionTick
	if first.Offset != (image.Point{}) && approach > 0 && elapsed < approach {
		from := cell.Position()
		to := motionPoint(cell, target, first.Offset)
		return m.travelPose(cell, m.Approach, to.X > from.X, moveKind)
	}
	var total time.Duration
	for _, span := range spans {
		total += span
	}
	if elapsed >= total {
		last := m.Frames[len(m.Frames)-1].Offset
		if last == (image.Point{}) {
			return m.ActionAt(cell, elapsed)
		}
		from := motionPoint(cell, target, last)
		return m.travelPose(cell, m.Return, cell.Position().X > from.X, moveKind)
	}
	return m.ActionAt(cell, elapsed)
}

// LiftAt is how far a leaping fighter rises above the ground (+0x2078,
// FUN_00373e58): with v the fraction of the leap travelled, the angle
// a = (0.5-|v-0.5|)*360 gives a-0.003*a*a, times the leap's scale.
func (m Motion) LiftAt(cell, target Cell, elapsed time.Duration) int {
	if len(m.Frames) == 0 {
		return 0
	}
	spans := m.frameSpans()
	first := m.Frames[0]
	if approach := spans[0] - first.Hold - motionTick; m.Approach == 1 && first.Offset != (image.Point{}) && approach > 0 && elapsed < approach {
		return leapLift(float64(elapsed)/float64(approach), m.LeapScale)
	}
	var total time.Duration
	for _, span := range spans {
		total += span
	}
	if back := m.returnTravel(); m.Return == 1 && elapsed >= total && elapsed-total < back {
		return leapLift(float64(elapsed-total)/float64(back), m.ReturnLeapScale)
	}
	return 0
}

const (
	leapHalf    = .5
	leapDegrees = 360
	leapCurve   = .003
)

func leapLift(v, scale float64) int {
	a := math.RoundToEven((leapHalf - math.Abs(v-leapHalf)) * leapDegrees)
	h := math.RoundToEven(a - leapCurve*a*a)
	return int(math.RoundToEven(h * scale))
}

func (m Motion) travelPose(cell Cell, mode byte, right bool, moveKind byte) (action, frame int) {
	if mode == 1 {
		switch {
		case cell.X > 2 && moveKind == MoveKindWalk:
			return motionLeapWalkRightSide, 0
		case cell.X > 2:
			return motionLeapRightSide, 0
		case moveKind == MoveKindWalk:
			return motionLeapWalkLeftSide, 0
		}
		return motionLeapLeftSide, 0
	}
	switch {
	case right && moveKind == MoveKindWalk:
		return motionWalkRight, -1
	case right:
		return motionRunRight, -1
	case moveKind == MoveKindWalk:
		return motionWalkLeft, -1
	}
	return motionRunLeft, -1
}

func interpolateMotion(from, to image.Point, elapsed, travel time.Duration) image.Point {
	if elapsed <= 0 {
		return from
	}
	if travel <= 0 || elapsed >= travel {
		return to
	}
	ratio := float64(elapsed) / float64(travel)
	return image.Pt(from.X+int(math.RoundToEven(float64(to.X-from.X)*ratio)), from.Y+int(math.RoundToEven(float64(to.Y-from.Y)*ratio)))
}

func motionPosition(cell Cell, offset image.Point) image.Point {
	if cell.X > 2 {
		offset.X = -offset.X
	}
	return cell.Position().Add(offset)
}

// ActionAt mirrors native directional actions exactly as FUN_003a1a9c does;
// neutral actions below 16 retain their authored value.
func (m Motion) ActionAt(cell Cell, elapsed time.Duration) (action, frame int) {
	if len(m.Frames) == 0 {
		return 0, -1
	}
	f := m.Frames[m.FrameAt(elapsed)]
	action, frame = int(f.Action), int(f.Frame)
	if cell.X > 2 {
		switch action {
		case nativeMotionWalkRight, nativeMotionStandRight:
			action += 4
		case nativeMotionWalkLeft, nativeMotionStandLeft:
			action -= 4
		default:
			if action >= nativeMotionPairedActions {
				action ^= 1
			}
		}
	}
	return action, frame
}
func (m Motion) FrameAt(elapsed time.Duration) int {
	for i, span := range m.frameSpans() {
		if elapsed < span {
			return i
		}
		elapsed -= span
	}
	return max(0, len(m.Frames)-1)
}

// FUN_00379c18 distinguishes frame triggers from millisecond triggers.
func (m Motion) EventAt(e MotionEvent) time.Duration {
	if e.Trigger == 0 || e.Trigger > nativeMotionLastFrameTrigger {
		return time.Duration(e.Trigger) * time.Millisecond
	}
	var at time.Duration
	for i, span := range m.frameSpans() {
		if uint32(i+1) >= e.Trigger {
			return at
		}
		at += span
	}
	return at
}

// DamageParts follows FUN_0039fdd8: round each authored percentage, then
// reconcile the final cues so displayed parts sum to the server's amount.
func (m Motion) DamageParts(amount uint32) []uint32 {
	if m.DamageMode == 0 || len(m.Events) == 0 {
		return []uint32{amount}
	}
	parts := make([]uint64, len(m.Events))
	var total uint64
	for i, e := range m.Events {
		parts[i] = uint64(math.RoundToEven(float64(amount) * float64(e.Percent) / 100))
		total += parts[i]
	}
	if total < uint64(amount) {
		parts[len(parts)-1] += uint64(amount) - total
	} else {
		excess := total - uint64(amount)
		for i := len(parts) - 1; i >= 0 && excess > 0; i-- {
			take := min(parts[i], excess)
			parts[i] -= take
			excess -= take
		}
	}
	out := make([]uint32, len(parts))
	for i, v := range parts {
		out[i] = uint32(v)
	}
	return out
}

// Effect animation modes (+0x7b, FUN_00372be4) and movement (+0x7f,
// FUN_003728cc).
const (
	EffectLoop     = 1 // wraps from Last to First; Repeat > 0 ends after that many plays
	EffectHold     = 2 // stops on Last
	EffectOnce     = 3 // ends after Last
	EffectStay     = 0 // drawn on the first point
	EffectTravel   = 1 // travels the points, then stays on the last
	EffectTravelTo = 2 // travels the points and ends on the last
)

// effectSuffixes are FUN_003768ec's picture suffixes by direction.
var effectSuffixes = [...]string{"", "L", "R", "F", "B"}

// EffectFrame is one effect's picture frame (1-based, of Frames rows)
// centred on At.
type EffectFrame struct {
	Picture       string
	Frame, Frames int
	At            image.Point
}

// EffectsAt lists the motion's effects shown at elapsed: each starts at its
// trigger (a keyframe's start, or milliseconds), steps a frame per interval
// and is placed on its attacker-relative points like the motion's own
// (motionPoint), travelling them at its speed when it moves. Fighters on
// the right swap the L and R pictures (FUN_00386f60).
func (m Motion) EffectsAt(cell, target Cell, elapsed time.Duration) []EffectFrame {
	var out []EffectFrame
	for _, e := range m.Effects {
		if len(e.Points) == 0 || e.Frames == 0 || e.First == 0 {
			continue
		}
		start := time.Duration(e.Trigger) * time.Millisecond
		if e.Trigger >= 1 && e.Trigger <= nativeMotionLastFrameTrigger {
			start = m.EventAt(MotionEvent{Trigger: e.Trigger})
		}
		t := elapsed - start
		if t < 0 {
			continue
		}
		frame, shown := e.frameAt(t)
		if !shown {
			continue
		}
		at, done := e.pointAt(t)
		if done {
			continue
		}
		dir := e.Direction
		if cell.X > 2 && (dir == 1 || dir == 2) {
			dir = 3 - dir
		}
		suffix := ""
		if int(dir) < len(effectSuffixes) {
			suffix = effectSuffixes[dir]
		}
		out = append(out, EffectFrame{Picture: fmt.Sprintf("S%d%s", e.ID, suffix), Frame: frame, Frames: int(e.Frames), At: motionPoint(cell, target, at)})
	}
	return out
}

// frameAt is FUN_00372be4's frame after t: First stepping toward Last.
func (e MotionEffect) frameAt(t time.Duration) (int, bool) {
	steps := 0
	if e.Interval > 0 && e.Animate != 0 {
		steps = int(t / e.Interval)
	}
	first, last := int(e.First), int(e.Last)
	step, span := 1, last-first+1
	if last < first {
		step, span = -1, first-last+1
	}
	frame := func(n int) int { return min(max(first+step*n, 1), int(e.Frames)) }
	switch e.Animate {
	case EffectLoop:
		if e.Repeat > 0 && steps >= span*int(e.Repeat) {
			return 0, false
		}
		return frame(steps % span), true
	case EffectOnce:
		if steps >= span {
			return 0, false
		}
	}
	return frame(min(steps, span-1)), true
}

// pointAt places the effect on its points: the first while it stays, else
// travelling them at Speed (Chebyshev distance, as motions travel).
func (e MotionEffect) pointAt(t time.Duration) (image.Point, bool) {
	p := e.Points[0]
	if e.Move == EffectStay || len(e.Points) == 1 {
		return p, false
	}
	speed := e.Speed
	if speed <= 0 {
		speed = compatibilityMotionSpeed
	}
	for _, next := range e.Points[1:] {
		d := max(math.Abs(float64(next.X-p.X)), math.Abs(float64(next.Y-p.Y)))
		travel := time.Duration(d / speed * float64(time.Millisecond))
		if t < travel {
			return interpolateMotion(p, next, t, travel), false
		}
		t -= travel
		p = next
	}
	return p, e.Move == EffectTravelTo
}

// end is how long a finite effect shows: played once, or looped Repeat
// times.
func (e MotionEffect) end() (time.Duration, bool) {
	if len(e.Points) == 0 || e.Frames == 0 || e.First == 0 {
		return 0, false
	}
	span := int(e.Last) - int(e.First)
	if span < 0 {
		span = -span
	}
	span++
	switch {
	case e.Animate == EffectOnce:
		return time.Duration(span) * e.Interval, true
	case e.Animate == EffectLoop && e.Repeat > 0:
		return time.Duration(span*int(e.Repeat)) * e.Interval, true
	}
	return 0, false
}
