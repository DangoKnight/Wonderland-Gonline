package battle

import (
	"image"
	"reflect"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/login"
)

// Independent native bytes; deliberately do not use the server's builders.
var playerWire = []byte{1, 2, 17, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 2, 100, 0, 0, 0, 20, 0, 100, 0, 0, 0, 20, 0, 1, 1, 0, 0, 0, 0}
var monsterWire = []byte{2, 7, 34, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 2, 100, 0, 0, 0, 20, 0, 100, 0, 0, 0, 20, 0, 1, 1, 0, 0, 0, 0}

func fixture(t *testing.T) *State {
	t.Helper()
	s := &State{Self: 17}
	for _, p := range [][]byte{append([]byte{11, 250, 1, 0}, playerWire...), append([]byte{11, 5}, monsterWire...), {50, 6, 4, 2, 0}, {52, 1}} {
		if h, v := s.Apply(p); !h || !v {
			t.Fatalf("rejected %x", p)
		}
	}
	return s
}
func TestRound(t *testing.T) {
	s := fixture(t)
	turn := s.Turn
	if !s.CanChoose(Cell{4, 2}) || s.CanChoose(Cell{2, 2}) {
		t.Fatal("ownership")
	}
	s.Apply([]byte{53, 5, 4, 2})
	s.Apply([]byte{52, 1})
	if s.Turn != turn || s.CanChoose(Cell{4, 2}) {
		t.Fatal("duplicate ready reset submission")
	}
	s.Apply([]byte{50, 6, 4, 2, 0})
	s.Apply([]byte{52, 1})
	if s.Turn != turn+1 || !s.CanChoose(Cell{4, 2}) {
		t.Fatal("new round")
	}
	s.Apply([]byte{11, 0, 17, 0, 0, 0, 0, 0})
	if s.Active || len(s.Fighters) > 0 {
		t.Fatal("exit")
	}
	if (Cell{4, 2}).Position() != image.Pt(660, 392) || (Cell{2, 2}).Position() != image.Pt(230, 392) {
		t.Fatal("formation geometry")
	}
}
func TestActionsAtomic(t *testing.T) {
	s := fixture(t)
	p := []byte{50, 1, 17, 0, 4, 2, 0x11, 0x27, 0, 1, 2, 2, 1, 0, 1, 0x19, 25, 0, 0, 0, 1}
	if h, v := s.Apply(p); !h || !v || len(s.Actions) != 1 || s.Actions[0].Skill != 10001 || s.Actions[0].Targets[0].Stats[0].Amount != 25 {
		t.Fatal("action decode")
	}
	for n := 2; n < len(p); n++ {
		before := *s
		before.Actions = append([]Action(nil), s.Actions...)
		if _, v := s.Apply(p[:n]); v {
			t.Fatalf("accepted truncation %d", n)
		}
		if !reflect.DeepEqual(before, *s) {
			t.Fatalf("mutation %d", n)
		}
	}
}
func TestBatchedStats(t *testing.T) {
	s := fixture(t)
	if h, v := s.Apply([]byte{51, 1, 4, 2, 0x19, 60, 0, 0, 0, 2, 2, 0x19, 40, 0, 0, 0}); !h || !v {
		t.Fatal("batch rejected")
	}
	if s.At(Cell{4, 2}).HP != 60 || s.At(Cell{2, 2}).HP != 40 {
		t.Fatal("batch values")
	}
	if _, v := s.Apply([]byte{51, 1, 4, 2, 0x19, 10, 0, 0, 0, 5, 2, 0x19, 0, 0, 0, 0}); v || s.At(Cell{4, 2}).HP != 60 {
		t.Fatal("partial mutation")
	}
}
func TestExportedAssets(t *testing.T) {
	a, e := Load(login.NewAssets("../../../data"))
	if e != nil {
		t.Fatal(e)
	}
	if len(a.Motions) < 100 || len(a.Motions[10001].Frames) != 4 {
		t.Fatal("motions")
	}
	if bg := a.Background(1); bg.Pictures != [3]uint16{59083, 59084, 59085} {
		t.Fatalf("scene %+v", bg)
	}
}

func TestNativeMotionPositionsAndDirections(t *testing.T) {
	// Actor-relative and mirrored coordinates from FUN_003a1a9c. A
	// synthetic diagonal also checks proportional travel and the hold.
	m := Motion{Frames: []MotionFrame{{Action: 35, Frame: 2, Offset: image.Pt(60, -30), Hold: 100 * time.Millisecond}}}
	for _, test := range []struct {
		cell     Cell
		at       time.Duration
		position image.Point
		action   int
	}{
		{Cell{4, 2}, 0, image.Pt(660, 392), 34},
		{Cell{4, 2}, 50 * time.Millisecond, image.Pt(630, 377), 34},
		{Cell{4, 2}, 150 * time.Millisecond, image.Pt(600, 362), 34},
		{Cell{4, 2}, 280 * time.Millisecond, image.Pt(630, 377), 34},
		{Cell{4, 2}, time.Second, image.Pt(660, 392), 34},
		{Cell{2, 2}, 150 * time.Millisecond, image.Pt(290, 362), 35},
	} {
		if got := m.PositionAt(test.cell, Cell{}, test.at); got != test.position {
			t.Fatalf("%v at %s: position %v, want %v", test.cell, test.at, got, test.position)
		}
		if action, frame := m.ActionAt(test.cell, test.at); action != test.action || frame != 2 {
			t.Fatalf("action/frame %d/%d", action, frame)
		}
	}
	for original, mirrored := range map[byte]int{2: 6, 6: 2, 10: 14, 14: 10, 4: 4, 16: 17, 17: 16, 64: 65} {
		m.Frames[0].Action = original
		if action, _ := m.ActionAt(Cell{4, 2}, 0); action != mirrored {
			t.Fatalf("mirrored action %d: %d, want %d", original, action, mirrored)
		}
	}
}

func TestNativeLateMotionCues(t *testing.T) {
	m := Motion{Frames: []MotionFrame{{Action: 16}}, Events: []MotionEvent{{Trigger: 1500, Percent: 100}}, Sounds: []SoundCue{{ID: 10001, Trigger: 2000}}}
	if m.Duration() <= 2*time.Second {
		t.Fatal("late authored cues discarded", m.Duration())
	}
}

func TestNativeRecoveryModesAndCapture(t *testing.T) {
	s := fixture(t)
	s.At(Cell{4, 2}).HP = 90
	p := []byte{50, 1, 17, 0, 4, 2, 0xf9, 0x2a, 0, 1, 4, 2, 1, 0, 1, 25, 20, 0, 0, 0, 3}
	if _, v := s.Apply(p); !v || s.At(Cell{4, 2}).HP != 100 {
		t.Fatal("native recovery")
	}
	p = []byte{50, 1, 17, 0, 4, 2, 0x18, 0x27, 0, 1, 2, 2, 1, 0, 1, 0, 0, 0, 0, 0, 1}
	if _, v := s.Apply(p); !v || !s.At(Cell{2, 2}).Removed {
		t.Fatal("captured target remained")
	}
}

func TestNativeDamageCues(t *testing.T) {
	m := Motion{DamageMode: 2, Events: []MotionEvent{{Percent: 33}, {Percent: 33}, {Percent: 34}}}
	if p := m.DamageParts(101); !reflect.DeepEqual(p, []uint32{33, 33, 35}) {
		t.Fatalf("parts %v", p)
	}
	m.Events = []MotionEvent{{Percent: 100}, {Percent: 100}, {Percent: 100}}
	if p := m.DamageParts(101); !reflect.DeepEqual(p, []uint32{101, 0, 0}) {
		t.Fatalf("excess %v", p)
	}
	if p := m.DamageParts(^uint32(0)); !reflect.DeepEqual(p, []uint32{^uint32(0), 0, 0}) {
		t.Fatalf("overflow %v", p)
	}
	a, e := Load(login.NewAssets("../../../data"))
	if e != nil {
		t.Fatal(e)
	}
	basic := a.Motions[10001]
	if basic.Events[0] != (MotionEvent{Trigger: 3, Percent: 100}) || basic.EventAt(basic.Events[0]) <= 0 || basic.Duration() < time.Second {
		t.Fatalf("basic cue %+v", basic)
	}
	flame := a.Motions[11011]
	if len(flame.Sounds) != 5 || flame.Sounds[0] != (SoundCue{ID: 10040, Trigger: 8, CompatibilityFlag: 5}) {
		t.Fatalf("native sounds %+v", flame.Sounds)
	}
}

// TestMotionTargetsTheOtherSide: approach paths aimed at a fighter of the
// other half scale with its distance, so a column-2 target is not passed.
func TestMotionTargetsTheOtherSide(t *testing.T) {
	m := Motion{Approach: 1, Frames: []MotionFrame{{Action: 35, Offset: image.Pt(488, 0), Speed: .6}, {Action: 33, Frame: 2, Offset: image.Pt(509, 0), Speed: .6, Hold: 50 * time.Millisecond}}}
	from, target := Cell{4, 2}, Cell{2, 2}
	at := m.frameSpans()[0] + m.frameSpans()[1] - time.Millisecond
	got := m.PositionAt(from, target, at)
	if got.X <= target.Position().X || got.X >= from.Position().X {
		t.Fatalf("strike at %v, target %v", got, target.Position())
	}
	want := from.Position().X + (target.Position().X-from.Position().X)*509/TargetReferenceDistance
	if got.X < want-1 || got.X > want+1 || got.Y != from.Position().Y {
		t.Fatalf("strike at %v, want x %d", got, want)
	}
	// A target in another row draws the path toward that row.
	if got := m.PositionAt(Cell{2, 1}, Cell{4, 3}, at); got.Y <= (Cell{2, 1}).Position().Y {
		t.Fatalf("path did not move toward row 3: %v", got)
	}
	// Allies and self keep actor-relative offsets.
	if got := m.PositionAt(from, Cell{3, 1}, at); got.X != from.Position().X-509 {
		t.Fatalf("ally path %v", got)
	}
}

// TestMotionTravelPoses: approach and return play the run (animated) or
// leap pose of FUN_003747f4/FUN_003753ac; keyframes keep their poses.
func TestMotionTravelPoses(t *testing.T) {
	m := Motion{Frames: []MotionFrame{{Action: 35, Frame: 0, Offset: image.Pt(300, 0), Speed: .6, Hold: 100 * time.Millisecond}}}
	from, target := Cell{4, 2}, Cell{2, 2}
	for _, tc := range []struct {
		approach, kind byte
		cell, target   Cell
		action, frame  int
	}{
		{0, 0, from, target, motionRunLeft, -1},
		{0, 0, target, from, motionRunRight, -1},
		{0, MoveKindWalk, from, target, motionWalkLeft, -1},
		{1, 0, from, target, motionLeapRightSide, 0},
		{1, 0, target, from, motionLeapLeftSide, 0},
		{1, MoveKindWalk, target, from, motionLeapWalkLeftSide, 0},
	} {
		m.Approach, m.Return = tc.approach, tc.approach
		if a, f := m.PoseAt(tc.cell, tc.target, 10*time.Millisecond, tc.kind); a != tc.action || f != tc.frame {
			t.Fatalf("%+v: approach pose %d/%d", tc, a, f)
		}
	}
	m.Approach, m.Return = 0, 0
	travel := m.frameSpans()[0] - m.Frames[0].Hold - motionTick
	if a, f := m.PoseAt(from, target, travel+time.Millisecond, 0); a != 34 || f != 0 {
		t.Fatalf("keyframe pose %d/%d", a, f)
	}
	if a, f := m.PoseAt(from, target, m.frameSpans()[0]+time.Millisecond, 0); a != motionRunRight || f != -1 {
		t.Fatalf("return pose %d/%d", a, f)
	}
}

// TestMotionReaction: the record's +0x2c is the targets' reaction motion.
func TestMotionReaction(t *testing.T) {
	b := []byte{0x10, 0, 1, 1, 1, 0x40, 6, 0, 0, 0, 0, 0, 0x40, 0, 0, 0, 0x40, 0, 0x6b, 0xea, 3, 0, 0, 0, 0, 0}
	b = append(b, make([]byte, 8)...)
	m, err := ParseMotion(b)
	if err != nil || m.Reaction != 60011 || m.Approach != 1 || m.Return != 1 {
		t.Fatalf("reaction %d approach %d return %d: %v", m.Reaction, m.Approach, m.Return, err)
	}
}

// TestMotionLeapArc: FUN_00373e58's lift rises to about 83 times the scale
// at the middle of the leap and is zero at both ends and outside it.
func TestMotionLeapArc(t *testing.T) {
	if got := leapLift(.5, 2); got != 166 {
		t.Fatalf("peak lift %d", got)
	}
	if leapLift(0, 2) != 0 || leapLift(1, 2) != 0 {
		t.Fatal("leap does not start and end on the ground")
	}
	m := Motion{Approach: 1, LeapScale: 2, Frames: []MotionFrame{{Action: 35, Offset: image.Pt(300, 0), Speed: .6, Hold: 100 * time.Millisecond}}}
	approach := m.frameSpans()[0] - m.Frames[0].Hold - motionTick
	if lift := m.LiftAt(Cell{4, 2}, Cell{2, 2}, approach/2); lift < 150 {
		t.Fatalf("mid-leap lift %d", lift)
	}
	if lift := m.LiftAt(Cell{4, 2}, Cell{2, 2}, approach+time.Millisecond); lift != 0 {
		t.Fatalf("keyframe lift %d", lift)
	}
	m.Approach = 0
	if lift := m.LiftAt(Cell{4, 2}, Cell{2, 2}, approach/2); lift != 0 {
		t.Fatalf("a run lifted %d", lift)
	}
}

// TestMotionEffects: an effect starts at its trigger, steps a frame per
// interval from First toward Last, ends after Last when played once, and
// swaps its L/R picture for fighters on the right.
func TestMotionEffects(t *testing.T) {
	m := Motion{Effects: []MotionEffect{{ID: 10416, Points: []image.Point{{2, -39}}, Interval: 60 * time.Millisecond, Frames: 9, First: 5, Last: 9, Trigger: 150, Animate: EffectOnce, Direction: 1}}}
	cell := Cell{2, 2}
	if got := m.EffectsAt(cell, Cell{}, 149*time.Millisecond); len(got) != 0 {
		t.Fatalf("before the trigger %v", got)
	}
	got := m.EffectsAt(cell, Cell{}, 150*time.Millisecond+130*time.Millisecond)
	if len(got) != 1 || got[0].Frame != 7 || got[0].Frames != 9 || got[0].Picture != "S10416L" || got[0].At != cell.Position().Add(image.Pt(2, -39)) {
		t.Fatalf("effect %+v", got)
	}
	if got := m.EffectsAt(cell, Cell{}, 150*time.Millisecond+300*time.Millisecond); len(got) != 0 {
		t.Fatalf("after the last frame %v", got)
	}
	if got := m.EffectsAt(Cell{4, 2}, Cell{}, 150*time.Millisecond); len(got) != 1 || got[0].Picture != "S10416R" {
		t.Fatalf("right-side picture %+v", got)
	}
	if d := m.Duration(); d < 450*time.Millisecond {
		t.Fatalf("duration %v does not cover the effect", d)
	}
	// A travelling effect moves along its points at its speed.
	m.Effects[0] = MotionEffect{ID: 1, Points: []image.Point{{0, 0}, {100, 0}}, Speed: .5, Interval: time.Second, Frames: 1, First: 1, Last: 1, Trigger: 101, Animate: EffectHold, Move: EffectTravel}
	got = m.EffectsAt(cell, Cell{}, 101*time.Millisecond+100*time.Millisecond)
	if len(got) != 1 || got[0].At != cell.Position().Add(image.Pt(50, 0)) {
		t.Fatalf("travelling effect %+v", got)
	}
}
