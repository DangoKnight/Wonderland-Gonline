package hud

import (
	"image"
	"strconv"

	"wonderland-gonline/client/wlo/seui"
)

// HotKeyForm is TSe_HotKeyForm (constructor FUN_00266098, layout
// FUN_0028f55c, paint FUN_00296870): the F1–F8 bar at the right edge,
// vertically centred, in its vertical layout (mode 1). It can be dragged
// and is kept within 800 × 0x226 (FUN_002968e0). The arrows page through
// three pages (FUN_0028f2d8, FUN_0028f2b4). Bindings render archive icons
// and delegate activation and drag/drop to the application.
type HotKeyForm struct {
	Horizontal   bool
	pageX, pageY int
	seui.FixedForm
	Slots      [hotKeySlots + 1]*seui.Component
	Minimize   *seui.FixedButton // +0x19c
	Prev, Next *seui.FixedButton // +0x178, +0x17c
	Page       byte              // +0x182, 1..3
	pages      [hotKeyPages + 1]int
	Bindings   Bindings
	Describe   func(Binding) (string, string)
	Use        func(Binding)
	BeginDrag  func(byte, Binding)
	Clear      func(byte)
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
	h := &HotKeyForm{Page: 1, pageX: hotKeyPageLeft, pageY: hotKeyPageTop}
	h.InitFixedForm(h, env)
	h.Name = "TSe_HotKeyForm"
	h.Init("Form_HotKey_V", screenW-hotKeyWidth, hotKeyHeight, hotKeyWidth, 0, 0, true, hotKeyHeight, hotKeyWidth, (screenH-hotKeyHeight)/2)
	h.SetHint([]byte("Hot Bar"))
	h.Minimize = newBarButton(env, h, "Btn_Minimize_1", hotKeyMinLeft, 0, "Toggle View")
	h.Minimize.OnClick = h.ToggleView
	for i := 1; i <= hotKeySlots; i++ {
		s := seui.NewComponent(env, h)
		s.Init("", hotKeySlotLeft, hotKeySlotSize, hotKeySlotSize, 0, 0, true, hotKeySlotSize, hotKeySlotSize, (i-1)*hotKeySlotStep+hotKeySlotTop)
		slot := byte(i)
		s.OnClick = func() {
			if h.Use != nil {
				h.Use(h.Bindings[h.Page][slot])
			}
		}
		s.OnDown = func() {
			if h.BeginDrag != nil {
				h.BeginDrag(slot, h.Bindings[h.Page][slot])
			}
		}
		s.OnUp = func() {
			if h.Clear != nil {
				h.Clear(slot)
			}
		}
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
	h.Refresh()
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
		h.Env.Pics.Draw(h.Env.Screen, h.pages[h.Page], h.Left+h.pageX, h.Top+h.pageY, true)
	}
}

func (h *HotKeyForm) Refresh() {
	for slot := 1; slot <= hotKeySlots; slot++ {
		s := h.Slots[slot]
		s.Image = -1
		s.HasHint = false
		b := h.Bindings[h.Page][slot]
		if b.ID == 0 || h.Describe == nil {
			continue
		}
		icon, name := h.Describe(b)
		s.Image = h.Env.Pics.Find(icon)
		if s.Image >= 0 {
			s.UseClip = true
			s.Clip = image.Rect(0, 0, hotKeySlotSize, hotKeySlotSize)
		}
		s.SetHint([]byte(name))
	}
}
func (h *HotKeyForm) SlotAt(x, y int) byte {
	if !h.Visible || h.Blocked() {
		return 0
	}
	for slot := 1; slot <= hotKeySlots; slot++ {
		s := h.Slots[slot]
		a := s.Abs()
		if s.Visible && image.Pt(x, y).In(image.Rect(a.X, a.Y, a.X+s.Width, a.Y+s.Height)) {
			return byte(slot)
		}
	}
	return 0
}

// ToggleView uses FUN_0028f55c's two expanded layouts. Bindings and the
// selected page are independent of orientation.
func (h *HotKeyForm) ToggleView() {
	h.Horizontal = !h.Horizontal
	if h.Horizontal {
		h.Init("Form_HotKey_H", h.Left, hotKeyWidth, hotKeyHeight, 0, 0, true, hotKeyWidth, hotKeyHeight, h.Top)
		for i := 1; i <= hotKeySlots; i++ {
			h.Slots[i].Left = 32 + (i-1)*hotKeySlotStep
			h.Slots[i].Top = 23
		}
		h.Minimize.Init("Btn_Minimize_1", 255, 19, 18, 0, 0, true, 19, 18, 38)
		h.Prev.Init("Btn_ArrowUp_2", 17, 9, 11, 0, 0, true, 9, 11, 12)
		h.Next.Init("Btn_ArrowDn_2", 17, 9, 11, 0, 0, true, 9, 11, 36)
		h.pageX, h.pageY = 15, 22
	} else {
		h.Init("Form_HotKey_V", h.Left, hotKeyHeight, hotKeyWidth, 0, 0, true, hotKeyHeight, hotKeyWidth, h.Top)
		for i := 1; i <= hotKeySlots; i++ {
			h.Slots[i].Left = hotKeySlotLeft
			h.Slots[i].Top = hotKeySlotTop + (i-1)*hotKeySlotStep
		}
		h.Minimize.Init("Btn_Minimize_1", hotKeyMinLeft, 19, 18, 0, 0, true, 19, 18, 0)
		h.Prev.Init("Btn_ArrowL_3", hotKeyPrevLeft, 11, 9, 0, 0, true, 11, 9, hotKeyArrowTop)
		h.Next.Init("Btn_ArrowR_3", hotKeyNextLeft, 11, 9, 0, 0, true, 11, 9, hotKeyArrowTop)
		h.pageX, h.pageY = hotKeyPageLeft, hotKeyPageTop
	}
	h.Left = min(h.Left, hotKeyKeepRight-h.Width)
	h.Top = min(h.Top, hotKeyKeepDown-h.Height)
}
