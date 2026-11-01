package hud

import (
	"strconv"

	"wonderland-gonline/client/wlo/seui"
)

// Business is the toolbar Trade button's menu (constructor FUN_0025b454,
// paint FUN_0025cfc0, PTR_DAT_004c9f14; tag 7 of the toolbar handler
// 0x295820 shows and hides it, FUN_0025d09c): Form_Business_1 at the main
// area's right edge, 0xb4 down, with the player's gold, Trade Items
// (0x25bdf4), and the pet, furniture and stall buttons, Abandon, Close and
// a corner close. Only Trade Items is ported; the others open forms this
// port does not have yet and do nothing.
type Business struct {
	seui.Form
	CloseCorner, Abandon, Close           *seui.FixedButton // +0x150, +0x130, +0x14c
	TradeItems, TradePets, TradeFurniture *seui.FixedButton // +0x138, +0x134, +0x13c
	StallItems, StallPets, StallFurniture *seui.FixedButton // +0x140, +0x148, +0x144
	Gold                                  func() uint32
	OnTradeItems                          func()
}

const (
	businessWidth = 0x104
	// FUN_00485774 returns 1 in this build: the tall form, Close at 0xb6.
	businessHeight = 0xd9
	businessTop    = 0xb4
	businessCloseY = 0xb6
	// The gold (+0x3b44) is drawn at (0x46, 0x46), 8 pixels a digit.
	businessGoldX   = 0x46
	businessGoldY   = 0x46
	businessGoldH   = 0xc
	businessDigitW  = 8
	businessGoldInk = 0x0841 // compatibility: the text object's current ink
)

// NewBusiness is FUN_0025b454.
func NewBusiness(env *seui.Env) *Business {
	b := &Business{}
	b.InitForm(b, env)
	b.Name = "Form_Business_1"
	b.Init("Form_Business_1", seui.MainAreaW-businessWidth, businessHeight, businessWidth, 0, 0, true, businessHeight, businessWidth, businessTop)
	button := func(picture string, left, top, w, h int, click func()) *seui.FixedButton {
		btn := seui.NewFixedButton(env, b)
		btn.Init(picture, left, h, w, 0, 0, true, h, w, top)
		btn.OnClick = click
		return btn
	}
	b.CloseCorner = button("Btn_Close_s_1", 0xd1, 0x18, 0x12, 0x12, b.Hide)
	b.Abandon = button("Btn_Abandon_1", 0xae, 0x44, 0x38, 0x14, nil)
	b.TradeItems = button("btn_TradeThing_1", 0x15, 0x60, 0x47, 0x14, func() { call(b.OnTradeItems) })
	b.TradePets = button("Btn_TradePet_1", 0x5e, 0x60, 0x47, 0x14, nil)
	b.TradeFurniture = button("Btn_TradeUtensil_1", 0xa7, 0x60, 0x47, 0x14, nil)
	b.StallItems = button("Btn_StallThing_1", 0x15, 0x7c, 0x47, 0x14, nil)
	b.StallPets = button("Btn_StallPet_1", 0x5e, 0x7c, 0x47, 0x14, nil)
	b.StallFurniture = button("Btn_StallUtensil_1", 0xa7, 0x7c, 0x47, 0x14, nil)
	b.Close = button("Btn_Close_1", 0x66, businessCloseY, 0x38, 0x14, b.Hide)
	return b
}

// Toggle is FUN_0025d09c.
func (b *Business) Toggle() {
	if b.Visible {
		b.Hide()
	} else {
		b.Show()
	}
}

// Paint is FUN_0025cfc0.
func (b *Business) Paint() {
	b.Form.Paint()
	if b.Gold == nil {
		return
	}
	s := []byte(strconv.FormatUint(uint64(b.Gold()), 10))
	at := b.Abs()
	b.Env.Text.Draw(at.X+businessGoldX, at.Y+businessGoldY, 0, false, true, b.Env.Screen, s, businessGoldH, len(s)*businessDigitW, 0, businessGoldInk, 0)
}
