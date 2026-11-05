package inventory

import (
	"fmt"
	"strings"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/game"
)

type destroyWarning struct {
	seui.Form
	owner      *Form
	text       string
	background *surface.Surface
	Confirm    *seui.FixedButton
}

func (f *Form) showDestroyWarning(item game.Item, count byte, confirm func()) {
	if f.destroyWarning != nil {
		f.destroyWarning.Hide()
	}
	name := f.State.Items[item.ID].Definition.Name
	if name == "" {
		name = fmt.Sprintf("Item %d", item.ID)
	}
	w := &destroyWarning{owner: f, text: fmt.Sprintf("Destroy %d × %s?\nThis item cannot be dropped.\nIt will be permanently destroyed.", count, name)}
	w.InitForm(w, f.Env)
	w.AnimateMessage(f.Now)
	w.Dockable = false
	w.Init("", (800-380)/2, 0, 0, 0, 0, false, 160, 380, 60)
	for i, name := range []string{"Btn_confirm_1", "Btn_Cancel_1"} {
		b := seui.NewFixedButton(w.Env, w)
		b.Init(name, 105+i*110, 20, 56, 0, 0, true, 20, 56, 118)
		if i == 0 {
			w.Confirm = b
			b.OnClick = func() { w.Hide(); confirm() }
		} else {
			b.OnClick = w.Hide
		}
	}
	f.destroyWarning = w
	f.Env.UI.Add(w)
	w.Show()
	f.Env.UI.SetModal(true, w)
}

func (w *destroyWarning) Paint() {
	if w.background == nil {
		w.background = seui.ConfirmationBackground(w.Env, w.Width, w.Height)
	}
	w.Env.Screen.DrawStretch(w.MessageRect(), w.background, true)
	for i, line := range strings.Split(w.text, "\n") {
		w.Env.Text.Draw(w.Left+max(24, (w.Width-len(line)*8)/2), w.Top+30+i*22, 0, false, true, w.Env.Screen, []byte(line), 20, w.Width-48, 0, 0xffe0, 0)
	}
}

func (w *destroyWarning) Hide() {
	if w.Env.UI.Modal == w {
		w.Env.UI.SetModal(false, w)
	}
	w.Form.Hide()
	w.Env.UI.Remove(w)
	if w.background != nil {
		w.background.Close()
		w.background = nil
	}
	w.owner.destroyWarning = nil
}

func (w *destroyWarning) KeyDown(key uint16, _ byte) {
	if key == 0x1b {
		w.Hide()
	}
}
