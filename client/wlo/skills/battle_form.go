package skills

import "wonderland-gonline/internal/localization"

import (
	"fmt"
	"sort"
	"strconv"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/clientassets"
)

const (
	battleFormWidth            = 274
	battleFormHeight           = 353
	battleRecentRows           = 3
	battleListRows             = 5
	battleRowHeight            = 25
	nativeBattleSkillFirstType = 1
	nativeBattleSkillLastType  = 7
)

// BattleForm is TRe_BattleSkillForm: FUN_0029b674 constructs the two lists;
// FUN_0029bad4 populates learned skills and FUN_0029b3ac maintains recent casts.
type BattleForm struct {
	seui.Form
	Browser          *Form
	List, Recent     *seui.SelectText
	Up, Down, Close  *seui.FixedButton
	Use              func(uint16, byte)
	Selected         byte
	rows, recentRows []uint16
	history          map[byte][]uint16
}

func NewBattleForm(env *seui.Env, browser *Form) *BattleForm {
	f := &BattleForm{Browser: browser, history: map[byte][]uint16{}}
	if browser.LoadPictures != nil {
		browser.LoadPictures("Form_BattleSkill_1", "Btn_ArrowDn_1", "Btn_Close_1")
	}
	f.InitForm(f, env)
	f.Name = "TRe_BattleSkillForm"
	f.Init("Form_BattleSkill_1", (800-battleFormWidth)/2, 0, 0, 0, 0, false, battleFormHeight, battleFormWidth, (600-battleFormHeight)/2)
	f.Dockable = false
	list := func(y, height int) *seui.SelectText {
		l := seui.NewSelectText(env, f)
		l.RowHeight, l.LineHeight, l.TextX, l.TextY = battleRowHeight, 15, 30, 3
		l.Embossed, l.FocusBox, l.ShowSel = false, false, false
		l.Color = textInk
		l.SetBounds(17, y, height, 242)
		return l
	}
	f.Recent = list(42, 81)
	f.Recent.Spacing = 3
	f.Recent.SetBounds(17, 42, 81, 242)
	f.List = list(151, 137)
	f.Recent.OnSelect = func(i int) { f.activate(f.recentRows, i) }
	f.List.OnSelect = func(i int) { f.activate(f.rows, i) }
	f.Up = button(env, f, "Btn_ArrowUp_1", 130, 127, 17, 17, f.List.ScrollUp)
	f.Down = button(env, f, "Btn_ArrowDn_1", 130, 292, 17, 17, f.List.ScrollDown)
	f.Close = button(env, f, "Btn_Close_1", 111, 318, 56, 20, f.Hide)
	return f
}

func (f *BattleForm) target() (Target, bool) {
	if f.Browser.Targets != nil {
		for _, t := range f.Browser.Targets() {
			if t.Slot == f.Selected {
				return t, true
			}
		}
	}
	return Target{}, false
}

func (f *BattleForm) activate(rows []uint16, i int) {
	if !f.Visible || i < 0 || i >= len(rows) || f.Use == nil {
		return
	}
	id := rows[i]
	recent := []uint16{id}
	for _, old := range f.history[f.Selected] {
		if old != id && len(recent) < battleRecentRows {
			recent = append(recent, old)
		}
	}
	f.history[f.Selected] = recent
	f.Use(id, f.Selected)
}

func (f *BattleForm) Show()  { f.Refresh(); f.Form.Show() }
func (f *BattleForm) Reset() { f.Hide(); f.history = map[byte][]uint16{}; f.Selected = 0 }
func (f *BattleForm) Refresh() {
	top := f.List.TopIndex
	f.List.Clear()
	f.Recent.Clear()
	f.rows, f.recentRows = nil, nil
	t, ok := f.target()
	if !ok {
		return
	}
	for id, progress := range t.Skills {
		// FUN_0029bad4 checks the learned table entry before resolving its
		// character-specific presentation. Stunt alias 15003 owns the order;
		// the actual character skill (for example Love Wish) has order zero.
		d := f.Browser.Catalog.Definitions[id]
		// FUN_004816d8 checks native type 1..7 and a nonzero table order.
		if progress.Grade != 0 && d.Type >= nativeBattleSkillFirstType && d.Type <= nativeBattleSkillLastType && d.TableOrder != 0 && id != BasicAttack && id != Defense {
			f.rows = append(f.rows, id)
		}
	}
	sort.Slice(f.rows, func(i, j int) bool {
		a, b := f.Browser.Catalog.Definitions[f.rows[i]], f.Browser.Catalog.Definitions[f.rows[j]]
		if a.TableOrder == b.TableOrder {
			return a.ID < b.ID
		}
		return a.TableOrder < b.TableOrder
	})
	add := func(l *seui.SelectText, id uint16) {
		d := f.Browser.definition(id, t)
		if f.Browser.LoadPictures != nil {
			f.Browser.LoadPictures(d.IconName())
		}
		l.Add(clientassets.Big5Text(d.Name), d.IconName(), strconv.Itoa(int(id)))
	}
	available := map[uint16]bool{}
	for _, id := range f.rows {
		available[id] = true
		add(f.List, id)
	}
	for _, id := range f.history[f.Selected] {
		if available[id] {
			f.recentRows = append(f.recentRows, id)
			add(f.Recent, id)
		}
	}
	f.List.SetTop(min(top, max(0, len(f.rows)-battleListRows)))
}

func (f *BattleForm) Update(in *seui.Input) {
	f.Form.Update(in)
	if !f.Visible {
		return
	}
	t, ok := f.target()
	if !ok {
		f.Hide()
		return
	}
	for _, part := range []struct {
		list *seui.SelectText
		rows []uint16
	}{{f.Recent, f.recentRows}, {f.List, f.rows}} {
		for row := 0; row < part.list.RowsShown; row++ {
			i := part.list.TopIndex + row
			if i >= len(part.rows) {
				break
			}
			d := f.Browser.definition(part.rows[i], t)
			at := part.list.Abs()
			f.Env.Text.Draw(at.X+199, at.Y+row*(part.list.RowHeight+part.list.Spacing)+3, 0, false, true, f.Env.Screen, []byte(fmt.Sprintf(localization.UI("ui.skills.d_sp_e5072ea2"), d.SP)), 15, 43, 0, textInk, 0)
		}
	}
}
