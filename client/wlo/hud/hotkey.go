package hud

import (
	"strconv"

	"wonderland-go/client/wlo/seui"
)

// HotKeyForm is TSe_HotKeyForm (constructor FUN_00266098, layout
// FUN_0028f55c, paint FUN_00296870): the F1–F8 bar at the right edge,
// vertically centred, in its vertical layout (mode 1). It can be dragged
// and is kept within 800 × 0x226 (FUN_002968e0). The arrows page through
// three pages (FUN_0028f2d8, FUN_0028f2b4). Not ported yet: the slots'
// contents (FUN_002a3260: an item's or a skill's icon from the hot-key
// table), the horizontal and minimised layouts (Toggle View,
// FUN_0028f8ec) and using a slot.
type HotKeyForm struct {
	seui.FixedForm
	Slots      [hotKeySlots + 1]*seui.GrBasic
	Minimize   *seui.FixedButton // +0x19c
	Prev, Next *seui.FixedButton // +0x178, +0x17c
	Page       byte              // +0x182, 1..3
	pages      [hotKeyPages + 1]int
}

const (
	hotKeySlots     = 8
	hotKeyPages     = 3
	hotKeyWidth     = 0x38
	hotKeyHeight    = 0x112
	hotKeySlotSize  = 0x18
	hotKeySlotLeft  = 0x17
	hotKeySlotTop   = 0x1d
	hotKeySlotStep  = 0x1b
	hotKeyMinLeft   = 0x25
	hotKeyPrevLeft  = 0xc
	hotKeyNextLeft  = 0x24
	hotKeyArrowTop  = 0xf8
	hotKeyPageLeft  = 0x17
	hotKeyPageTop   = 0xf5
	hotKeyKeepRight = 800
	hotKeyKeepDown  = 0x226
)

// NewHotKeyForm is FUN_00266098 followed by FUN_0028f55c(…, 1).
func NewHotKeyForm(env *seui.Env, screenW, screenH int) *HotKeyForm {
	h := &HotKeyForm{Page: 1}
	h.InitFixedForm(h, env)
	h.Name = "TSe_HotKeyForm"
	h.Init("Form_HotKey_V", screenW-hotKeyWidth, hotKeyHeight, hotKeyWidth, 0, 0, true, hotKeyHeight, hotKeyWidth, (screenH-hotKeyHeight)/2)
	h.SetHint([]byte("Hot Bar"))
	h.Minimize = newBarButton(env, h, "Btn_Minimize_1", hotKeyMinLeft, 0, "Toggle View")
	for i := 1; i <= hotKeySlots; i++ {
		s := seui.NewGrBasic(env, h)
		s.Init("", hotKeySlotLeft, hotKeySlotSize, hotKeySlotSize, 0, 0, true, hotKeySlotSize, hotKeySlotSize, (i-1)*hotKeySlotStep+hotKeySlotTop)
		s.Tag = i
		h.Slots[i] = s
	}
	h.Prev = newBarButton(env, h, "Btn_ArrowL_3", hotKeyPrevLeft, hotKeyArrowTop, "")
	h.Prev.OnClick = func() { h.turn(-1) }
	h.Next = newBarButton(env, h, "Btn_ArrowR_3", hotKeyNextLeft, hotKeyArrowTop, "")
	h.Next.OnClick = func() { h.turn(1) }
	for i := 1; i <= hotKeyPages; i++ {
		h.pages[i] = env.Pics.Find("HotKeyPage_" + strconv.Itoa(i))
	}
	return h
}

// turn is FUN_0028f2d8 (back) and FUN_0028f2b4 (forward): pages 1..3,
// wrapping.
func (h *HotKeyForm) turn(by int) {
	p := int(h.Page) + by
	switch {
	case p < 1:
		p = hotKeyPages
	case p > hotKeyPages:
		p = 1
	}
	h.Page = byte(p)
}

// Update is FUN_002968e0: a dragged bar stays within the play area.
func (h *HotKeyForm) Update(in *seui.Input) {
	if !h.Visible {
		return
	}
	if h.Dragging {
		h.Left = max(h.Left, 0)
		if h.Right() > hotKeyKeepRight {
			h.Left = hotKeyKeepRight - h.Width
		}
		h.Top = max(h.Top, 0)
		if h.Top+h.Height > hotKeyKeepDown {
			h.Top = hotKeyKeepDown - h.Height
		}
	}
	h.FixedForm.Update(in)
}

// Paint is FUN_00296870: the bar's picture, then the page number.
func (h *HotKeyForm) Paint() {
	h.FixedForm.Paint()
	if h.Page >= 1 && h.Page <= hotKeyPages {
		h.Env.Pics.Draw(h.Env.Screen, h.pages[h.Page], h.Left+hotKeyPageLeft, h.Top+hotKeyPageTop, true)
	}
}
