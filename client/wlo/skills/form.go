package skills

import (
	"fmt"
	"image"
	"sort"
	"strconv"
	"strings"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
)

const (
	Physical          = 1
	Magical           = 2
	Assistant         = 3
	Life              = 4
	Intro             = 5
	formWidth         = 290
	formHeight        = 459
	listRows          = 8
	rowHeight         = 26
	textInk           = uint16(0x0841)
	detailInk         = uint16(0x001f)
	gradeInk          = uint16(0xffe0)
	treeColumns       = 3
	treeRows          = 6
	treeFirstY        = 129
	treeStepY         = 34
	treeIconSize      = 28
	nativeRangeRanged = 3
)

// These native category exceptions are in FUN_002bba90, independently of
// combat behavior: Zavrl's Wing, Collapse, Curdle, Fervidity, Fast, Fatal Blow.
var assistantOverrides = map[uint16]bool{17019: true, 15222: true, 15225: true, 25067: true, 15166: true, 25147: true}

type Target struct {
	Slot    byte
	Name    []byte
	Element byte
	Skills  map[uint16]Progress
	Stunt   uint16
}
type Form struct {
	seui.Form
	Catalog               *Catalog
	Targets               func() []Target
	Notice                func(string)
	LoadPictures          func(...string)
	Use                   func(uint16, byte)
	Tabs                  [Intro]*seui.FixedButton
	Previous, Next, Close *seui.FixedButton
	List                  *seui.SelectText
	Scroll                *seui.ScrollButton
	Up, Down              *seui.FixedButton
	Tab                   int
	Selected              byte
	rows                  []uint16
	positions             [Intro]int
}

func button(env *seui.Env, owner seui.Control, asset string, x, y, w, h int, click func()) *seui.FixedButton {
	b := seui.NewFixedButton(env, owner)
	b.Init(asset, x, h, w, 0, 0, true, h, w, y)
	b.OnClick = click
	return b
}

// NewForm ports FUN_002a00f0 and FUN_002a2234; native lists are separate
// from the battle command menu. Opening or browsing never sends a command.
func NewForm(env *seui.Env, c *Catalog) *Form {
	f := &Form{Catalog: c, Tab: Physical}
	f.InitForm(f, env)
	f.Name = "TRe_SkillForm"
	f.Init("Form_Skill_2", (800-formWidth)/2, 0, 0, 0, 0, false, formHeight, formWidth, (600-formHeight)/2)
	f.Dockable = false
	f.SetHint([]byte("Skills [Ctrl+S]"))
	button(env, f, "Btn_Close_s_1", 240, 24, 18, 18, f.Hide)
	f.Close = button(env, f, "btn_Close_1", 116, 416, 56, 20, f.Hide)
	f.Previous = button(env, f, "Btn_ArrowL_5", 73, 38, 17, 17, func() { f.turn(-1) })
	f.Next = button(env, f, "Btn_ArrowR_5", 191, 38, 17, 17, func() { f.turn(1) })
	f.Previous.SetHint([]byte("Previous"))
	f.Next.SetHint([]byte("Next"))
	for i := range f.Tabs {
		tab := i + 1
		f.Tabs[i] = button(env, f, "Btn_SkillIcon_"+strconv.Itoa(tab), 23+i*38, 59, 38, 21, func() { f.SetTab(tab) })
	}
	l := seui.NewSelectText(env, f)
	l.RowHeight, l.LineHeight, l.TextX, l.TextY = rowHeight, 15, 28, 7
	l.Embossed, l.FocusBox, l.ShowSel = false, false, true
	l.Color = textInk
	l.Highlight = 0xf5bf89
	l.SetBounds(28, 88, listRows*rowHeight, 216)
	f.List = l
	s := seui.NewScrollButton(env, f)
	s.Init("Rail_H5", 246, 182, 17, 0, 0, true, 203, 17, 104)
	s.SetThumb("bar_H4", 0, 21, 9, 0)
	s.SetCaps(7, 16)
	s.SetRange(listRows, listRows)
	l.Scroll = s
	s.OnChange = func() { l.TopIndex = s.Pos }
	f.Scroll = s
	f.Up = button(env, f, "Btn_ArrowUp_1", 246, 88, 17, 17, l.ScrollUp)
	f.Down = button(env, f, "Btn_ArrowDn_1", 246, 306, 17, 17, l.ScrollDown)
	l.OnSelect = func(i int) { l.Selected = i }
	l.OnDblClick = func(i int) {
		if f.Use != nil && i >= 0 && i < len(f.rows) {
			f.Use(f.rows[i], f.Selected)
			return
		}
		if f.Notice != nil {
			f.Notice("Skill targeting is not implemented in the Go client yet.")
		}
	}
	f.SetTab(Physical)
	return f
}
func (f *Form) target() (Target, bool) {
	if f.Targets == nil {
		return Target{}, false
	}
	for _, t := range f.Targets() {
		if t.Slot == f.Selected {
			return t, true
		}
	}
	return Target{}, false
}
func (f *Form) turn(by int) {
	if f.Targets == nil {
		return
	}
	targets := f.Targets()
	if len(targets) == 0 {
		return
	}
	index := 0
	for i, t := range targets {
		if t.Slot == f.Selected {
			index = i
			break
		}
	}
	index = (index + by + len(targets)) % len(targets)
	f.Selected = targets[index].Slot
	f.positions = [Intro]int{}
	f.Refresh()
}
func (f *Form) Reset() { f.Hide(); f.Selected = 0; f.positions = [Intro]int{}; f.SetTab(Physical) }
func (f *Form) Show()  { f.Refresh(); f.Form.Show() }
func (f *Form) SetTab(tab int) {
	if tab < Physical || tab > Intro {
		return
	}
	if f.Tab >= Physical && f.Tab <= Intro {
		f.positions[f.Tab-1] = f.List.TopIndex
	}
	f.List.Clear()
	f.List.HoverRow = -1
	f.List.TopIndex = f.positions[tab-1]
	f.Tab = tab
	for i, b := range f.Tabs {
		b.Sticky = i+1 == tab
		if b.Sticky {
			b.State = 2
		} else {
			b.State = 0
		}
	}
	for _, control := range []seui.Control{f.List, f.Scroll, f.Up, f.Down} {
		control.Base().SetVisible(tab != Intro)
	}
	f.Refresh()
}
func category(d Definition, tab int) bool {
	special := assistantOverrides[d.ID]
	switch tab {
	case Physical, Magical:
		return int(d.EffectLayer) == tab && !special
	case Life:
		return d.EffectLayer == LifeLayer
	case Assistant:
		layer := d.EffectLayer
		return special || layer >= 3 && layer <= 9 || layer >= 14 && layer <= 16 || layer >= 18 && layer <= 21
	}
	return false
}
func (f *Form) definition(id uint16, t Target) Definition {
	if id == game.StarterStuntClientID && t.Stunt != 0 {
		if d, ok := f.Catalog.Definitions[t.Stunt]; ok {
			return d
		}
	}
	d := f.Catalog.Definitions[id]
	if id == BasicAttack {
		d.Name = "Basic Atk"
	} else if id == Defense {
		d.Name = "Defense"
	}
	return d
}
func (f *Form) Refresh() {
	t, ok := f.target()
	if !ok && f.Targets != nil {
		targets := f.Targets()
		if len(targets) > 0 {
			f.Selected = targets[0].Slot
			t, ok = targets[0], true
		}
	}
	oldID := uint16(0)
	if f.List.Selected >= 0 && f.List.Selected < len(f.rows) {
		oldID = f.rows[f.List.Selected]
	}
	top := f.positions[f.Tab-1]
	if f.List.Visible {
		top = f.List.TopIndex
	}
	f.List.Clear()
	f.List.HoverRow = -1
	f.rows = nil
	if !ok || f.Tab == Intro {
		return
	}
	if f.Tab == Physical {
		f.rows = append(f.rows, BasicAttack, Defense)
	}
	var ids []uint16
	for id, p := range t.Skills {
		if p.Grade != 0 && id != BasicAttack && id != Defense && category(f.definition(id, t), f.Tab) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := f.Catalog.Definitions[ids[i]], f.Catalog.Definitions[ids[j]]
		if a.TableOrder == b.TableOrder {
			return a.ID < b.ID
		}
		return a.TableOrder < b.TableOrder
	})
	f.rows = append(f.rows, ids...)
	for i, id := range f.rows {
		d := f.definition(id, t)
		if f.LoadPictures != nil {
			f.LoadPictures(d.IconName())
		}
		f.List.Add(clientassets.Big5Text(d.Name), d.IconName(), strconv.Itoa(int(id)))
		if id == oldID {
			f.List.Selected = i
		}
	}
	f.List.SetTop(min(max(0, top), max(0, len(f.rows)-listRows)))
	f.Scroll.SetPos(f.List.TopIndex)
}
func (f *Form) drawText(x, y, w int, text string, color uint16) {
	a := f.Abs()
	f.Env.Text.Draw(a.X+x, a.Y+y, 0, false, true, f.Env.Screen, clientassets.Big5Text(text), 15, w, 0, color, 0)
}
func (f *Form) Paint() {
	f.Form.Paint()
	if f.Tab != Intro {
		return
	}
	t, ok := f.target()
	if !ok {
		return
	}
	a := f.Abs()
	f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find("Icon_SkillNeed"), a.X+24, a.Y+374, true)
	if t.Slot != 0 {
		return
	}
	background := "form_skillexpress" + strconv.Itoa(int(t.Element))
	if f.LoadPictures != nil {
		f.LoadPictures(background)
	}
	f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find(background), a.X+69, a.Y+87, true)
	for i, e := range game.ElementSkillTree(t.Element) {
		if i >= treeColumns*treeRows {
			break
		}
		if t.Skills[e.ID].Grade != 0 {
			p := treePoint(i)
			if f.LoadPictures != nil {
				f.LoadPictures(f.Catalog.Definitions[e.ID].IconName())
			}
			f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find(f.Catalog.Definitions[e.ID].IconName()), a.X+p.X, a.Y+p.Y, true)
		}
	}
}
func treePoint(i int) image.Point {
	return image.Pt([treeColumns]int{69, 128, 186}[i/treeRows], treeFirstY+i%treeRows*treeStepY)
}
func (f *Form) Update(in *seui.Input) {
	if !f.Visible {
		return
	}
	if _, ok := f.target(); !ok {
		f.Refresh()
	}
	f.Form.Update(in)
	t, ok := f.target()
	if !ok {
		return
	}
	a := f.Abs()
	nameX := (formWidth-len(t.Name)*8)/2 - 4
	f.Env.Text.Draw(a.X+nameX, a.Y+40, 0, false, true, f.Env.Screen, t.Name, 15, 230, 0, detailInk, 0)
	if f.Tab == Intro {
		if t.Slot != 0 || f.Blocked() {
			return
		}
		for i, e := range game.ElementSkillTree(t.Element) {
			if i >= treeColumns*treeRows {
				break
			}
			p := a.Add(treePoint(i))
			if image.Pt(in.X, in.Y).In(image.Rect(p.X, p.Y, p.X+treeIconSize, p.Y+treeIconSize)) {
				d := f.Catalog.Definitions[e.ID]
				f.drawText(27, 339, 230, d.Name, detailInk)
				f.drawText(27, 359, 230, d.Description, detailInk)
				f.drawText(100, 395, 160, requirements(e.Minimum), detailInk)
				break
			}
		}
		return
	}
	for row := 0; row < listRows; row++ {
		i := f.List.TopIndex + row
		if i >= len(f.rows) {
			break
		}
		id := f.rows[i]
		if id == BasicAttack || id == Defense {
			f.drawText(28, 88+row*rowHeight+13, 24, strconv.Itoa(game.MinSkillGrade), gradeInk)
			continue
		}
		d := f.definition(id, t)
		p := t.Skills[id]
		f.drawText(28, 88+row*rowHeight+13, 24, strconv.Itoa(int(p.Grade)), gradeInk)
		if d.SP != 0 {
			f.drawText(190, 88+row*rowHeight+7, 52, fmt.Sprintf("%d sp", d.SP), textInk)
		}
	}
	i := f.List.IndexAt()
	if i < 0 {
		i = f.List.Selected
	}
	if i < 0 || i >= len(f.rows) {
		return
	}
	id := f.rows[i]
	d := f.definition(id, t)
	if id == BasicAttack || id == Defense {
		f.drawText(31, 339, 230, d.Name, detailInk)
		return
	}
	f.drawText(31, 339, 230, d.Description, detailInk)
	p := t.Skills[id]
	f.drawText(68, 375, 30, strconv.Itoa(int(p.Grade)), detailInk)
	attack := "Assist"
	if d.EffectLayer == Physical || d.EffectLayer == Magical {
		attack = "Nearby"
		if d.Attack == nativeRangeRanged {
			attack = "Ranged"
		}
	} else if d.EffectLayer == LifeLayer {
		attack = "Life Skill"
	}
	f.drawText(175, 376, 100, attack, detailInk)
	proficiency := fmt.Sprintf("%.2f%%", float64(p.Proficiency)/100)
	if d.MaximumGrade != 0 && p.Grade >= d.MaximumGrade {
		proficiency = "Max"
	}
	f.drawText(80, 395, 78, proficiency, detailInk)
	for i, allowed := range d.Weapons {
		if allowed {
			f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find(fmt.Sprintf("Icon_Weapon%d_1", i+1)), a.X+174+i*18, a.Y+395, true)
		}
	}
}
func requirements(a game.Attributes) string {
	var parts []string
	for _, r := range []struct {
		name  string
		value uint16
	}{{"STR", a.Strength}, {"CON", a.Constitution}, {"INT", a.Intelligence}, {"WIS", a.Wisdom}, {"AGI", a.Agility}} {
		if r.value != 0 {
			parts = append(parts, fmt.Sprintf("%s %d", r.name, r.value))
		}
	}
	if len(parts) == 0 {
		return "None"
	}
	return strings.Join(parts, ", ")
}

// SkillAt excludes the Intro tree: unlearned reference nodes cannot be bound.
func (f *Form) SkillAt(x, y int) (uint16, byte) {
	if !f.Visible || f.Tab == Intro || f.Blocked() {
		return 0, 0
	}
	a := f.List.Abs()
	if !image.Pt(x, y).In(image.Rect(a.X, a.Y, a.X+f.List.Width, a.Y+f.List.Height)) {
		return 0, 0
	}
	row := f.List.TopIndex + (y-a.Y)/rowHeight
	if row >= len(f.rows) {
		return 0, 0
	}
	return f.rows[row], f.Selected
}
