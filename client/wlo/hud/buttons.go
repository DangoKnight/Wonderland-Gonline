package hud

import "wonderland-gonline/internal/localization"

import (
	"strconv"
	"time"

	"wonderland-gonline/client/wlo/seui"
)

// stateRows is the number of state rows in a button picture (idle, under
// the pointer, held); a button is its picture's width by one row.
const stateRows = 3

// newBarButton places a three-row picture button with its hint.
func newBarButton(env *seui.Env, owner seui.Control, picture string, left, top int, hint func() string) *seui.FixedButton {
	b := seui.NewFixedButton(env, owner)
	w, h := env.Pics.Size(env.Pics.Find(picture))
	h /= stateRows
	b.Init(picture, left, h, w, 0, 0, true, h, w, top)
	if hint != nil {
		b.BindLanguage("hint", func() { b.SetHint([]byte(hint())) })
	}
	return b
}

// FuncBtnForm is TRE_FuncBtnForm (constructor FUN_00295fb8), the toolbar
// at the top right: World PK, then Btn_1..Btn_9 every 0x25 pixels from
// 0x1ae (Btn_7, Guild Invite, is hidden and the two after it move left),
// and the item mall. The app wires supported actions to their forms.
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
	1: localization.UI("ui.hud.world_pk_bf29a09d"), 2: "PK", 3: localization.UI("ui.hud.jump_in_63bd1f8d"), 4: localization.UI("ui.hud.spectate_f56e0a1e"), 5: localization.UI("ui.hud.join_team_971d010d"),
	6: localization.UI("ui.hud.make_friends_cc1ba3e5"), 7: localization.UI("ui.hud.trade_9709653f"), 8: localization.UI("ui.hud.guild_invite_2af3295e"), 9: localization.UI("ui.hud.events_8d14f6e7"), 10: localization.UI("ui.hud.lucky_draw_a6113b86"),
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
		b := newBarButton(env, f, picture, slot*funcStep+funcLeft, 0, func() string { return funcHints[i] })
		b.Tag = i
		if i > funcHiddenButton {
			// The app enables Lucky Draw's animation when draws are available.
			b.SetAnimation(picture, 0, funcAnimationLast, funcAnimationFirst, funcAnimationEvery, b.Height, b.Width, 0)
		}
		f.Buttons[i] = b
	}
	f.Buttons[funcHiddenButton].SetVisible(false)
	f.Mall = newBarButton(env, f, "Btn_711", funcMallLeft, 0, func() string { return localization.UI("ui.hud.item_mall_56cd7c32") })
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
	mainButtons                = 6
	mainBackgroundX            = 0x213
	mainBackgroundY            = 0x233
	furnitureLeft              = 0x2fb
	furnitureTop               = 0x1fb
	inventoryAttentionInterval = 100 * time.Millisecond
	inventoryAttentionWidth    = 39
	inventoryAttentionHeight   = 43
)

// mainPlaces are the buttons' corners from the constructor.
var mainPlaces = [mainButtons + 1]struct {
	left, top int
	hint      string
}{
	1: {0x213, 0x223, localization.UI("ui.hud.inventory_758f487f")}, 2: {0x23b, 0x223, localization.UI("ui.hud.skills_66d0f523")}, 3: {0x26c, 0x222, localization.UI("ui.hud.alchemy_2805fc43")},
	4: {0x297, 0x225, localization.UI("ui.hud.team_5985039f")}, 5: {0x2c2, 0x220, localization.UI("ui.hud.social_f1b7505a")}, 6: {0x2ef, 0x225, localization.UI("ui.hud.options_d0db8b5e")},
}

// NewMainBtnForm is FUN_002950b8.
func NewMainBtnForm(env *seui.Env) *MainBtnForm {
	m := &MainBtnForm{background: env.Pics.Find("Btn_BackGround_2")}
	m.InitFixedForm(m, env)
	m.Name = "TRE_MainBtnForm"
	m.Draggable = false
	for i := 1; i <= mainButtons; i++ {
		p := mainPlaces[i]
		b := newBarButton(env, m, "Btn_1"+strconv.Itoa(i), p.left, p.top, func() string { return mainPlaces[i].hint })
		b.Tag = i
		m.Buttons[i] = b
	}
	m.Furniture = newBarButton(env, m, "Btn_BringUpFur_1", furnitureLeft, furnitureTop, func() string { return localization.UI("ui.hud.pickup_furn_9607789e") })
	m.Furniture.SetVisible(false)
	return m
}

// Paint is FUN_002956ac's background (Btn_BackGround_2 at 0x213, 0x233).
func (m *MainBtnForm) Paint() {
	m.Env.Pics.Draw(m.Env.Screen, m.background, mainBackgroundX, mainBackgroundY, true)
}

// InventoryAttention follows FUN_002956e0, including the animated artwork's
// different origin and 39x43 frames.
func (m *MainBtnForm) InventoryAttention(on bool) {
	b := m.Buttons[1]
	if b.Animating == on {
		return
	}
	p := mainPlaces[1]
	if on {
		b.SetPos(p.left+2, p.top+8)
		b.SetAnimation("Btn_11_1", 0, 0, 0, inventoryAttentionInterval, inventoryAttentionHeight, inventoryAttentionWidth, 0)
		b.StartAnimation()
	} else {
		b.SetPos(p.left, p.top)
		b.StopAnimation()
	}
}

func init() {
	localization.Register(func() {
		funcHints = [funcButtons + 1]string{
			1: localization.UI("ui.hud.world_pk_bf29a09d"), 2: "PK", 3: localization.UI("ui.hud.jump_in_63bd1f8d"), 4: localization.UI("ui.hud.spectate_f56e0a1e"), 5: localization.UI("ui.hud.join_team_971d010d"),
			6: localization.UI("ui.hud.make_friends_cc1ba3e5"), 7: localization.UI("ui.hud.trade_9709653f"), 8: localization.UI("ui.hud.guild_invite_2af3295e"), 9: localization.UI("ui.hud.events_8d14f6e7"), 10: localization.UI("ui.hud.lucky_draw_a6113b86"),
		}
		mainPlaces = [mainButtons + 1]struct {
			left, top int
			hint      string
		}{
			1: {0x213, 0x223, localization.UI("ui.hud.inventory_758f487f")}, 2: {0x23b, 0x223, localization.UI("ui.hud.skills_66d0f523")}, 3: {0x26c, 0x222, localization.UI("ui.hud.alchemy_2805fc43")},
			4: {0x297, 0x225, localization.UI("ui.hud.team_5985039f")}, 5: {0x2c2, 0x220, localization.UI("ui.hud.social_f1b7505a")}, 6: {0x2ef, 0x225, localization.UI("ui.hud.options_d0db8b5e")},
		}
	})
}
