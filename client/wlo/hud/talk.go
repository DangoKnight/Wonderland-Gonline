package hud

import (
	"bytes"
	"image"
	"math/rand"
	"time"

	"wonderland-go/client/wlo/seui"
	"wonderland-go/client/wlo/surface"
)

// Talk is the event talk window (PTR_DAT_004c9950, a TSe_TalkMsgFormPlus
// built by FUN_0034bfa4) as event dialogue uses it: the interpreter
// (FUN_00304fd0, kind 1) sets the speaker (FUN_0034c814) and the line
// (slot +0xa0, with its markup, talkmarkup.go). The "next" icon
// (Icon_Nextone_1, two frames swapped every 250 ms, FUN_0034c9e4) bobs at
// the bottom centre.
//
// FUN_0034c814 picks the speaker's mode. Players and NPCs with a face
// sprite (Npc.dat +0x58) use face mode (FUN_0034d990): an NPC's face
// stands at the window's right, a player's at its left, with the
// speaker's name below it. An NPC without one keeps body mode
// (FUN_0034cb7c): its whole sprite stands at the right, facing down-left
// (action 0xb), with its name below.
//
// The window fits its content (FUN_0034ba20): the longest line's 8-pixel
// cells plus the margins (0x32 each side) and 10 wide, the lines plus the
// margins (0x1f above and below) and 4 tall. A face adds 0x78 to the width
// and makes it at least 0x9c tall; a body adds its frame's width + 0x1e
// and makes it at least its height + 0x46 tall; an NPC whose name stands
// lower (+0x5b) adds 0x44. Then 5 more, and it is centred at its
// constructed top (0x19). Both captures (Ship_Deck_Talking.png, the cat's
// Cat_Dialogue.png) match. Lines wrap at 39 to 44
// characters there; the exact width is not traced. Sound tags play when the line
// shows, as the typing effect that reaches them is not ported. Not ported
// either: the OK, Yes, Cancel and Close buttons, the scroll bar, face
// expressions (#F1..#F3) and the recorded voices (FUN_0034d3e8, Data\odd.dat,
// which the reference install does not have).
type Talk struct {
	Env *seui.Env
	Now func() time.Time
	// Sound plays a client sound path.
	Sound func(path string)

	panel *seui.Panel
	next  int
	text  talkText
	lines [][2]int // byte ranges of text
	who   Speaker
	shown bool // a line is open (+0x196)
	w, h  int  // the fitted size
	// Lowered is FUN_0046fe50's local_14: in the game modes 1, 3 and 4
	// (PTR_DAT_004ca2f8 +0x1d; the HUD is hidden then, as in
	// Player_Dialogue.png) the wide form sits 0x19 lower.
	Lowered bool
	// big is the wide form; origin the window's final top-left.
	big      bool
	origin   image.Point
	bigPanel *seui.Panel
	// A question's options (FUN_0034d00c): rows under the text and a
	// Close button below them; listTop is the first row's offset.
	options  [][]byte
	yesNo    bool // OK and Cancel buttons instead of a list
	optRows  []optionRow
	listTop  int
	buttonsY int
	// Pointer is the mouse position, for the hovered option.
	Pointer func() (x, y int)
	close   int
	ok      int
	cancel  int
	// The open and close animation (FUN_00472664, FUN_00472868).
	visible    bool      // +0x14: drawn, opening, open or closing
	textOn     bool      // +0x17a: the content is drawn
	cols, rows int       // +0x168, +0x16c
	start      time.Time // +0x130
	cw, ch     int       // the current size
	blinkAt    time.Time
	blinkTo    time.Time
}

// Face draws a speaker's face with its feet (anchor) at (x, y).
type Face func(dst *surface.Surface, x, y int, blinking bool)

// Speaker is who says a line.
type Speaker struct {
	Name []byte
	// Face is the face-mode drawer; Left puts it at the window's left (a
	// player).
	Face Face
	Left bool
	// Body draws the whole NPC in body mode, its feet at (x, y); BodyW and
	// BodyH are its frame's size (the library's +8, +0xc).
	Body         func(dst *surface.Surface, x, y int)
	BodyW, BodyH int
	// Art draws the player's large art (face action 3) for the wide form
	// (FUN_0046fe50's Form_Talk branch, +0x2a8); ArtW is the body sprite's
	// frame width in that action (FUN_00437cd4).
	Art  Face
	ArtW int
	// Npc.dat +0x56 and +0x57 (memory +0x5a, +0x5b): the sprite stands
	// lower, and the name lower still.
	BodyLow, NameLow bool
}

const (
	talkScreenW   = 800
	talkTop       = 0x19 // FUN_0034bfa4's Init
	talkMarginX   = 0x32
	talkMarginY   = 0x1f
	talkPadW      = 10
	talkPadH      = 4
	talkFaceW     = 0x78
	talkFaceMinH  = 0x9c
	talkFaceTextH = 0x82
	talkBodyPadW  = 0x1e
	talkBodyPadH  = 0x46
	talkNameLowH  = 0x44
	talkExtraH    = 5
	// The option list (FUN_0034bfa4's list at (6, 8) in its panel at 0xe,
	// FUN_0034ba20): 0x14 a row.
	talkButtonRowH  = 0x28
	talkButtonsUp   = 0x2d
	talkCloseRowH   = 0x20
	talkButtonGap   = 0x14
	talkButtonsMinW = 0xaa
	talkListAbove   = 0x12
	talkListInsetY  = 6
	// Measured from Question_Robinson.png: the rows run from 18
	// px inside the window's left to 23 px inside its right, inside a
	// 2-pixel frame; the text starts 5 px into a row.
	talkListLeft     = 18
	talkListRight    = 23
	talkListFrame    = 2
	talkListTextX    = 5
	talkListInk      = 0x8dfe // the frame and the hover bar, (140, 190, 247)
	talkOptionH      = 0x14
	talkListGap      = 8
	talkButtonStates = 3 // normal, hovered, pressed, stacked
	talkOptionInk    = 0xffe0
	// FUN_0046fcf4: four column and four row steps, one step per
	// millisecond of the frame clock (+0x140 = 1.0); the in-between sizes
	// are the margins plus 0x14 wide and 6 tall.
	talkSteps     = 4
	talkStepEvery = time.Millisecond
	talkGrowW     = 0x14
	talkGrowH     = 6
	talkLineH     = 18
	talkCharW     = 8
	talkWrapChars = 0x28 // +0x294, the characters a line holds
	// The wide form (FUN_0046fe50): Form_Talk_1 at (0x23, 0x195), 0x300 ×
	// 0xa9, margins 200 and 0x2d, lines of 0x2d characters, left-aligned;
	// the art's feet at (its width / 2 + 0xa7, bottom − 0x1e)
	// (FUN_0034d990).
	talkBigLeft     = 0x23
	talkBigTop      = 0x195
	talkBigW        = 0x300
	talkBigH        = 0xa9
	talkBigWrap     = 0x2d
	talkBigTextX    = 200 + 9
	talkBigTextY    = 0x2d + 1
	talkBigArtX     = 0xa7
	talkBigArtUp    = 0x1e
	talkLowered     = 0x19
	talkTextY       = 0x21
	talkTextShift   = -3
	talkInk         = 0xffe0
	talkStyle       = 2
	talkNPCFaceX    = -0x49 // from the right edge
	talkFaceY       = 0x46
	talkPlayerFaceX = 0x2d + 0x40 // 0x2d plus half the face sprite (+8), taken as 128 wide
	talkNameY       = 0x24
	talkNameInk     = 0xffff
	// Body mode (FUN_0034cb7c): the feet at (right − width/2 − 0x1e,
	// top + height + 0x14); +0x5a moves them to top + 0x56 + 0x14, +0x5b
	// moves the name 0x28 down; the name is 0x10 below.
	talkBodyX      = 0x1e
	talkBodyY      = 0x14
	talkBodyLowY   = 0x56
	talkBodyNameY  = 0x10
	talkNameLowY   = 0x28
	talkNextBottom = 0x19
	talkNextFrames = 2
	talkNextEvery  = 250 * time.Millisecond
	// The face blinks for 50 ms every 6 to 8 seconds (+0x320, +0x330).
	talkBlinkEvery  = 6000 * time.Millisecond
	talkBlinkSpread = 2000
	talkBlinkFor    = 50 * time.Millisecond
	soundDir        = `sound\`
	soundExt        = ".wav"
)

// NewTalk is FUN_0034bfa4's resting window.
func NewTalk(env *seui.Env) *Talk {
	t := &Talk{Env: env, Now: time.Now, next: env.Pics.Find("Icon_Nextone_1"), close: env.Pics.Find("Btn_Close_1"),
		ok: env.Pics.Find("btn_ok_1"), cancel: env.Pics.Find("btn_cancel_1")}
	t.panel = seui.NewPanel(env, nil)
	t.panel.Init("panel22", 0, 0xa8, 0x14c, 0, 0, true, 0, 0, 0)
	t.panel.SetMargins(0x32, 0x20, 0x20, 0x3c)
	t.panel.Hover = false
	t.bigPanel = seui.NewPanel(env, nil)
	t.bigPanel.Init("Form_Talk_1", 0, talkBigH, talkBigW, 0, 0, true, 0, 0, 0)
	t.bigPanel.SetMargins(0x32, 0x20, 0x20, 0x3c)
	t.bigPanel.Hover = false
	return t
}

// Say shows one line, its markup applied with player as #n, and plays its
// sound tags.
func (t *Talk) Say(text []byte, who Speaker, player []byte) {
	t.text = parseTalk(text, player)
	t.big = who.Art != nil
	if t.big {
		t.lines = wrapWidth(t.text.text, talkBigWrap)
	} else {
		t.lines = wrapTalk(t.text.text)
	}
	t.who, t.shown, t.options, t.yesNo = who, true, nil, false
	t.fit()
	// FUN_0046fe50 restarts the animation for every line.
	t.visible, t.textOn, t.cols, t.rows, t.start = true, false, 0, 0, t.Now()
	for _, s := range t.text.sounds {
		if t.Sound != nil {
			t.Sound(soundDir + s + soundExt)
		}
	}
}

// fit is FUN_0034ba20's size; the wide form keeps its own.
func (t *Talk) fit() {
	if t.big {
		t.w, t.h, t.origin = talkBigW, talkBigH, image.Pt(talkBigLeft, talkBigTop)
		if t.Lowered {
			t.origin.Y += talkLowered
		}
		return
	}
	defer func() { t.origin = image.Pt((talkScreenW-t.w)/2, talkTop) }()
	longest := 0
	for _, r := range t.lines {
		longest = max(longest, r[1]-r[0])
	}
	t.w = longest*talkCharW + 2*talkMarginX + talkPadW
	t.h = len(t.lines)*talkLineH + 2*talkMarginY + talkPadH
	who := t.who
	switch {
	case who.Face != nil:
		if t.h < talkFaceTextH {
			t.h = talkFaceMinH
		}
		t.w += talkFaceW
	case who.Body != nil:
		t.h = max(t.h, who.BodyH+talkBodyPadH)
		t.w += who.BodyW + talkBodyPadW
		if who.NameLow {
			t.h += talkNameLowH
		}
	}
	switch {
	case t.yesNo:
		// FUN_0034d150's OK and Cancel buttons: a 0x28 row, at least
		// 0xaa wide, the buttons 0x2d above its bottom.
		t.w = max(t.w, talkButtonsMinW)
		t.h += talkButtonRowH
		t.buttonsY = t.h - talkButtonsUp
	case len(t.options) > 0:
		// FUN_0034d00c's list (+0x2b8): its panel 0x12 above the text's
		// bottom, the rows 8 into it, 0x14 apart, 0xc + 8 more; then
		// the Close button (+0x308) 0x20 lower, 0x2d above the bottom.
		// The window keeps the text's width; options wrap within the
		// list (0x21 narrower than the window).
		t.optRows = t.optRows[:0]
		per := max((t.w-talkListLeft-talkListRight-talkListTextX)/talkCharW, 1)
		for i, o := range t.options {
			for _, r := range wrapWidth(o, per) {
				t.optRows = append(t.optRows, optionRow{o[r[0]:r[1]], i})
			}
		}
		t.listTop = t.h - talkListAbove + talkListInsetY
		t.h += len(t.optRows)*talkOptionH + talkListGap + talkCloseRowH
		t.buttonsY = t.h - talkButtonsUp
	}
	t.h += talkExtraH
}

// Ask shows a question: its prompt as the line and its options, which
// Pick reports; yesNo shows OK and Cancel buttons for two options instead.
func (t *Talk) Ask(prompt [][]byte, who Speaker, player []byte, options [][]byte, yesNo bool) {
	t.Say(bytes.Join(prompt, []byte{'\r'}), who, player)
	t.yesNo = yesNo
	t.options = make([][]byte, len(options))
	for i, o := range options {
		t.options[i] = parseTalk(o, player).text
	}
	t.fit()
}

// Choosing reports whether options are showing.
func (t *Talk) Choosing() bool { return t.shown && len(t.options) > 0 }

// Pick is a click on the window while choosing: the option's row (from 0),
// or cancel for the Close button, or neither.
func (t *Talk) Pick(x, y int) (row int, cancel bool) {
	row = -1
	if !t.textOn || len(t.options) == 0 {
		return row, false
	}
	r, p := t.rect(), image.Pt(x, y)
	if t.yesNo {
		ok, cancel := t.buttonRects(r)
		switch {
		case p.In(ok):
			return 0, false
		case p.In(cancel):
			return 1, false
		}
		return row, false
	}
	for i, o := range t.optRows {
		if p.In(t.optionRect(r, i)) {
			return o.option, false
		}
	}
	return row, p.In(t.closeRect(r))
}

// buttonRects are FUN_0034ba20's OK and Cancel places: 0x14 either side of
// the centre, 0x2d above the row's bottom.
func (t *Talk) buttonRects(r image.Rectangle) (ok, cancel image.Rectangle) {
	w, h := t.picSize(t.ok)
	cx, y := r.Min.X+r.Dx()/2, r.Min.Y+t.buttonsY
	ok = image.Rect(cx-talkButtonGap-w, y, cx-talkButtonGap, y+h)
	w, h = t.picSize(t.cancel)
	cancel = image.Rect(cx+talkButtonGap, y, cx+talkButtonGap+w, y+h)
	return
}

// picSize is one state of a three-state button picture.
func (t *Talk) picSize(pic int) (w, h int) {
	if t.Env == nil || pic < 0 {
		return 0, 0
	}
	w, h = t.Env.Pics.Size(pic)
	return w, h / talkButtonStates
}

// Options is the shown options' text.
func (t *Talk) Options() [][]byte { return t.options }

// OptionPoint is the centre of an option's first row on screen.
func (t *Talk) OptionPoint(i int) image.Point {
	for j, o := range t.optRows {
		if o.option == i {
			r := t.optionRect(t.rect(), j)
			return r.Min.Add(r.Max).Div(2)
		}
	}
	return image.Pt(-1, -1)
}

// optionRow is one drawn row of an option.
type optionRow struct {
	text   []byte
	option int
}

// optionRect is an option row's screen rectangle.
func (t *Talk) optionRect(r image.Rectangle, i int) image.Rectangle {
	top := r.Min.Y + t.listTop + i*talkOptionH
	return image.Rect(r.Min.X+talkListLeft, top, r.Max.X-talkListRight, top+talkOptionH)
}

// closeRect is the Close button's rectangle: centred, 0x2d above the
// bottom (FUN_0034ba20 for +0x308).
func (t *Talk) closeRect(r image.Rectangle) image.Rectangle {
	w, h := t.picSize(t.close)
	x, y := r.Min.X+(r.Dx()-w)/2, r.Min.Y+t.buttonsY
	return image.Rect(x, y, x+w, y+h)
}

// Contains reports whether a screen point is on the drawn window.
func (t *Talk) Contains(x, y int) bool {
	if !t.visible {
		return false
	}
	r := t.rect()
	return image.Pt(x, y).In(r)
}

// rect is the window's current rectangle: the fitted one, shrunk about its
// centre while animating.
func (t *Talk) rect() image.Rectangle {
	left := t.origin.X + (t.w-t.cw)/2
	top := t.origin.Y + (t.h-t.ch)/2
	return image.Rect(left, top, left+t.cw, top+t.ch)
}

// steps is the size of one column and one row step (FUN_0034ba20: the
// text's width and height over steps − 1).
func (t *Talk) steps() (colW, rowH int) {
	longest := 0
	for _, r := range t.lines {
		longest = max(longest, r[1]-r[0])
	}
	return longest * talkCharW / (talkSteps - 1), len(t.lines) * talkLineH / (talkSteps - 1)
}

// elapsed is the whole steps since start, plus extra (the close adds one).
func (t *Talk) elapsed(now time.Time, extra int) int {
	return int(now.Sub(t.start)/talkStepEvery) + extra
}

// tick is the panel's tick (FUN_00477810) in style 1: opening while a line
// is open, closing otherwise.
func (t *Talk) tick(now time.Time) {
	colW, rowH := t.steps()
	baseW, baseH := 2*talkMarginX+talkGrowW, 2*talkMarginY+talkGrowH
	if t.shown {
		// FUN_00472664: the columns grow first, then the rows.
		if t.cols < talkSteps {
			t.cw, t.ch = t.cols*colW+baseW, baseH
		} else {
			t.cw = t.w
		}
		if t.cols == talkSteps {
			if t.rows < talkSteps {
				t.ch = t.rows*rowH + baseH
			} else {
				t.ch = t.h
			}
		}
		if t.cols == talkSteps && t.rows == talkSteps {
			t.textOn = true
			return
		}
		if t.cols < talkSteps {
			t.cols = t.elapsed(now, 0)
		} else {
			t.cols = talkSteps
		}
		if t.cols == talkSteps && t.rows < talkSteps {
			t.rows = t.elapsed(now, 0)
		} else {
			t.rows = talkSteps
		}
		return
	}
	// FUN_00472868: the rows shrink first, then the columns; at nothing
	// the window hides (FUN_00470ee4).
	t.textOn = false
	if t.cols == 0 && t.rows == 0 {
		t.visible = false
		return
	}
	if t.rows == talkSteps {
		t.start = now
	}
	if t.rows < 1 {
		t.rows = 0
	} else {
		t.rows = talkSteps - t.elapsed(now, 1)
	}
	if t.rows == 0 {
		t.cols = talkSteps - t.elapsed(now, 1)
	}
	t.cols = max(t.cols, 0)
	if t.rows == 0 {
		t.cw = t.cols*colW + baseW
	}
	if t.rows >= 0 {
		t.ch = t.rows*rowH + baseH
	}
}

// Drawn reports whether the window is on screen, animating or not.
func (t *Talk) Drawn() bool { return t.visible }

// Size is the window's fitted size.
func (t *Talk) Size() (w, h int) { return t.w, t.h }

// Hide closes the line; the window shrinks away over the next frames.
func (t *Talk) Hide() { t.shown = false }

// Shown reports whether a line is open.
func (t *Talk) Shown() bool { return t.shown }

// Lines is the wrapped text shown.
func (t *Talk) Lines() [][]byte {
	out := make([][]byte, len(t.lines))
	for i, r := range t.lines {
		out[i] = t.text.text[r[0]:r[1]]
	}
	return out
}

// wrapTalk splits text at its line breaks and wraps each paragraph at word
// boundaries, keeping Big5 pairs whole; it returns byte ranges.
func wrapTalk(text []byte) [][2]int { return wrapWidth(text, talkWrapChars) }

// wrapWidth is wrapTalk at per characters a line.
func wrapWidth(text []byte, per int) [][2]int {
	var lines [][2]int
	start := 0
	for start <= len(text) {
		end := len(text)
		if i := bytes.IndexAny(text[start:], "\r\n"); i >= 0 {
			end = start + i
		}
		para := [2]int{start, end}
		for para[1]-para[0] > per {
			p := text[para[0]:para[1]]
			cut := per
			for i := 0; i < per; {
				if p[i] >= 0x80 && i+1 < len(p) {
					if i+2 > per {
						cut = i
						break
					}
					i += 2
					continue
				}
				i++
			}
			if sp := bytes.LastIndexByte(p[:cut], ' '); sp > 0 {
				cut = sp + 1
			}
			lines = append(lines, [2]int{para[0], para[0] + cut})
			para[0] += cut
		}
		if para[1] > para[0] {
			lines = append(lines, para)
		}
		if end == len(text) {
			break
		}
		start = end + 1
		if text[end] == '\r' && start < len(text) && text[start] == '\n' {
			start++
		}
	}
	return lines
}

// blinking keeps the face's blink clock.
func (t *Talk) blinking(now time.Time) bool {
	if t.blinkAt.IsZero() || !now.Before(t.blinkAt) {
		if !t.blinkAt.IsZero() {
			t.blinkTo = now.Add(talkBlinkFor)
		}
		t.blinkAt = now.Add(talkBlinkEvery + time.Duration(rand.Intn(talkBlinkSpread))*time.Millisecond)
	}
	return now.Before(t.blinkTo)
}

// Draw paints the window, the text, the speaker and its name, and the
// next icon.
func (t *Talk) Draw() {
	if !t.visible {
		return
	}
	env, now := t.Env, t.Now()
	t.tick(now)
	if !t.visible {
		return
	}
	r := t.rect()
	p := t.panel
	if t.big {
		p = t.bigPanel
	}
	p.Left, p.Top, p.Width, p.Height = r.Min.X, r.Min.Y, r.Dx(), r.Dy()
	p.Paint()
	if !t.textOn {
		return
	}
	left, top := t.origin.X, t.origin.Y
	center := left + t.w/2
	for i, r := range t.lines {
		x := center - (r[1]-r[0])*talkCharW/2 + talkTextShift
		y := top + talkTextY + i*talkLineH
		if t.big {
			x, y = left+talkBigTextX, top+talkBigTextY+i*talkLineH
		}
		t.drawRuns(x, y, r)
	}
	t.drawSpeaker(left, now)
	if len(t.options) > 0 {
		t.drawOptions(r)
		return
	}
	if t.next >= 0 {
		w, h := env.Pics.Size(t.next)
		h /= talkNextFrames
		frame := int(now.UnixMilli()/talkNextEvery.Milliseconds()) % talkNextFrames
		x := center - w/2
		y := top + t.h - talkNextBottom - h
		env.Pics.DrawRect(env.Screen, t.next, x, y, image.Rect(0, frame*h, w, (frame+1)*h), true)
	}
}

// drawOptions draws the option rows, the hovered one white, and the Close
// button.
func (t *Talk) drawOptions(r image.Rectangle) {
	env := t.Env
	px, py := -1, -1
	if t.Pointer != nil {
		px, py = t.Pointer()
	}
	button := func(pic int, br image.Rectangle) {
		if pic < 0 {
			return
		}
		state := 0
		if image.Pt(px, py).In(br) {
			state = 1
		}
		env.Pics.DrawRect(env.Screen, pic, br.Min.X, br.Min.Y, image.Rect(0, state*br.Dy(), br.Dx(), (state+1)*br.Dy()), true)
	}
	if t.yesNo {
		ok, cancel := t.buttonRects(r)
		button(t.ok, ok)
		button(t.cancel, cancel)
		return
	}
	hovered := -1
	for i, o := range t.optRows {
		if image.Pt(px, py).In(t.optionRect(r, i)) {
			hovered = o.option
		}
	}
	if n := len(t.optRows); n > 0 {
		box := t.optionRect(r, 0).Union(t.optionRect(r, n-1)).Inset(-talkListFrame)
		box.Min.X, box.Max.X = box.Min.X+talkListFrame, box.Max.X-talkListFrame
		env.Screen.Frame(box, talkListInk)
		env.Screen.Frame(box.Inset(1), talkListInk)
	}
	for i, o := range t.optRows {
		or := t.optionRect(r, i)
		if o.option == hovered {
			env.Screen.Fill(or, talkListInk)
		}
		env.Text.Draw(or.Min.X+talkListTextX, or.Min.Y+2, 0, false, true, env.Screen, o.text, talkOptionH, len(o.text)*talkCharW+talkCharW, 0, talkOptionInk, talkStyle)
	}
	button(t.close, t.closeRect(r))
}

// drawRuns draws one line in runs of equal style.
func (t *Talk) drawRuns(x, y int, r [2]int) {
	env := t.Env
	for i := r[0]; i < r[1]; {
		j := i + 1
		for j < r[1] && t.text.marks[j] == t.text.marks[i] {
			j++
		}
		mark, run := t.text.marks[i], t.text.text[i:j]
		env.Text.Draw(x+(i-r[0])*talkCharW, y, 0, mark&markBold != 0, true, env.Screen, run, talkLineH,
			len(run)*talkCharW+talkCharW, 0, inkOf(mark), talkStyle)
		i = j
	}
}

// drawSpeaker draws the face or the body, and the name below it.
func (t *Talk) drawSpeaker(left int, now time.Time) {
	env, who := t.Env, t.who
	var x, nameY int
	switch {
	case who.Art != nil:
		// The wide form's art: no name (FUN_0034d990 skips it).
		y := t.origin.Y + t.h - talkBigArtUp
		if t.Lowered {
			y += talkLowered // FUN_0034d990's local_c, on top of the window's
		}
		who.Art(env.Screen, who.ArtW/2+talkBigArtX, y, t.blinking(now))
		return
	case who.Face != nil:
		x = left + t.w + talkNPCFaceX
		if who.Left {
			x = left + talkPlayerFaceX
		}
		y := talkTop + talkFaceY
		who.Face(env.Screen, x, y, t.blinking(now))
		nameY = y + talkNameY
	case who.Body != nil:
		x = left + t.w - who.BodyW/2 - talkBodyX
		y := talkTop + who.BodyH + talkBodyY
		nameY = y + talkBodyNameY
		if who.NameLow {
			nameY += talkNameLowY
		}
		if who.BodyLow {
			y += talkBodyLowY - who.BodyH
		}
		who.Body(env.Screen, x, y)
	default:
		return
	}
	if len(who.Name) > 0 {
		w := len(who.Name) * talkCharW
		env.Text.Draw(x-w/2, nameY, 0, false, true, env.Screen, who.Name, 0x14, w, 0, talkNameInk, talkStyle)
	}
}
