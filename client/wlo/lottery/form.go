package lottery

import "wonderland-gonline/internal/localization"

import (
	"fmt"
	"strconv"

	"wonderland-gonline/client/wlo/seui"
)

const (
	formWidth         = 171 // FUN_0015aaec, Form_LotteryOL.
	formHeight        = 183
	formRightMargin   = 12
	formTop           = 48
	iconSize          = 32
	iconLeft          = 69
	iconTop           = 72
	textInk           = 0x0841
	textHeight        = 15
	previewRows       = 10
	previewWidth      = 207
	previewExtraWidth = 130
	previewHeight     = 427
	previewRowStep    = 36
	previewColumnStep = 150
)

type Form struct {
	seui.FixedForm
	State                                State
	Spin, Close, CloseCorner, RewardIcon *seui.FixedButton
	Preview                              *Preview
	OnSpin                               func()
	CanDraw                              func() bool
	DrawItem                             func(Reward, int, int)
	ItemName                             func(uint16) string
}

// NewForm uses FUN_0015aaec's controls in a draggable window below the right
// toolbar. Clicking the icon opens the reward list; opening never draws a prize.
func NewForm(env *seui.Env) *Form {
	f := &Form{}
	f.InitFixedForm(f, env)
	f.Name = "TLa_LotteryOLForm"
	f.Init("Form_LotteryOL", seui.MainAreaW-formWidth-formRightMargin, formHeight, formWidth, 0, 0, true, formHeight, formWidth, formTop)
	button := func(pic string, x, y, width, height int, click func()) *seui.FixedButton {
		b := seui.NewFixedButton(env, f)
		b.Init(pic, x, height, width, 0, 0, true, height, width, y)
		b.OnClick = click
		return b
	}
	f.CloseCorner = button("Btn_Close_s_1", 145, 5, 18, 18, f.Hide)
	f.Close = button("Btn_Close_2", 92, 142, 39, 20, f.Hide)
	f.Spin = button("Btn_LotteryOL", 37, 142, 39, 20, func() {
		if f.OnSpin != nil {
			f.OnSpin()
		}
	})
	f.RewardIcon = button("", iconLeft, iconTop, iconSize, iconSize, func() { f.Preview.Open() })
	f.RewardIcon.BindLanguage("hint", func() { f.RewardIcon.SetHint([]byte(localization.UI("ui.lottery.click_to_see_rewards_b99fd419"))) })
	f.Preview = newPreview(env, f)
	f.Refresh()
	return f
}

func (f *Form) Refresh() {
	f.Spin.Enabled = f.State.Ready && len(f.State.Rewards) > 0 && f.State.Remaining() > 0 && !f.State.Busy() && (f.CanDraw == nil || f.CanDraw())
	hint := localization.UI("ui.lottery.draw_once_de04c241")
	switch {
	case !f.State.Ready:
		hint = localization.UI("ui.lottery.rewards_have_not_arrived_yet_da5529af")
	case len(f.State.Rewards) == 0:
		hint = localization.UI("ui.lottery.no_rewards_are_available_2a78b6ec")
	case f.State.Pending:
		hint = localization.UI("ui.lottery.waiting_for_the_draw_result_688e659a")
	case f.State.Rolling:
		hint = "Drawing..."
	case f.State.Remaining() == 0:
		hint = localization.UI("ui.lottery.no_draws_remain_come_again_tomorrow_fb83a87f")
	}
	f.Spin.SetHint([]byte(hint))
}
func (f *Form) Show() { f.Refresh(); f.FixedForm.Show() }
func (f *Form) Hide() {
	f.State.CancelAnimation()
	f.Preview.Hide()
	f.FixedForm.Hide()
}
func (f *Form) Reset() { f.Hide(); f.State.Reset(); f.Refresh() }

func (f *Form) Paint() {
	f.Refresh()
	f.FixedForm.Paint()
	at := f.Abs()
	remaining := "?"
	if f.State.Ready {
		remaining = strconv.Itoa(f.State.Remaining())
	}
	f.Env.Text.Draw(at.X+84, at.Y+116, 0, false, true, f.Env.Screen, []byte(remaining), textHeight, 8, 0, textInk, 0)
	if reward, ok := f.State.DisplayReward(); ok && f.DrawItem != nil {
		f.DrawItem(reward, at.X+iconLeft, at.Y+iconTop)
	} else {
		f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find("Icon_UnKnow"), at.X+iconLeft, at.Y+iconTop, true)
	}
}

type Preview struct {
	seui.Form
	draw               *Form
	CloseCorner, Close *seui.FixedButton
}

// The native Article panel (FUN_0015a3c0/0015aa00) lists up to twenty
// rewards, ten rows per column, in the catalog's one-based slot order.
func newPreview(env *seui.Env, draw *Form) *Preview {
	p := &Preview{draw: draw}
	p.InitForm(p, env)
	p.Name = "TLa_LotteryOLViewForm"
	p.Dockable = false
	p.Init("Article", 0, previewHeight, previewWidth, 0, 0, true, previewHeight, previewWidth, 60)
	p.CloseCorner = seui.NewFixedButton(env, p)
	p.CloseCorner.Init("Btn_Close_S_1", 186, 18, 18, 0, 0, true, 18, 18, 2)
	p.CloseCorner.OnClick = p.Hide
	p.Close = seui.NewFixedButton(env, p)
	p.Close.Init("Btn_Cancel_1", 75, 20, 56, 0, 0, true, 20, 56, 400)
	p.Close.OnClick = p.Hide
	return p
}
func (p *Preview) Open() {
	p.Width = previewWidth
	if len(p.draw.State.Rewards) > previewRows {
		p.Width += previewExtraWidth
	}
	p.Left, p.Top = (seui.MainAreaW-p.Width)/2, 60
	p.CloseCorner.Left, p.Close.Left = p.Width-20, (p.Width-56)/2
	p.Show()
}
func (p *Preview) Paint() {
	p.Form.Paint()
	a := p.Abs()
	p.Env.Text.Draw(a.X+24, a.Y+10, 0, false, true, p.Env.Screen, []byte(localization.UI("ui.lottery.lucky_draw_rewards_902b6ea4")), textHeight, p.Width-48, 0, textInk, 0)
	for i, reward := range p.draw.State.Rewards {
		x, y := a.X+(i/previewRows)*previewColumnStep+8, a.Y+(i%previewRows)*previewRowStep+37
		if p.draw.DrawItem != nil {
			p.draw.DrawItem(reward, x, y)
		}
		name := fmt.Sprintf(localization.UI("ui.lottery.item_d_c8edea5f"), reward.ID)
		if p.draw.ItemName != nil {
			name = p.draw.ItemName(reward.ID)
		}
		if reward.Quantity > 1 {
			name += " x" + strconv.Itoa(int(reward.Quantity))
		}
		p.Env.Text.Draw(x+iconSize, y+8, 0, false, true, p.Env.Screen, []byte(name), textHeight, previewColumnStep-iconSize-4, 0, textInk, 0)
	}
}
