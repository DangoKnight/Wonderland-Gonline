package hud

import (
	"strconv"
	"time"

	"wonderland-go/client/wlo/seui"
)

// stateRows is the number of state rows in a button picture (idle, under
// the pointer, held); a button is its picture's width by one row.
const stateRows = 3

// newBarButton places a three-row picture button with its hint.
func newBarButton(env *seui.Env, owner seui.Control, picture string, left, top int, hint string) *seui.FixedButton {
	b := seui.NewFixedButton(env, owner)
	w, h := env.Pics.Size(env.Pics.Find(picture))
	h /= stateRows
	b.Init(picture, left, h, w, 0, 0, true, h, w, top)
	if hint != "" {
		b.SetHint([]byte(hint))
	}
	return b
}

// FuncBtnForm is TRE_FuncBtnForm (constructor FUN_00295fb8), the toolbar
// at the top right: World PK, then Btn_1..Btn_9 every 0x25 pixels from
// 0x1ae (Btn_7, Guild Invite, is hidden and the two after it move left),
// and the item mall. The buttons have no actions yet.
type FuncBtnForm struct {
	seui.FixedForm
	Buttons    [funcButtons + 1]*seui.FixedButton
	Mall       *seui.FixedButton
	background int
}

const (
	funcButtons        = 10
	funcLeft           = 0x1ae
	funcStep           = 0x25
	funcBackgroundX    = 0x1b0
	funcHiddenButton   = 8 // Guild Invite
	funcMallLeft       = 0x2fb
	funcAnimationEvery = 300 * time.Millisecond
	funcAnimationFirst = 2
	funcAnimationLast  = 3
)

var funcHints = [funcButtons + 1]string{
	1: "World PK", 2: "PK", 3: "Jump In", 4: "Spectate", 5: "Join Team",
	6: "Make Friends", 7: "Trade", 8: "Guild Invite", 9: "Events", 10: "Lucky Draw",
}

// NewFuncBtnForm is FUN_00295fb8.
func NewFuncBtnForm(env *seui.Env) *FuncBtnForm {
	f := &FuncBtnForm{background: env.Pics.Find("Btn_BackGround_1")}
	f.InitFixedForm(f, env)
	f.Name = "TRE_FuncBtnForm"
	f.Draggable = false
	for i := 1; i <= funcButtons; i++ {
		picture, slot := "Btn_"+strconv.Itoa(i-1), i-1
		if i == 1 {
			picture = "Btn_PlayerBattle"
		}
		if i > funcHiddenButton {
			slot-- // placed over the hidden button
		}
		b := newBarButton(env, f, picture, slot*funcStep+funcLeft, 0, funcHints[i])
		b.Tag = i
		if i > funcHiddenButton {
			// Cycles while an event or the lottery is open (not ported).
			b.SetAnimation(picture, 0, funcAnimationLast, funcAnimationFirst, funcAnimationEvery, b.Height, b.Width, 0)
		}
		f.Buttons[i] = b
	}
	f.Buttons[funcHiddenButton].SetVisible(false)
	f.Mall = newBarButton(env, f, "Btn_711", funcMallLeft, 0, "Item Mall")
	return f
}

// Paint is FUN_00296744's background (Btn_BackGround_1 at 0x1b0, 0).
func (f *FuncBtnForm) Paint() {
	f.Env.Pics.Draw(f.Env.Screen, f.background, funcBackgroundX, 0, true)
}

// MainBtnForm is TRE_MainBtnForm (constructor FUN_002950b8), the menu at
// the bottom right: Btn_11..Btn_16 on Btn_BackGround_2. The buttons have
// no actions yet; Pickup Furn (Btn_BringUpFur_1) only shows at home.
type MainBtnForm struct {
	seui.FixedForm
	Buttons    [mainButtons + 1]*seui.FixedButton
	Furniture  *seui.FixedButton
	background int
}

const (
	mainButtons     = 6
	mainBackgroundX = 0x213
	mainBackgroundY = 0x233
	furnitureLeft   = 0x2fb
	furnitureTop    = 0x1fb
)

// mainPlaces are the buttons' corners from the constructor.
var mainPlaces = [mainButtons + 1]struct {
	left, top int
	hint      string
}{
	1: {0x213, 0x223, "Inventory"}, 2: {0x23b, 0x223, "Skills"}, 3: {0x26c, 0x222, "Alchemy"},
	4: {0x297, 0x225, "Team"}, 5: {0x2c2, 0x220, "Social"}, 6: {0x2ef, 0x225, "Options"},
}

// NewMainBtnForm is FUN_002950b8.
func NewMainBtnForm(env *seui.Env) *MainBtnForm {
	m := &MainBtnForm{background: env.Pics.Find("Btn_BackGround_2")}
	m.InitFixedForm(m, env)
	m.Name = "TRE_MainBtnForm"
	m.Draggable = false
	for i := 1; i <= mainButtons; i++ {
		p := mainPlaces[i]
		b := newBarButton(env, m, "Btn_1"+strconv.Itoa(i), p.left, p.top, p.hint)
		b.Tag = i
		m.Buttons[i] = b
	}
	m.Furniture = newBarButton(env, m, "Btn_BringUpFur_1", furnitureLeft, furnitureTop, "Pickup Furn")
	m.Furniture.SetVisible(false)
	return m
}

// Paint is FUN_002956ac's background (Btn_BackGround_2 at 0x213, 0x233).
func (m *MainBtnForm) Paint() {
	m.Env.Pics.Draw(m.Env.Screen, m.background, mainBackgroundX, mainBackgroundY, true)
}
