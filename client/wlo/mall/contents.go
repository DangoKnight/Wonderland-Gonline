package mall

import (
	"fmt"
	"image"
	"strconv"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/localization"
)

// Native Article form: FUN_002a3404/002a494c, ten rows per column.
const contentsRows = 10
const contentsMaximumRewards = 40 // Native Article has forty usable icon controls.
const contentsColumnWidth = 150
const contentsBaseWidth = 207
const contentsAddedWidth = 130
const contentsHeight = 427
const contentsTitleHeight = 33

type ContentsForm struct {
	seui.FixedForm
	mall          *Form
	frame         *seui.Panel
	Close, Cancel *seui.FixedButton
	ItemID        uint16
	Rewards       []clientassets.MallReward
}

func newContentsForm(f *Form) *ContentsForm {
	p := &ContentsForm{mall: f}
	p.InitFixedForm(p, f.Env)
	p.Name = "MallBagContents"
	p.Draggable = true
	p.Init("", 0, contentsHeight, contentsBaseWidth, 0, 0, false, contentsHeight, contentsBaseWidth, 60)
	p.frame = seui.NewPanel(f.Env, nil)
	p.frame.Init("Article", 0, contentsHeight, contentsBaseWidth, 0, 0, true, contentsHeight, contentsBaseWidth, 0)
	p.frame.SetMargins(4, 34, 34, 4)
	p.Close = seui.NewFixedButton(f.Env, p)
	p.Close.Init("Btn_Close_S_1", contentsBaseWidth-20, 18, 18, 0, 0, true, 18, 18, 2)
	p.Close.OnClick = p.Hide
	p.Cancel = seui.NewFixedButton(f.Env, p)
	p.Cancel.Init("Btn_Cancel_1", contentsBaseWidth/2-28, 20, 56, 0, 0, true, 20, 56, 400)
	p.Cancel.OnClick = p.Hide
	f.Env.UI.Add(p)
	return p
}
func (f *Form) ShowContents(id uint16) bool {
	if !f.Visible || f.InCart || f.Env.UI.Modal != nil && f.Env.UI.Modal != f.ContentsForm {
		return false
	}
	rewards := f.Contents[id]
	if len(rewards) == 0 || len(rewards) > contentsMaximumRewards {
		return false
	}
	p := f.ContentsForm
	p.ItemID = id
	p.Rewards = append(p.Rewards[:0], rewards...)
	columns := (len(rewards) + contentsRows - 1) / contentsRows
	p.Width = contentsBaseWidth + (columns-1)*contentsAddedWidth
	p.Left = max(0, f.Left+Width-p.Width-35)
	p.Top = 60
	p.Close.Left = p.Width - 20
	p.Cancel.Left = p.Width/2 - 20
	if f.LoadPictures != nil {
		for _, r := range rewards {
			if n, ok := f.Items[r.ItemID]; ok {
				f.LoadPictures(strconv.Itoa(int(n.Icon)))
			}
		}
	}
	p.FixedForm.Show()
	p.Env.UI.SetModal(true, p)
	return true
}
func (p *ContentsForm) Hide() {
	p.Dragging = false
	if p.Env.UI.Modal == p {
		p.Env.UI.SetModal(false, p)
	}
	p.FixedForm.Hide()
	p.Rewards = nil
	p.ItemID = 0
}

// While modal, own the entire pointer area so an outside press can dismiss
// without reaching the catalog or world behind this window.
func (p *ContentsForm) HitTest(x, y int) bool {
	if !p.Visible {
		return false
	}
	if p.Env.UI.Modal == p {
		return true
	}
	return p.FixedForm.HitTest(x, y)
}
func (p *ContentsForm) LeftDown(shift byte, x, y int) {
	if !image.Pt(x, y).In(image.Rect(p.Left, p.Top, p.Right(), p.Bottom())) {
		p.Hide()
		p.Env.UI.Input.Captured = p
		return
	}
	if y < p.Top+contentsTitleHeight {
		p.FixedForm.LeftDown(shift, x, y)
		p.Env.UI.Input.Captured = p
	} else {
		p.Component.LeftDown(shift, x, y)
	}
}
func (p *ContentsForm) RightDown(shift byte, x, y int) {
	if !image.Pt(x, y).In(image.Rect(p.Left, p.Top, p.Right(), p.Bottom())) {
		p.Hide()
		p.Env.UI.Input.Hovered = nil
		return
	}
	p.Component.RightDown(shift, x, y)
}
func (p *ContentsForm) ExclusivePointerCapture() bool { return true }
func (p *ContentsForm) CapturedMove(shift byte, x, y int) {
	if p.Dragging {
		p.FixedForm.MouseMove(shift, x, y)
	}
}
func (p *ContentsForm) LeftUpOutside(shift byte, x, y int) { p.FixedForm.LeftUp(shift, x, y) }
func (p *ContentsForm) Paint() {
	p.frame.Left, p.frame.Top, p.frame.Width = p.Left, p.Top, p.Width
	p.frame.Paint()
	text := *p.mall
	text.Left, text.Top = p.Left, p.Top
	titleKey := "ui.mall.contents_title"
	if p.mall.IncompleteContents[p.ItemID] {
		titleKey = "ui.mall.known_contents_title"
	}
	title := fmt.Sprintf(localization.UI(titleKey), p.mall.Items[p.ItemID].Definition.Name)
	width := len(clientassets.Big5Text(title)) * 8
	text.text(max(5, (p.Width-width)/2), 11, p.Width-30, 1, title)
	for i, r := range p.Rewards {
		x, y := 5+(i/contentsRows)*contentsColumnWidth, 35+(i%contentsRows)*36
		if !p.mall.CompanionContents[p.ItemID] || p.mall.RewardIcon == nil || !p.mall.RewardIcon(r.ItemID, image.Rect(p.Left+x, p.Top+y, p.Left+x+32, p.Top+y+32)) {
			text.item(Entry{ID: r.ItemID}, x, y)
		}
		name := p.mall.Items[r.ItemID].Definition.Name
		if p.mall.CompanionContents[p.ItemID] && p.mall.RewardName != nil {
			if value := p.mall.RewardName(r.ItemID); value != "" {
				name = value
			}
		}
		if name == "" {
			name = fmt.Sprintf("#%d", r.ItemID)
		}
		if r.Quantity > 1 {
			suffix := fmt.Sprintf("×%d", r.Quantity)
			// Preserve the count when long localized names fill the column.
			for len(clientassets.Big5Text(name+suffix))*8 > contentsColumnWidth-35 {
				runes := []rune(name)
				if len(runes) == 0 {
					break
				}
				name = string(runes[:len(runes)-1])
			}
			name += suffix
		}
		text.text(x+35, y+10, contentsColumnWidth-35, 1, name)
	}
}
func (p *ContentsForm) Hint() {
	in := p.Env.UI.Input
	x, y := in.X-p.Left, in.Y-p.Top
	if x < 5 || y < 35 || y >= 395 {
		return
	}
	column, row := (x-5)/contentsColumnWidth, (y-35)/36
	i := column*contentsRows + row
	if row >= contentsRows || i >= len(p.Rewards) {
		return
	}
	n := p.mall.Items[p.Rewards[i].ItemID]
	seui.WrappedTooltip(p.Env, in.X+16, in.Y+20, clientassets.Big5Text(n.Definition.Name+"\n"+n.Description), 0xffffff, 0x805020, 16, 36)
}

func (f *Form) LeftUp(shift byte, x, y int) {
	f.FixedForm.LeftUp(shift, x, y)
	if !f.Blocked() && !f.InCart && f.Category == TabForging && !f.State.Bonus {
		x, y = x-f.Left-forgeGridLeft, y-f.Top-forgeGridTop
		if x >= 0 && x < forgeGridColumns*forgeCellStep && y >= 0 && y < game.BagRows*forgeCellStep {
			f.selectForge(byte(y/forgeCellStep*forgeGridColumns + x/forgeCellStep + 1))
		}
		return
	}
	if f.Blocked() || f.InCart || x < f.Left+218 || x >= f.Left+375 || y < f.Top+33 {
		return
	}
	row := (y - f.Top - 33) / catalogRowHeight
	entries := f.Entries()
	index := (f.Page-1)*CatalogRows + row
	if row >= CatalogRows || index >= len(entries) {
		return
	}
	if !f.ShowContents(entries[index].ID) && !f.TryOn(entries[index].ID) {
		f.notice("ui.mall.contents_unavailable")
	}
}
