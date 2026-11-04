package inventory

import (
	"strings"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
)

// FUN_001cc7b8 lists learned skills endangered by a normal pill failure.
type potentialWarning struct {
	seui.Form
	owner           *useDialog
	text            string
	background      *surface.Surface
	Confirm, Cancel *seui.FixedButton
}

func (d *useDialog) showSkillWarning(names []string) {
	w := &potentialWarning{owner: d, text: "Failure will forget skill:\n" + strings.Join(names, "\n")}
	w.InitForm(w, d.Env)
	w.AnimateMessage(d.owner.Now)
	w.Dockable = false
	height := max(144, 94+len(names)*18)
	w.Init("", (800-324)/2, 0, 0, 0, 0, false, height, 324, 30)
	for i, name := range []string{"Btn_confirm_1", "Btn_Cancel_1"} {
		b := seui.NewFixedButton(w.Env, w)
		b.Init(name, 86+i*96, 20, 56, 0, 0, true, 20, 56, height-42)
		if i == 0 {
			w.Confirm = b
			b.OnClick = func() { w.Hide(); d.submit() }
		} else {
			w.Cancel = b
			b.OnClick = w.Hide
			b.SetHint([]byte("[ESC]"))
		}
	}
	d.warning = w
	d.Env.UI.Add(w)
	w.Show()
	d.Env.UI.SetModal(true, w)
}

func (w *potentialWarning) Paint() {
	if w.background == nil {
		w.background = seui.ConfirmationBackground(w.Env, w.Width, w.Height)
	}
	w.Env.Screen.DrawStretch(w.MessageRect(), w.background, true)
	for i, line := range strings.Split(w.text, "\n") {
		x := w.Left + max(40, (w.Width-len(line)*8)/2)
		w.Env.Text.Draw(x, w.Top+30+i*18, 0, false, true, w.Env.Screen, []byte(line), 18, w.Width-80, 0, 0xffe0, 0)
	}
}

func (w *potentialWarning) Hide() {
	w.Form.Hide()
	w.Env.UI.Remove(w)
	if w.background != nil {
		w.background.Close()
		w.background = nil
	}
	w.owner.warning = nil
	if w.Env.UI.Modal == w {
		w.Env.UI.SetModal(false, w)
		if w.owner.Visible {
			w.Env.UI.SetModal(true, w.owner)
		}
	}
}

func (w *potentialWarning) KeyDown(key uint16, _ byte) {
	if key == 0x1b {
		w.Hide()
	}
}
