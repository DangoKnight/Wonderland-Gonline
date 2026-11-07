package notebook

import (
	"fmt"
	"image"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/localization"
)

type Form struct {
	seui.Form
	State          *State
	Quest, Monster *Book
	Star           func()
}
type Book struct {
	seui.Form
	State                  *State
	Monsters               bool
	Page, Selected, Scroll int
	Preview                func(uint16, int, int)
	row                    [RowsPerPage]*row
	Previous, Next         *seui.FixedButton
}
type row struct {
	seui.Component
	book  *Book
	index int
}

func (r *row) Paint() {}
func (r *row) LeftUp(_ byte, _, _ int) {
	r.book.Selected = r.book.Page*RowsPerPage + r.index
	r.book.Scroll = 0
}
func button(env *seui.Env, parent seui.Control, name string, x, y, w, h int, click func()) *seui.FixedButton {
	b := seui.NewFixedButton(env, parent)
	b.Init(name, x, h, w, 0, 0, true, h, w, y)
	b.OnClick = click
	return b
}
func New(env *seui.Env, state *State) *Form {
	f := &Form{State: state}
	f.InitForm(f, env)
	f.Name = "CH_NoteBook"
	f.Init("Form_NoteBook", 300, 0, 0, 0, 0, false, 220, 220, 150)
	f.Quest = newBook(env, state, false)
	f.Monster = newBook(env, state, true)
	button(env, f, "Btn_MissionBook", 63, 58, 96, 32, func() { f.Hide(); f.Quest.Show() })
	button(env, f, "Btn_MonsterBook", 63, 99, 96, 32, func() { f.Hide(); f.Monster.Show() })
	button(env, f, "Btn_StarBook", 63, 142, 96, 32, func() {
		f.Hide()
		if f.Star != nil {
			f.Star()
		}
	})
	button(env, f, "Btn_Close_1", 80, 190, 56, 20, f.Hide)
	button(env, f, "Btn_Close_s_1", 173, 26, 18, 18, f.Hide)
	env.UI.Add(f)
	return f
}
func (f *Form) Paint() { f.Env.Pics.Draw(f.Env.Screen, f.Image, f.Left, f.Top, true) }
func (f *Form) KeyDown(key uint16, _ byte) {
	if key == 0x1b {
		f.Hide()
	}
}
func (f *Form) CloseAll() { f.Hide(); f.Quest.Hide(); f.Monster.Hide() }
func newBook(env *seui.Env, state *State, monsters bool) *Book {
	b := &Book{State: state, Monsters: monsters, Selected: -1}
	b.InitForm(b, env)
	b.Name = "NotebookQuestLog"
	skin := "form_taskview_1"
	w, left, top := 432, 80, 60
	closeX, closeY := 188, 332
	titleX := 383
	if monsters {
		b.Name = "NotebookMonsterBook"
		skin = "Form_Cellection_1"
		w, left, top = 463, 150, 100
		closeX, titleX = 202, 413
	}
	b.Init(skin, left, 0, 0, 0, 0, false, 367, w, top)
	button(env, b, "Btn_Close_1", closeX, closeY, 56, 20, b.Hide)
	button(env, b, "Btn_Close_s_1", titleX, 24, 18, 18, b.Hide)
	previousSkin, nextSkin, previousX, nextX := "Btn_ArrowL_8", "Btn_ArrowR_8", 69, 144
	if !monsters {
		// Tjo_GetTask uses Arrow_L3/R3 at (74,298)/(137,298).
		// These must cover the arrows already painted into form_taskview_1.
		previousSkin, nextSkin, previousX, nextX = "Arrow_L3", "Arrow_R3", 74, 137
	}
	b.Previous = button(env, b, previousSkin, previousX, 298, 21, 21, func() {
		if b.Page > 0 {
			b.Page--
			b.Selected = b.Page * RowsPerPage
			b.Scroll = 0
		}
	})
	b.Next = button(env, b, nextSkin, nextX, 298, 21, 21, func() {
		if (b.Page+1)*RowsPerPage < b.count() {
			b.Page++
			b.Selected = b.Page * RowsPerPage
			b.Scroll = 0
		}
	})
	for i := range b.row {
		r := &row{book: b, index: i}
		r.InitComponent(r, env, b)
		r.Init("", 19, 0, 0, 0, 0, false, 22, 190, 54+i*24)
		b.row[i] = r
	}
	env.UI.Add(b)
	return b
}
func (b *Book) count() int {
	if b.Monsters {
		return len(b.State.MonsterList())
	}
	return len(b.State.Entries())
}
func (b *Book) KeyDown(key uint16, _ byte) {
	if key == 0x1b {
		b.Hide()
	}
}
func (b *Book) WheelUp() {
	if b.Env.UI.Input.X >= b.Left+218 && !b.Monsters {
		b.Scroll = max(0, b.Scroll-1)
	} else if b.Page > 0 {
		b.Page--
		b.Selected = b.Page * RowsPerPage
		b.Scroll = 0
	}
}
func (b *Book) WheelDown() {
	if b.Env.UI.Input.X >= b.Left+218 && !b.Monsters {
		b.Scroll++
	} else if (b.Page+1)*RowsPerPage < b.count() {
		b.Page++
		b.Selected = b.Page * RowsPerPage
		b.Scroll = 0
	}
}
func (b *Book) text(x, y, w int, s string, ink uint16) {
	if ink == 0 {
		ink = 0x0841
	} // Native near-black; RGB565 zero is the transparent color key.
	b.Env.Text.Draw(b.Left+x, b.Top+y, 0, false, true, b.Env.Screen, clientassets.Big5Text(s), 20, w, 0, ink, 0)
}
func (b *Book) Paint() {
	b.Env.Pics.Draw(b.Env.Screen, b.Image, b.Left, b.Top, true)
	if !b.Monsters {
		b.Env.Pics.DrawRect(b.Env.Screen, b.Image, b.Left+220, b.Top+292, image.Rect(220, 272, 410, 296), true)
	}
	entries := b.State.Entries()
	monsters := b.State.MonsterList()
	n := len(entries)
	if b.Monsters {
		n = len(monsters)
	}
	pages := max(1, (n+RowsPerPage-1)/RowsPerPage)
	b.Page = min(b.Page, pages-1)
	if b.Selected >= n {
		b.Selected = -1
	}
	b.Previous.Enabled = b.Page > 0
	b.Next.Enabled = b.Page+1 < pages
	for i, r := range b.row {
		idx := b.Page*RowsPerPage + i
		r.Visible = idx < n
		if !r.Visible {
			continue
		}
		name := ""
		done := false
		if b.Monsters {
			name = monsters[idx].Name
		} else {
			name, done = entries[idx].Name, entries[idx].Completed
		}
		if b.Selected == idx {
			b.Env.Screen.FillAlpha(image.Rect(b.Left+20, b.Top+55+i*24, b.Left+208, b.Top+75+i*24), 0xa8c6e8, 85)
		}
		x := 23
		if done {
			px, py := b.Left+23, b.Top+57+i*24
			for j := 0; j < 4; j++ {
				b.Env.Screen.Fill(image.Rect(px+j, py+4+j, px+j+2, py+6+j), 0xf800)
			}
			for j := 0; j < 7; j++ {
				b.Env.Screen.Fill(image.Rect(px+3+j, py+7-j, px+5+j, py+9-j), 0xf800)
			}
			x = 39
		}
		b.text(x, 56+i*24, 207-x, singleLine(name, (207-x)/8), 0)
	}
	b.text(106, 300, 40, fmt.Sprintf("%d/%d", b.Page+1, pages), 0x001f)
	if b.Selected < 0 || b.Selected >= n {
		return
	}
	if b.Monsters {
		m := monsters[b.Selected]
		b.text(229, 69, 170, m.Name, 0)
		b.text(374, 67, 25, fmt.Sprint(m.Level), 0)
		b.text(244, 91, 45, fmt.Sprint(m.HP), 0)
		b.text(319, 91, 60, fmt.Sprint(m.SP), 0)
		for i, v := range m.Stats {
			b.text(260, 110+20*i, 50, fmt.Sprint(v), 0)
		}
		if m.Element > 0 {
			b.Env.Pics.Draw(b.Env.Screen, b.Env.Pics.Find(fmt.Sprintf("Icon_Element_%d_1", m.Element)), b.Left+403, b.Top+65, true)
		}
		if b.Preview != nil {
			b.Preview(m.ID, b.Left+350, b.Top+220)
		}
	} else {
		e := entries[b.Selected]
		b.text(233, 56, 175, e.Name, 0x001f)
		lines := wrap(e.Description, 23)
		b.Scroll = min(b.Scroll, max(0, len(lines)-10))
		for i, line := range lines[b.Scroll:] {
			if i >= 10 {
				break
			}
			b.text(223, 87+i*20, 184, line, 0)
		}
		if len(lines) > 10 {
			b.text(253, 294, 150, localization.UI("ui.notebook.scroll"), 0x7bef)
		}
	}
}
func wrap(s string, limit int) []string {
	lines := []string{}
	line := []rune{}
	width := 0
	for _, r := range s {
		if r == '\r' {
			continue
		}
		advance := 1
		if r > 127 {
			advance = 2
		}
		if r == '\n' || width+advance > limit {
			lines = append(lines, string(line))
			line = nil
			width = 0
			if r == '\n' {
				continue
			}
		}
		line = append(line, r)
		width += advance
	}
	if len(line) > 0 {
		lines = append(lines, string(line))
	}
	return lines
}
func singleLine(s string, columns int) string {
	lines := wrap(s, columns)
	if len(lines) == 0 {
		return ""
	}
	if len(lines) == 1 {
		return lines[0]
	}
	r := []rune(lines[0])
	if len(r) > 2 {
		return string(r[:len(r)-2]) + ".."
	}
	return lines[0]
}
