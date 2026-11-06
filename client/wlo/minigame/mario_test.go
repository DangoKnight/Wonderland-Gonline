package minigame

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/picdb"
)

type marioRig struct {
	g       *Mario
	now     time.Time
	sent    [][]byte
	notices []string
	sounds  []string
	prompts []string
	room    bool
}

func newMarioRig(kind byte) *marioRig {
	r := &marioRig{now: time.Unix(0, 0), room: true}
	g := NewMario(picdb.New(), kind, r.now)
	g.Send = func(p []byte) { r.sent = append(r.sent, p) }
	g.Notice = func(t string, _ time.Duration) { r.notices = append(r.notices, t) }
	g.Sound = func(p string) { r.sounds = append(r.sounds, p) }
	g.Confirm = func(p string, ok func()) { r.prompts = append(r.prompts, p); ok() }
	g.HasRoom = func() bool { return r.room }
	g.Name = func(id uint16) []byte { return []byte("Prize") }
	r.g = g
	return r
}

func (r *marioRig) frames(n int) {
	for range n {
		r.now = r.now.Add(marioFrame)
		r.g.Update(r.now)
	}
}

// TestMarioStrip: FUN_0016b710 holds each frame ticks+1 calls; the wrap
// check restarts the strip on its last frame's final tick, so that frame
// shows one call less.
func TestMarioStrip(t *testing.T) {
	s := marioStrip{frame: 14, last: 15, ticks: 1}
	var got []int
	for range 6 {
		got = append(got, s.forward())
		s.wrapForward()
	}
	if want := []int{14, 14, 15, 0, 0, 1}; !equalInts(got, want) {
		t.Fatalf("forward %v, want %v", got, want)
	}
	s = marioStrip{frame: 1, last: 15, ticks: 1}
	got = nil
	for range 6 {
		s.wrapReverse()
		got = append(got, s.reverse())
	}
	if want := []int{1, 1, 0, 15, 15, 14}; !equalInts(got, want) {
		t.Fatalf("reverse %v, want %v", got, want)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestMarioPlay: Mario 1 asks its price once, arms with enough points,
// sends 71/11, spins on the reply and stops the reels on the server's
// positions in order, then shows the reward and re-arms by itself.
func TestMarioPlay(t *testing.T) {
	r := newMarioRig(MarioKind)
	g := r.g
	g.Begin(r.now)
	g.Points = 30
	g.Press()
	if len(r.prompts) != 1 || r.prompts[0] != "IM Points: 30\rPrice per game 25 Points!\nContinue?" || !g.armed {
		t.Fatalf("prompts %q armed %v", r.prompts, g.armed)
	}
	g.Press()
	g.Press()
	if len(r.sent) != 1 || !bytes.Equal(r.sent[0], []byte{71, MarioKind}) || !g.pending {
		t.Fatalf("sent %x", r.sent)
	}
	// Reply 4: prize 3, reels 1, 2, 3, the lamp, two pieces.
	if !g.Receive([]byte{MarioKind, marioPrize, 3, 1, 2, 3, marioLampWin, 2}, r.now) || !g.spinning || !g.lamp {
		t.Fatal("reply 4 did not spin with the lamp")
	}
	want := [marioReels]int{marioStops[0][1], marioStops[1][2], marioStops[2][3]}
	if g.targets != want || g.message != "Got reward: 2 pcs [ Prize ]" {
		t.Fatalf("targets %v message %q", g.targets, g.message)
	}
	lamp := false
	for range 2000 {
		r.frames(1)
		if g.stage >= 1 && g.shown[0] != want[0] {
			t.Fatalf("top reel moved after stopping: %d", g.shown[0])
		}
		lamp = lamp || g.lampShown
		if len(r.notices) > 0 {
			break
		}
	}
	if g.stage != 0 || g.shown != want || !lamp {
		t.Fatalf("stage %d shown %v lamp %v", g.stage, g.shown, lamp)
	}
	stops := strings.Count(strings.Join(r.sounds, ","), "9911")
	if stops != marioReels || len(r.notices) != 1 || r.notices[0] != "Got reward: 2 pcs [ Prize ]" {
		t.Fatalf("stops %d notices %q", stops, r.notices)
	}
	// The confirmed price re-arms the machine without another prompt.
	if !g.armed || g.pending || g.spinning || len(r.prompts) != 1 {
		t.Fatalf("not re-armed: armed %v pending %v prompts %d", g.armed, g.pending, len(r.prompts))
	}
}

// TestMarioPayment: Mario 2 and 3 send their payment mode; tokens in the
// bag offer the payment choice; a denial disarms.
func TestMarioPayment(t *testing.T) {
	r := newMarioRig(Mario3Kind)
	g := r.g
	g.Begin(r.now)
	g.Tokens = func(id uint16) int {
		if id == marioPlayToken {
			return 4
		}
		return 0
	}
	var choices []string
	g.Choose = func(p string, o []string, pick func(int)) { choices = o; pick(2) }
	g.Press()
	if len(choices) != 3 || !g.armed || g.mode != MarioPayPlayTokens {
		t.Fatalf("choices %q armed %v mode %d", choices, g.armed, g.mode)
	}
	if r.prompts[0] != "Tokens left:4 pcs\rTokens per game 1 pcs\nContinue?" {
		t.Fatalf("prompt %q", r.prompts[0])
	}
	g.Press()
	if len(r.sent) != 1 || !bytes.Equal(r.sent[0], []byte{71, Mario3Kind, MarioPayPlayTokens}) {
		t.Fatalf("sent %x", r.sent)
	}
	g.Receive([]byte{Mario3Kind, marioDenied}, r.now)
	if g.armed || r.notices[len(r.notices)-1] != "Not enough Points!" {
		t.Fatalf("denial: armed %v notices %q", g.armed, r.notices)
	}
	// Too few points: no arming.
	r2 := newMarioRig(Mario2Kind)
	r2.g.Begin(r2.now)
	r2.g.Points = 99
	r2.g.Press()
	if r2.g.armed || r2.notices[0] != "Not enough Points!" {
		t.Fatalf("armed %v notices %q", r2.g.armed, r2.notices)
	}
}

// TestMarioReplies: malformed replies and other kinds are refused; a
// jackpot flashes the bingo picture; leaving shows an unshown reward.
func TestMarioReplies(t *testing.T) {
	r := newMarioRig(MarioKind)
	g := r.g
	var results []bool
	g.Result = func(win bool) { results = append(results, win) }
	for _, p := range [][]byte{
		{Mario2Kind, marioJackpot, 1, 1, 1, 1, 1},
		{MarioKind, marioJackpot, 11, 1, 1, 1, 1},
		{MarioKind, marioJackpot, 1, 9, 1, 1, 1},
		{MarioKind, marioPrize, 1, 1, 1, 1},
		{MarioKind, 9},
	} {
		if g.Receive(p, r.now) {
			t.Errorf("accepted %x", p)
		}
	}
	if !g.Receive([]byte{MarioKind, marioJackpot, 4, 5, 5, 5, 1}, r.now) || !g.jackpot {
		t.Fatal("jackpot not taken")
	}
	bingo := false
	for range 600 {
		r.frames(1)
		bingo = bingo || g.bingoShown >= 0
		if g.stage > marioLampStart+5 {
			break
		}
	}
	if !bingo {
		t.Fatal("the bingo picture never showed")
	}
	g.End(true)
	if len(results) != 1 || results[0] || len(r.notices) != 1 {
		t.Fatalf("results %v notices %q", results, r.notices)
	}
}
