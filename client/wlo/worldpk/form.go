// Package worldpk presents the native World PK window. It owns no transport or
// gameplay rules; server-backed matchmaking and rewards are intentionally separate.
package worldpk

import (
	"image"
	"strconv"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/localization"
)

// Constructor FUN_002a9ccc and painters FUN_002aaff4 / FUN_002ab238.
const (
	Width            = 356
	Height           = 493
	nativeViewWidth  = 800
	nativeViewHeight = 600
)

const (
	PVP = iota
	Rankings
	Settings
	tabCount
	rowsPerPage    = 4
	rankingLimit   = 300
	previewChances = 10
	rankingRowStep = 78
	whiteDigitRows = 10
	pageDigitRows  = 11
	textInk        = 0x0841
)

type Entry struct {
	Name     []byte
	Level    byte
	Wins     uint32
	Portrait *surface.Surface
	Badge    string
}

// State is presentation data, ready for a future validated server adapter.
// No screenshot leaderboard identities are installed as live player data.
type State struct {
	Rank, Wins   uint32
	Chances      int
	Entries      []Entry
	AllowOffline bool
}

type Form struct {
	seui.FixedForm
	State                                             State
	Tab, Page                                         int
	Tabs                                              [tabCount]*seui.FixedButton
	Close, CloseCorner, Start, ViewRanking, MyRanking *seui.FixedButton
	First, Previous, Next, Last                       *seui.FixedButton
	Allow, Disallow, Confirm, Cancel                  *seui.FixedButton
	Rewards                                           [3]*seui.FixedButton
	draftAllow                                        bool
}

func NewForm(env *seui.Env) *Form {
	f := &Form{State: State{Chances: previewChances, AllowOffline: true}, Page: 1, draftAllow: true}
	f.InitFixedForm(f, env)
	f.Name = "WorldPK"
	f.Init("Form_PlayerBattleBG", (nativeViewWidth-Width)/2, Height, Width, 0, 0, true, Height, Width, (nativeViewHeight-Height)/2)
	button := func(pic string, x, y, w, h int, click func()) *seui.FixedButton {
		b := seui.NewFixedButton(env, f)
		b.Init(pic, x, h, w, 0, 0, true, h, w, y)
		b.OnClick = click
		return b
	}
	f.CloseCorner = button("Btn_Close_S_1", 304, 24, 18, 18, f.Hide)
	f.Close = button("Btn_Close_1", 150, 463, 56, 20, f.Hide)
	for i, pic := range []string{"Btn_PlayerBattleMain", "Btn_PlayerBattleRank", "Btn_PlayerBattleSetting"} {
		tab := i
		f.Tabs[i] = button(pic, 21+i*87, 55, 87, 21, func() { f.SelectTab(tab) })
	}
	f.ViewRanking = button("Btn_PlayerBattleCheckRank", 250, 95, 71, 20, func() { f.SelectTab(Rankings) })
	f.Start = button("Btn_PlayerBattleStart", 94, 394, 168, 47, nil)
	f.Start.Enabled = false // No matchmaking requests before packet investigation.
	for i := range f.Rewards {
		f.Rewards[i] = button("Btn_Reward01_01", 31+i*102, 223, 106, 93, nil)
		f.Rewards[i].Enabled = false
		f.Rewards[i].Visible = false
	}
	f.MyRanking = button("Btn_PlayerBattleMyRank", 25, 428, 71, 20, func() {
		if f.State.Rank > 0 {
			f.setPage((int(f.State.Rank)-1)/rowsPerPage + 1)
		}
	})
	f.First = button("Btn_ArrowL_8a", 75, 394, 31, 21, func() { f.setPage(1) })
	f.Previous = button("Btn_ArrowL_8", 121, 394, 21, 21, func() { f.setPage(f.Page - 1) })
	f.Next = button("Btn_ArrowR_8", 214, 394, 21, 21, func() { f.setPage(f.Page + 1) })
	f.Last = button("Btn_ArrowR_8a", 250, 394, 31, 21, func() { f.setPage(f.pages()) })
	f.Allow = button("btn_Check_1", 71, 306, 16, 16, func() { f.draftAllow = true; f.Refresh() })
	f.Disallow = button("btn_UnCheck_1", 217, 306, 16, 16, func() { f.draftAllow = false; f.Refresh() })
	f.Confirm = button("btn_ok_1", 96, 426, 56, 20, func() { f.State.AllowOffline = f.draftAllow; f.Refresh() })
	f.Cancel = button("Btn_Cancel_1", 196, 426, 56, 20, func() { f.draftAllow = f.State.AllowOffline; f.Refresh() })
	f.Refresh()
	return f
}

func (f *Form) pages() int {
	return max(1, (min(len(f.State.Entries), rankingLimit)+rowsPerPage-1)/rowsPerPage)
}
func (f *Form) setPage(page int) { f.Page = max(1, min(page, f.pages())); f.Refresh() }
func (f *Form) SelectTab(tab int) {
	if tab < 0 || tab >= tabCount {
		return
	}
	f.Tab = tab
	f.draftAllow = f.State.AllowOffline
	f.Refresh()
}
func (f *Form) Show() { f.draftAllow = f.State.AllowOffline; f.Refresh(); f.FixedForm.Show() }
func (f *Form) Hide() { f.draftAllow = f.State.AllowOffline; f.FixedForm.Hide() }
func (f *Form) Reset() {
	f.Hide()
	f.State = State{Chances: previewChances, AllowOffline: true}
	f.Tab = PVP
	f.Page = 1
	f.draftAllow = true
	f.Refresh()
}

func (f *Form) Refresh() {
	f.Page = max(1, min(f.Page, f.pages()))
	for i, b := range f.Tabs {
		b.Sticky = i == f.Tab
	}
	f.Start.Visible, f.ViewRanking.Visible = f.Tab == PVP, f.Tab == PVP
	for _, b := range []*seui.FixedButton{f.First, f.Previous, f.Next, f.Last, f.MyRanking} {
		b.Visible = f.Tab == Rankings
	}
	f.First.Enabled, f.Previous.Enabled = f.Page > 1, f.Page > 1
	f.Next.Enabled, f.Last.Enabled = f.Page < f.pages(), f.Page < f.pages()
	f.MyRanking.Enabled = f.State.Rank > 0 && int(f.State.Rank) <= min(len(f.State.Entries), rankingLimit)
	f.Allow.Visible, f.Disallow.Visible = f.Tab == Settings, f.Tab == Settings
	dirty := f.draftAllow != f.State.AllowOffline
	f.Confirm.Visible, f.Cancel.Visible = f.Tab == Settings && dirty, f.Tab == Settings && dirty
	checked, unchecked := f.Env.Pics.Find("btn_Check_1"), f.Env.Pics.Find("btn_UnCheck_1")
	f.Allow.Image, f.Disallow.Image = unchecked, checked
	if f.draftAllow {
		f.Allow.Image, f.Disallow.Image = checked, unchecked
	}
}

func (f *Form) digits(picture string, value uint32, x, y int) {
	i := f.Env.Pics.Find(picture)
	w, h := f.Env.Pics.Size(i)
	h /= whiteDigitRows
	for n, d := range strconv.FormatUint(uint64(value), 10) {
		f.Env.Pics.DrawRect(f.Env.Screen, i, x+n*w, y, image.Rect(0, int(d-'0')*h, w, (int(d-'0')+1)*h), true)
	}
}

func (f *Form) Paint() {
	f.Refresh()
	f.FixedForm.Paint()
	at := f.Abs()
	panel := []string{"Form_PlayerBattleMain", "Form_PlayerBattleRank", "Form_PlayerBattleSetting"}[f.Tab]
	f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find(panel), at.X+21, at.Y+75, false)
	switch f.Tab {
	case PVP:
		f.digits("Num_White5_1", f.State.Rank, at.X+155, at.Y+95)
		f.digits("Num_White5_1", f.State.Wins, at.X+155, at.Y+137)
		f.digits("Num_White3_1", uint32(max(0, f.State.Chances)), at.X+120, at.Y+358)
		label := []byte(localization.UI("ui.worldpk.chest_clear_interval"))
		f.Env.Text.Draw(at.X+(Width-len(label)*8)/2, at.Y+336, 0, false, true, f.Env.Screen, label, 16, 300, 0, textInk, 0)
		f.Env.Pics.DrawRect(f.Env.Screen, f.Env.Pics.Find("Panel42"), at.X+142, at.Y+362, image.Rect(0, 0, 140*min(max(f.State.Chances, 0), previewChances)/previewChances, 4), true)
	case Rankings:
		f.digits("Num_White5_1", f.State.Rank, at.X+111, at.Y+429)
		for row := 0; row < rowsPerPage; row++ {
			index := (f.Page-1)*rowsPerPage + row
			if index >= min(len(f.State.Entries), rankingLimit) {
				break
			}
			e := f.State.Entries[index]
			y := at.Y + 82 + row*rankingRowStep
			f.digits("Num_White5_1", uint32(index+1), at.X+50, y+25)
			if e.Portrait != nil {
				f.Env.Screen.DrawStretch(image.Rect(at.X+96, y, at.X+168, y+72), e.Portrait, false)
			}
			f.Env.Text.Draw(at.X+173, y+7, 0, false, true, f.Env.Screen, e.Name, 16, 150, 0, textInk, 0)
			f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find("Main_Word_Lv"), at.X+173, y+27, true)
			f.digits("Num_White3_1", uint32(e.Level), at.X+202, y+27)
			f.digits("Num_White5_1", e.Wins, at.X+222, y+44)
			if e.Badge != "" {
				f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find(e.Badge), at.X+307, y+6, true)
			}
		}
		f.drawPage(at.X+165, at.Y+400)
	}
}

func (f *Form) drawPage(x, y int) {
	i := f.Env.Pics.Find("Num_MainInfo")
	w, h := f.Env.Pics.Size(i)
	h /= pageDigitRows
	for _, d := range strconv.Itoa(f.Page) + "/" + strconv.Itoa(f.pages()) {
		row := int(d - '0')
		if d == '/' {
			row = 10
		}
		f.Env.Pics.DrawRect(f.Env.Screen, i, x, y, image.Rect(0, row*h, w, (row+1)*h), true)
		x += w
	}
}
