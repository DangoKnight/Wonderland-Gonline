package hud

import (
	"fmt"
	"image"

	"wonderland-gonline/client/wlo/seui"
)

// The Social window's Emote tab (LAB_0027e890, mode 1, picture
// Form_Emotion_1): five Ctrl phrases (editors at x 0x4d with their
// Btn_ctrlN_1 labels at 0x22), the emoticon grid (TEmotionImage E101…,
// four columns from (0x25, 0xb4), 0x22 apart), the action grid
// (TActionImage btn_Action_N, four columns from (0xb4, 0xb4)) and eight Alt
// slots at y 0x194. An emoticon plays expression 100 + n (FUN_00430700)
// and sends AC32:1 (LAB_0027f1e4); an action plays the pose the table at
// 0x4bd7bc names and sends AC32:2 (LAB_0027e814 → FUN_00430f90). A grid
// cell dragged onto an Alt slot puts itself there (FUN_0027f44c); Alt+N
// uses slot N (FUN_0027f248). A Ctrl label copies its phrase into the chat
// bar (0x27f140). The TAnimateImage frames are drawn as the three button
// rows; the cells' btn_blank4 backdrop is not drawn.
const (
	emoteCells      = 24
	emoteColumns    = 4
	emoteStep       = 0x22
	emoteTop        = 0xb4
	emoticonLeft    = 0x25
	actionLeft      = 0xb4
	emoteSize       = 0x20
	EmoticonBase    = 100 // expression code = 100 + cell
	EmoteSlots      = 8
	altTop          = 0x194
	CtrlPhrases     = 5
	ctrlEditLeft    = 0x4d
	ctrlEditWidth   = 0xef
	ctrlEditHeight  = 0x11
	ctrlEditMax     = 0x1d
	ctrlLabelLeft   = 0x22
	ctrlLabelWidth  = 0x2a
	ctrlLabelHeight = 0x10
	// EmoteDragDistance starts a drag (the hotbar's threshold).
	EmoteDragDistance = 4
)

// Slot kinds (+0x1f8): an emoticon or an action.
const (
	SlotEmoticon = 1
	SlotAction   = 2
)

// altLefts are the Alt slots' x (DAT_004bd824, plus one); ctrlTops the
// phrase rows' y (DAT_004bd848).
var (
	altLefts = [EmoteSlots]int{36, 71, 106, 141, 175, 211, 246, 281}
	ctrlTops = [CtrlPhrases]int{74, 94, 114, 134, 154}
	// PoseTable is 0x4bd7bc: an action cell's pose, counted from 1.
	PoseTable = [...]byte{0, 0, 8, 16, 26, 28, 30, 32, 34, 36, 38, 40, 42, 46, 54, 64}
)

// EmoteSlot is an Alt slot's content.
type EmoteSlot struct {
	Kind  byte `json:"kind"`
	Value int  `json:"value"`
}

// DefaultSlots: the constructor puts action 13 (sitting) in Alt+1.
func DefaultSlots() [EmoteSlots]EmoteSlot {
	return [EmoteSlots]EmoteSlot{{Kind: SlotAction, Value: 13}}
}

type emoteTab struct {
	ctrlEdit  [CtrlPhrases]*seui.Editor
	ctrlLabel [CtrlPhrases]*seui.FixedButton
	emoticons [emoteCells]*AnimImage
	actions   [emoteCells]*AnimImage
	alt       [EmoteSlots]*AnimImage
	Slots     [EmoteSlots]EmoteSlot
	drag      *EmoteSlot
	dragFrom  image.Point
}

func slotPicture(s EmoteSlot) string {
	switch s.Kind {
	case SlotEmoticon:
		return fmt.Sprintf("E%d", s.Value+EmoticonBase)
	case SlotAction:
		return fmt.Sprintf("btn_Action_%d", s.Value)
	}
	return ""
}

// newEmoteTab is FUN_0027c6a4's Emote part.
func (s *Social) newEmoteTab() {
	e := &s.emote
	env := s.Env
	cell := func(picture string, left, top int) *AnimImage {
		b := NewAnimImage(env, s)
		b.Init(picture, left, emoteSize, emoteSize, 0, 0, true, emoteSize, emoteSize, top)
		b.Visible = false
		return b
	}
	for i := range e.ctrlEdit {
		i := i
		ed := seui.NewEditor(env, s)
		ed.Init("", ctrlEditLeft, ctrlEditHeight, ctrlEditWidth, 0, 0, false, ctrlEditHeight, ctrlEditWidth, ctrlTops[i])
		ed.SetColor(labelColor)
		ed.AllowPaste = true
		ed.SetMaxLen(ctrlEditMax)
		ed.Visible = false
		ed.OnChange = func() {
			if s.EmotesChanged != nil {
				s.EmotesChanged()
			}
		}
		e.ctrlEdit[i] = ed
		lb := seui.NewFixedButton(env, s)
		lb.Init(fmt.Sprintf("Btn_ctrl%d_1", i+1), ctrlLabelLeft, ctrlLabelHeight, ctrlLabelWidth, 0, 0, true, ctrlLabelHeight, ctrlLabelWidth, ctrlTops[i])
		lb.Visible = false
		lb.OnClick = func() { s.UsePhrase(i) }
		e.ctrlLabel[i] = lb
	}
	for i := range e.emoticons {
		n := i + 1
		col, row := i%emoteColumns, i/emoteColumns
		b := cell(fmt.Sprintf("E%d", n+EmoticonBase), col*emoteStep+emoticonLeft, row*emoteStep+emoteTop)
		b.OnClick = func() { s.useSlot(EmoteSlot{SlotEmoticon, n}) }
		b.OnDown = func() { s.StartDrag(EmoteSlot{SlotEmoticon, n}) }
		e.emoticons[i] = b
		a := cell(fmt.Sprintf("btn_Action_%d", n), col*emoteStep+actionLeft, row*emoteStep+emoteTop)
		a.OnClick = func() { s.useSlot(EmoteSlot{SlotAction, n}) }
		a.OnDown = func() { s.StartDrag(EmoteSlot{SlotAction, n}) }
		e.actions[i] = a
	}
	for i := range e.alt {
		i := i
		b := cell("", altLefts[i], altTop)
		b.OnClick = func() { s.UseAlt(i) }
		b.OnDown = func() { s.StartDrag(e.Slots[i]) }
		e.alt[i] = b
	}
	e.Slots = DefaultSlots()
	s.SetSlots(e.Slots)
}

// showEmote shows or hides the Emote tab's controls.
func (s *Social) showEmote(on bool) {
	e := &s.emote
	for i := range e.ctrlEdit {
		e.ctrlEdit[i].SetVisible(on)
		e.ctrlLabel[i].SetVisible(on)
	}
	for i := range e.emoticons {
		e.emoticons[i].SetVisible(on && e.emoticons[i].Image >= 0)
		e.actions[i].SetVisible(on && e.actions[i].Image >= 0)
	}
	for i := range e.alt {
		e.alt[i].SetVisible(on)
	}
}

// SetSlots shows the Alt slots' pictures.
func (s *Social) SetSlots(slots [EmoteSlots]EmoteSlot) {
	e := &s.emote
	e.Slots = slots
	for i, sl := range slots {
		e.alt[i].Image = s.Env.Pics.Find(slotPicture(sl))
	}
}

// Slots is the Alt slots' content.
func (s *Social) Slots() [EmoteSlots]EmoteSlot { return s.emote.Slots }

// SetPhrases fills the Ctrl phrases; Phrases reads them.
func (s *Social) SetPhrases(p [CtrlPhrases][]byte) {
	for i, t := range p {
		s.emote.ctrlEdit[i].SetText(t)
	}
}

func (s *Social) Phrases() (out [CtrlPhrases][]byte) {
	for i, ed := range s.emote.ctrlEdit {
		out[i] = append([]byte(nil), ed.Text...)
	}
	return out
}

// UsePhrase is Ctrl+N or a Ctrl label: the phrase, if any, goes to the
// chat bar.
func (s *Social) UsePhrase(i int) {
	if t := s.emote.ctrlEdit[i].Text; len(t) > 0 && s.Phrase != nil {
		s.Phrase(t)
	}
}

// UseAlt is Alt+N (FUN_0027f248).
func (s *Social) UseAlt(i int) { s.useSlot(s.emote.Slots[i]) }

func (s *Social) useSlot(sl EmoteSlot) {
	switch sl.Kind {
	case SlotEmoticon:
		if s.Emote != nil {
			s.Emote(byte(sl.Value + EmoticonBase))
		}
	case SlotAction:
		if s.Pose != nil {
			s.Pose(sl.Value)
		}
	}
}

// StartDrag is a grid cell or slot pressed (the drag start at
// LAB_0027c654).
func (s *Social) StartDrag(sl EmoteSlot) {
	if sl.Kind == 0 {
		return
	}
	in := s.Env.UI.Input
	s.emote.drag, s.emote.dragFrom = &sl, image.Pt(in.X, in.Y)
}

// Drop ends a drag at (x, y): over an Alt slot the dragged cell takes it.
// It reports whether a drag was under way.
func (s *Social) Drop(x, y int) bool {
	e := &s.emote
	d := e.drag
	e.drag = nil
	if d == nil {
		return false
	}
	if max(abs(x-e.dragFrom.X), abs(y-e.dragFrom.Y)) < EmoteDragDistance {
		return false
	}
	for i, b := range e.alt {
		if b.Visible && b.HitTest(x, y) {
			e.Slots[i] = *d
			s.SetSlots(e.Slots)
			if s.EmotesChanged != nil {
				s.EmotesChanged()
			}
			break
		}
	}
	return true
}

// DrawDragged draws the dragged cell under the pointer.
func (s *Social) DrawDragged() {
	d := s.emote.drag
	in := s.Env.UI.Input
	if d == nil || max(abs(in.X-s.emote.dragFrom.X), abs(in.Y-s.emote.dragFrom.Y)) < EmoteDragDistance {
		return
	}
	if i := s.Env.Pics.Find(slotPicture(*d)); i >= 0 {
		s.Env.Pics.DrawRect(s.Env.Screen, i, in.X-emoteSize/2, in.Y-emoteSize/2, image.Rect(0, 0, emoteSize, emoteSize), true)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
