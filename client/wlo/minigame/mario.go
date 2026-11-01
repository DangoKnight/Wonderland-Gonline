package minigame

import (
	"fmt"
	"image"
	"time"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/protocol"
)

// Mario ports the three "Little Mary" fruit machines, sport kinds 11, 12
// and 14 (constructors FUN_0014618c, FUN_00146558 and FUN_00146594; VMTs
// 0x144f60, 0x145004 and 0x145090). Three 16-frame reel strips spin when
// the server answers a paid AC71 request and stop one by one on the reel
// positions it chose; a jackpot flashes the bingo picture and a lamp win
// blinks its two halves. Payment happens on the server: the client only
// checks the shown balance and the bag before asking for a play.
//
//	VMT +0x14  prize names            FUN_00146708 / 00146770 / 001467d8
//	VMT +0x18  leave (loss result)    FUN_001468cc / 001469ac / 00146a8c
//	VMT +0x1c  arm with points        FUN_00146b98 / 00147e40 / 00147fc8
//	VMT +0x20  play again             FUN_00148150 / 00148274 / 00148558
//	VMT +0x24  AC71 reply 3           FUN_00149040 / 0014920c / 00149390
//	VMT +0x28  AC71 reply 4           FUN_001488cc / 00148b48 / 00148dc4
//	frame      reels and lamps        FUN_00149694 (drawn by FUN_00149f9c)
type Mario struct {
	Round
	Kind byte
	// Send sends a packet (AC71 plays, the AC34:1 balance query).
	Send func([]byte)
	// Notice shows a timed message on the notice board (slot +0x9c).
	Notice func(text string, d time.Duration)
	// Confirm shows FUN_0034d150's OK/Cancel prompt; ok runs on OK.
	Confirm func(prompt string, ok func())
	// Choose shows FUN_0034d00c's option list; pick gets the row.
	Choose func(prompt string, options []string, pick func(int))
	// HasRoom is FUN_00146b70: the bag has an empty slot.
	HasRoom func() bool
	// Tokens is FUN_0044f684: how many of an item the bag holds.
	Tokens func(id uint16) int
	// Item draws an item's icon; Name is its Big5 name.
	Item func(dst *surface.Surface, id uint16, x, y int)
	Name func(id uint16) []byte
	// Text draws Big5 text with the font at PTR_DAT_004ca63c.
	Text func(dst *surface.Surface, x, y int, text []byte, lineH, width int, paper, ink uint16, style byte)

	// Points is +0xac, the IM point balance from 35/4.
	Points int

	armed     bool // +0x9c: paid for, the Start button plays
	finished  bool // +0x9d: the reels have stopped
	pending   bool // +0x9e: a play was requested
	announce  bool // +0x9f: the reward message is still to be shown
	spinning  bool // +0xa0
	jackpot   bool // +0xa1: the bingo picture flashes
	lamp      bool // +0xa2: the lamp halves blink
	spinSound bool // +0xa3
	lampLit   bool // +0xa4, toggled every five lamp frames
	confirmed bool // the start form's +0x140: no more price prompts
	mode      byte // +0xc4, the payment mode of Mario 2 and 3
	spinUp    int  // +0xb0
	stage     int  // +0xb4
	lampTicks int  // +0xb8
	changes   int  // +0xbc: reel frame changes since the last stop
	soundAt   time.Time
	message   string // +0xa8
	reels     [marioReels]marioStrip
	targets   [marioReels]int // +0x90..+0x98
	bingo     marioStrip      // +0x80
	frameAt   time.Time
	// What the last game frame drew, for the display-rate draw.
	shown      [marioReels]int
	bingoShown int
	lampShown  bool
	lampBright bool
}

// Mario sport kinds.
const (
	MarioKind  = 11
	Mario2Kind = 12
	Mario3Kind = 14
)

// Payment modes (+0xc4); Mario 2 and 3 send theirs with the play.
const (
	MarioPayPoints     = 1
	MarioPayFunTokens  = 2 // item 0x84d7
	MarioPayPlayTokens = 3 // item 0x84d8
	marioFunToken      = 0x84d7
	marioPlayToken     = 0x84d8
)

// AC71 replies (receive dispatcher 0x2ee353..0x2ee50f).
const (
	marioAccepted = 1
	marioDenied   = 2 // FUN_0014883c
	marioJackpot  = 3 // VMT +0x24: prize, three reels, quantity
	marioPrize    = 4 // VMT +0x28: prize, three reels, lamp, quantity
	marioBagFull  = 5
	marioLampWin  = 1 // reply 4's lamp byte
)

const (
	marioReels       = 3
	marioPrizes      = 11
	marioReelFrames  = 16 // Mario_Up/Mid/Buttom: 420 × 70 frames
	marioReelW       = 0x1a4
	marioReelH       = 0x46
	marioReelX       = 0xaf
	marioBingoX      = 0x118
	marioBingoY      = 0x58
	marioBingoSide   = 0xd2
	marioGroundX     = 0x62
	marioGroundY     = 0x10
	marioStopChanges = 3  // more frame changes than this before a reel stops
	marioLampStart   = 9  // lamps show after this stage
	marioAnnounce    = 41 // 3 + 0x26: the reward message
	marioLampToggle  = 4
	marioNameX       = -0x78
	marioNameY       = 8
	marioNameLine    = 0xf
	marioNameInk     = 0xffff
	marioNameStyle   = 2
	marioSpinSound   = 800 * time.Millisecond
	marioMessageFor  = 0xdac * time.Millisecond
	marioNoticeFor   = 0x514 * time.Millisecond
	marioFrame       = 30 * time.Millisecond
)

// marioReelY are the reels' rows; marioReelStart and marioReelTicks are
// FUN_00149604's first frames and FUN_0016b7b4's ticks per frame.
var (
	marioReelY     = [marioReels]int{0x58, 0x9e, 0xe4}
	marioReelStart = [marioReels]int{4, 0, 10}
	marioReelTicks = [marioReels]int{3, 4, 5}
	marioReelPics  = [marioReels]string{"Mario_Up", "Mario_Mid", "Mario_Buttom"}
)

// marioStops are FUN_0014966c's tables (DAT_004b9f49, 004b9f52 and
// 004b9f5b): a reel position from AC71 to the strip's frame.
var marioStops = [marioReels][9]int{
	{0, 4, 8, 10, 14, 2, 12, 0, 6},
	{0, 0, 14, 12, 8, 10, 6, 4, 2},
	{0, 10, 8, 6, 4, 0, 2, 14, 12},
}

// MarioPrizePlaces is DAT_004b9ecc: prizes 1-10's icon positions; their
// hint buttons sit MarioHintDrop lower.
var MarioPrizePlaces = [marioPrizes - 1]image.Point{
	{287, 361}, {287, 400}, {287, 439}, {287, 478}, {287, 518},
	{610, 361}, {610, 400}, {610, 438}, {610, 478}, {610, 518},
}

// MarioHintDrop is how far below an icon its prize button sits.
const MarioHintDrop = 0x18

// marioRules are each machine's prices (FUN_00146b98 0x19, 00147e40 100,
// 00147fc8 5 points; FUN_00147890 0x14 and FUN_001479dc 1 token) and prize
// tables (PTR_DAT_004ca80c, 004c9888 and 004c9ec0, shared with the slot
// machines). The tables are executable compatibility data: AC71 carries
// indexes into them.
var marioRules = map[byte]struct {
	points, tokens int
	prizes         [marioPrizes]uint16
}{
	MarioKind:  {25, 0, [marioPrizes]uint16{32095, 34123, 34116, 34115, 34114, 34112, 34111, 34110, 34109, 34108, 34107}},
	Mario2Kind: {100, 20, [marioPrizes]uint16{30063, 61044, 61039, 61037, 61035, 61033, 61032, 61031, 61030, 61028, 61026}},
	Mario3Kind: {5, 1, [marioPrizes]uint16{30063, 62009, 34152, 21159, 24125, 23132, 34020, 34015, 34016, 32162, 33019}},
}

// MarioKindSupported reports a Mario machine.
func MarioKindSupported(kind byte) bool { _, ok := marioRules[kind]; return ok }

// MarioPrice is a machine's price in points (the start form's paint,
// FUN_001b7004 / 001b7084 / 001b7104).
func MarioPrice(kind byte) int { return marioRules[kind].points }

// marioStrip is the strip animation of FUN_0016b448: frame +0x0c, last
// frame +0x10, tick +0x14 and ticks per frame +0x18.
type marioStrip struct{ frame, last, tick, ticks int }

// forward is VMT +4 (FUN_0016b710): it draws the frame, then counts a
// tick or moves on. It returns the frame drawn.
func (s *marioStrip) forward() int {
	if s.frame > s.last {
		s.frame = s.last
	}
	shown := s.frame
	if s.tick < s.ticks {
		s.tick++
	} else {
		s.tick, s.frame = 0, s.frame+1
	}
	return shown
}

// reverse is FUN_0016b834, the same backwards.
func (s *marioStrip) reverse() int {
	if s.frame < 0 {
		s.frame = 0
	}
	shown := s.frame
	if s.tick < s.ticks {
		s.tick++
	} else {
		s.tick, s.frame = 0, s.frame-1
	}
	return shown
}

// atLast is FUN_0016b6f4, atFirst FUN_0016b704; held means the frame's
// last tick.
func (s *marioStrip) atLast() bool { return s.frame >= s.last }
func (s *marioStrip) held() bool   { return s.tick == s.ticks }

// wrapForward restarts a forward strip after its last frame (FUN_0016b4a0).
func (s *marioStrip) wrapForward() {
	if s.atLast() && s.held() {
		s.frame, s.tick = 0, 0
	}
}

// wrapReverse sends a backward strip to its last frame after the first.
func (s *marioStrip) wrapReverse() {
	if s.frame == 0 && s.held() {
		s.frame, s.tick = s.last, 0
	}
}

// NewMario is the machine's constructor: the reels at their first frames
// and the AC34:1 balance query (FUN_002c2394(0x22, 1)).
func NewMario(pics *picdb.DB, kind byte, now time.Time) *Mario {
	g := &Mario{Round: newRound(pics), Kind: kind, frameAt: now, bingoShown: -1}
	for i := range g.reels {
		g.reels[i] = marioStrip{frame: marioReelStart[i], last: marioReelFrames - 1, ticks: marioReelTicks[i]}
		g.shown[i] = g.reels[i].frame
	}
	g.bingo = marioStrip{last: 1, ticks: 4}
	g.spinSound = true
	return g
}

// Query is the constructor's AC34:1 balance request; the server answers
// with 35/4.
func (g *Mario) Query() {
	g.send([]byte{protocol.CommandMallCheckout, protocol.MallCheckoutBalanceRequest, protocol.MallCheckoutModeWireCode0})
}

func (g *Mario) Pictures() []string {
	return []string{"MarioGround", "Mario_Up", "Mario_Mid", "Mario_Buttom", "Mario_Bingo", "Mario_Start"}
}

// Prizes are the machine's prize items, index 0 first.
func (g *Mario) Prizes() []uint16 {
	t := marioRules[g.Kind].prizes
	return t[:]
}

func (g *Mario) send(p []byte) {
	if g.Send != nil {
		g.Send(p)
	}
}

func (g *Mario) notice(text string, d time.Duration) {
	if g.Notice != nil {
		g.Notice(text, d)
	}
}

func (g *Mario) hasRoom() bool { return g.HasRoom == nil || g.HasRoom() }

func (g *Mario) tokens(id uint16) int {
	if g.Tokens == nil {
		return 0
	}
	return g.Tokens(id)
}

// Begin is the start form's Start (0x1b5211): the machine's form shows.
// Nothing is paid until the machine's own Start button.
func (g *Mario) Begin(now time.Time) {
	g.Started = true
	g.startedAt = now
}

// reset is FUN_00146840.
func (g *Mario) reset() {
	g.message = ""
	g.pending, g.spinning, g.announce, g.jackpot, g.armed, g.lamp = false, false, false, false, false, false
	g.spinSound = true
	g.spinUp, g.stage, g.changes = 0, 0, 0
	g.targets = [marioReels]int{}
	g.soundAt = time.Time{}
}

// Armed reports a paid-for machine waiting for its Start button.
func (g *Mario) Armed() bool { return g.armed }

// Spinning reports turning reels.
func (g *Mario) Spinning() bool { return g.spinning }

// arm is VMT +0x1c with points, or FUN_00147890 / 001479dc (Fun Tokens)
// and FUN_00147b68 / 00147cb4 (Play Tokens): with enough to pay and an
// empty bag slot, the machine is reset and armed. Prompts are skipped
// from then on, even when the bag was full.
func (g *Mario) arm(mode byte) {
	g.confirmed = false
	r := marioRules[g.Kind]
	switch mode {
	case MarioPayFunTokens, MarioPayPlayTokens:
		id := uint16(marioFunToken)
		if mode == MarioPayPlayTokens {
			id = marioPlayToken
		}
		if g.tokens(id) < r.tokens {
			g.notice("Not enough token", marioNoticeFor)
			return
		}
	default:
		if g.Points < r.points {
			g.notice("Not enough Points!", marioNoticeFor)
			return
		}
	}
	if g.hasRoom() {
		if g.Kind != MarioKind {
			g.mode = mode
		}
		g.reset()
		g.armed, g.finished = true, false
		if g.Music != nil {
			g.Music("sound\\BGM0019.wav")
		}
	} else {
		g.notice("Inv. full\nMake more space", marioNoticeFor)
	}
	g.confirmed = true
}

// pointsPrompt is the price question of FUN_00145aa4 and FUN_00146d18.
func (g *Mario) pointsPrompt() string {
	return fmt.Sprintf("IM Points: %d\rPrice per game %d Points!\nContinue?", g.Points, marioRules[g.Kind].points)
}

// tokensPrompt is FUN_001471d8's token question.
func (g *Mario) tokensPrompt(id uint16) string {
	return fmt.Sprintf("Tokens left:%d pcs\rTokens per game %d pcs\nContinue?", g.tokens(id), marioRules[g.Kind].tokens)
}

func (g *Mario) confirm(prompt string, ok func()) {
	if g.Confirm != nil {
		g.Confirm(prompt, ok)
	} else {
		ok()
	}
}

// play sends the paid request: Mario 1 sends only its kind, Mario 2 and 3
// their payment mode too (serializer 0x2da5e1, 0x2da64b and 0x2da6f3).
func (g *Mario) play() {
	g.pending = true
	p := []byte{protocol.CommandArcadeGame, g.Kind}
	if g.Kind != MarioKind {
		p = append(p, g.mode)
	}
	g.send(p)
}

// Press is the machine's Start button (FUN_00145aa4, 00145c84, 00145d94).
func (g *Mario) Press() {
	if !g.Started || g.Done() {
		return
	}
	if g.armed && !g.finished {
		if !g.pending {
			g.play()
		}
		return
	}
	if g.Kind == MarioKind {
		if !g.hasRoom() {
			g.notice("Inventory Full\nMake room for prize", marioNoticeFor)
		} else if !g.confirmed {
			g.confirm(g.pointsPrompt(), func() { g.arm(MarioPayPoints) })
		} else {
			g.arm(MarioPayPoints)
		}
		return
	}
	if !g.confirmed {
		g.payment()
	} else if g.armed && !g.pending {
		g.play()
	}
}

// payment is FUN_00146d18 / 00146f78: the price question, or with tokens
// in the bag a choice of payment first.
func (g *Mario) payment() {
	if g.tokens(marioFunToken)+g.tokens(marioPlayToken) == 0 || g.Choose == nil {
		g.confirm(g.pointsPrompt(), func() { g.arm(MarioPayPoints) })
		return
	}
	g.Choose("Payment Method?", []string{"Points", "Token", "Fun Token"}, g.paymentChoice)
}

// paymentChoice is FUN_001471d8 / 00147510.
func (g *Mario) paymentChoice(row int) {
	if !g.hasRoom() {
		g.notice("Inv. full\nMake more space", marioNoticeFor)
		return
	}
	switch row {
	case 0:
		g.confirm(g.pointsPrompt(), func() { g.arm(MarioPayPoints) })
	case 1:
		g.confirm(g.tokensPrompt(marioFunToken), func() { g.arm(MarioPayFunTokens) })
	case 2:
		g.confirm(g.tokensPrompt(marioPlayToken), func() { g.arm(MarioPayPlayTokens) })
	}
}

// again is VMT +0x20 after the reels stop: once the price was confirmed
// the machine re-arms at once, otherwise it asks (FUN_00148150's text).
func (g *Mario) again() {
	if g.confirmed {
		g.rearm()
		return
	}
	prompt := fmt.Sprintf("IM Points: %d\rPlay again?Points used%d Pts", g.Points, marioRules[g.Kind].points)
	switch g.mode {
	case MarioPayFunTokens:
		prompt = g.tokensPrompt(marioFunToken)
	case MarioPayPlayTokens:
		prompt = g.tokensPrompt(marioPlayToken)
	}
	g.confirm(prompt, g.rearm)
}

func (g *Mario) rearm() {
	mode := g.mode
	if g.Kind == MarioKind || mode == 0 {
		mode = MarioPayPoints
	}
	g.arm(mode)
}

// Exit is the machine form's Leave (0x1460e0): with a play under way it
// asks first; nothing is charged for leaving.
func (g *Mario) Exit() {
	if g.Done() {
		return
	}
	if !g.armed && !g.finished {
		g.End(false)
		return
	}
	if !g.pending {
		g.confirm("Leave the game?\nNo points will be used", func() { g.End(false) })
	}
	if g.finished {
		g.End(false)
	}
}

// End is VMT +0x18: an unshown reward message is shown first, and the
// machine always reports a loss (FUN_003bdfa4(0)).
func (g *Mario) End(bool) {
	if g.Done() {
		return
	}
	if g.announce {
		g.notice(g.message, marioMessageFor)
		g.sound("sound\\wav0152.wav")
		g.announce = false
	}
	g.Round.End(false)
}

// Receive takes AC71's payload (kind first).
func (g *Mario) Receive(p []byte, _ time.Time) bool {
	if len(p) < 2 || p[0] != g.Kind || g.Done() {
		return false
	}
	switch p[1] {
	case marioAccepted:
		return true
	case marioDenied:
		g.armed = false
		g.notice("Not enough Points!", marioNoticeFor)
		return true
	case marioBagFull:
		g.notice("Inventory Full\nMake room for prize", marioNoticeFor)
		return true
	case marioJackpot, marioPrize:
	default:
		return false
	}
	size := 7
	if p[1] == marioPrize {
		size = 8
	}
	if len(p) < size || int(p[2]) >= marioPrizes {
		return false
	}
	for i, stop := range p[3:6] {
		if int(stop) >= len(marioStops[i]) {
			return false
		}
	}
	for i, stop := range p[3:6] {
		g.targets[i] = marioStops[i][stop]
	}
	quantity := p[size-1]
	name := ""
	if g.Name != nil {
		name = string(g.Name(marioRules[g.Kind].prizes[p[2]]))
	}
	g.message = fmt.Sprintf("Got reward: %d pcs [ %s ]", quantity, name)
	g.spinning, g.announce = true, true
	if p[1] == marioJackpot {
		if p[2] != 0 {
			g.jackpot = true
		}
	} else if p[6] == marioLampWin {
		g.lamp = true
	}
	return true
}

// Update runs FUN_00149694 once per 30 ms game frame.
func (g *Mario) Update(now time.Time) {
	if now.Sub(g.frameAt) > time.Second {
		g.frameAt = now.Add(-marioFrame)
	}
	for !now.Before(g.frameAt.Add(marioFrame)) {
		g.frameAt = g.frameAt.Add(marioFrame)
		g.step(now)
	}
}

// step is FUN_00149694's logic; it records what the frame draws.
func (g *Mario) step(now time.Time) {
	g.bingoShown, g.lampShown = -1, false
	up, mid, bottom := &g.reels[0], &g.reels[1], &g.reels[2]
	if !g.spinning {
		for i := range g.reels {
			g.shown[i] = g.reels[i].frame
		}
		return
	}
	if g.spinSound && (g.soundAt.IsZero() || now.Sub(g.soundAt) >= marioSpinSound) {
		g.sound("sound\\9912.wav")
		g.soundAt = now
	}
	counted := func(s *marioStrip, move func() int) int {
		before := s.frame
		shown := move()
		if s.frame != before {
			g.changes++
		}
		return shown
	}
	stopped := func() {
		g.changes = 0
		g.stage++
		g.sound("sound\\9911.wav")
	}
	if g.spinUp < 1 {
		// The first turns: all three reels run until the top one has
		// changed frame four times.
		g.shown[0] = counted(up, up.forward)
		if g.changes > marioStopChanges {
			g.changes = 0
			g.spinUp++
		}
		up.wrapForward()
		mid.wrapReverse()
		g.shown[1] = mid.reverse()
		g.shown[2] = bottom.forward()
		bottom.wrapForward()
		return
	}
	switch {
	case g.stage == 0:
		if up.frame == g.targets[0] && up.held() && g.changes > marioStopChanges {
			stopped()
		}
		g.shown[0] = counted(up, up.forward)
		up.wrapForward()
		if g.stage == 0 {
			mid.wrapReverse()
		} else {
			g.shown[0] = g.targets[0]
		}
		g.shown[1] = mid.reverse()
		g.shown[2] = bottom.forward()
		bottom.wrapForward()
	case g.stage == 1:
		g.shown[0] = g.targets[0]
		if mid.frame == g.targets[1] && mid.held() && g.changes > marioStopChanges {
			stopped()
		}
		mid.wrapReverse()
		if g.stage == 1 {
			g.shown[1] = counted(mid, mid.reverse)
		} else {
			g.shown[1] = g.targets[1]
		}
		g.shown[2] = bottom.forward()
		bottom.wrapForward()
	case g.stage == 2:
		g.shown[0], g.shown[1] = g.targets[0], g.targets[1]
		if bottom.frame == g.targets[2] && bottom.held() && g.changes > marioStopChanges {
			// FUN_00405138(0) stops the spin sound's channel.
			stopped()
			g.spinSound = false
		}
		if g.stage == 2 {
			g.shown[2] = counted(bottom, bottom.forward)
		} else {
			g.shown[2] = g.targets[2]
		}
		bottom.wrapForward()
	case g.stage < marioAnnounce:
		g.shown = g.targets
		g.stage++
		switch {
		case g.jackpot:
			if g.stage > marioLampStart {
				if g.bingo.atLast() {
					g.bingoShown = g.bingo.reverse()
				} else {
					g.bingoShown = g.bingo.forward()
				}
			}
		case g.lamp:
			if g.stage > marioLampStart {
				g.blinkLamp()
			}
		default:
			g.stage = marioAnnounce
		}
	case g.stage == marioAnnounce:
		g.shown = g.targets
		g.notice(g.message, marioMessageFor)
		g.sound("sound\\wav0152.wav")
		g.finished, g.announce = true, false
		for i := range g.reels {
			g.reels[i].frame, g.reels[i].tick = g.targets[i], 0
		}
		g.stage++
		g.again()
	default:
		g.shown = g.targets
	}
}

// blinkLamp is FUN_0014a090: with +0xa4 clear the halves show their lit
// rows, otherwise the unlit ones; +0xa4 flips every five frames.
func (g *Mario) blinkLamp() {
	g.lampBright = !g.lampLit
	g.lampTicks++
	if g.lampTicks > marioLampToggle {
		g.lampLit, g.lampTicks = !g.lampLit, 0
	}
	g.lampShown = true
}

// Click: the machine's buttons are form controls.
func (g *Mario) Click(int, int, time.Time) {}

// Draw is FUN_00149f9c and the text pass FUN_00149fb0.
func (g *Mario) Draw(dst *surface.Surface, _, _ int, _ time.Time) {
	dst.Fill(image.Rect(0, 0, dst.W, dst.H), 0)
	if g.Pics == nil {
		return
	}
	if i := g.Pics.Find("MarioGround"); i >= 0 {
		g.Pics.Draw(dst, i, marioGroundX, marioGroundY, false)
	}
	prizes := marioRules[g.Kind].prizes
	names := make([][]byte, len(MarioPrizePlaces))
	for i, at := range MarioPrizePlaces {
		if g.Item != nil {
			g.Item(dst, prizes[i+1], at.X, at.Y-1)
		}
		if g.Name != nil {
			names[i] = g.Name(prizes[i+1])
		}
	}
	for i := range g.reels {
		g.strip(dst, marioReelPics[i], marioReelX, marioReelY[i], g.shown[i], marioReelFrames)
	}
	if b := g.Pics.Find("Mario_Bingo"); b >= 0 {
		if g.bingoShown >= 0 {
			r := image.Rect(0, g.bingoShown*marioBingoSide, marioBingoSide, (g.bingoShown+1)*marioBingoSide)
			g.Pics.DrawRect(dst, b, marioBingoX, marioBingoY, r, true)
		}
		if g.lampShown {
			// The halves (+0x84, +0x88) are rows 0-0x46 and 0x8c-0xd2 of
			// the unlit picture, or of the lit one 0xd2 lower, drawn
			// 0x46 and 0x8c below the bingo's top.
			top := 0
			if g.lampBright {
				top = marioBingoSide
			}
			for i, half := range []int{0, 2 * marioReelH} {
				y := top + half
				g.Pics.DrawRect(dst, b, marioBingoX, marioBingoY+(i+1)*marioReelH, image.Rect(0, y, marioBingoSide, y+marioReelH), true)
			}
		}
	}
	if g.Text != nil {
		for i, at := range MarioPrizePlaces {
			if len(names[i]) > 0 {
				g.Text(dst, at.X+marioNameX, at.Y+marioNameY, names[i], marioNameLine, len(names[i])*8, marioNameInk, marioNameInk, marioNameStyle)
			}
		}
	}
}
