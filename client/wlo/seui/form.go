package seui

// Form is TSe_Form (constructor FUN_004687e8): a draggable panel kept on
// screen, registered with the manager.
type Form struct {
	Panel
	DragX, DragY int  // +0x118, +0x11c pointer offset while dragging
	Draggable    bool // +0x120
	Docked       bool // +0x128
	Dockable     bool // +0x129 right-click docks the form at the bottom
	TitleIcon    int  // +0x12c
}

func NewForm(env *Env) *Form {
	f := &Form{}
	f.InitForm(f, env)
	return f
}

// InitForm initialises an embedded Form; derived forms call it from their
// constructors.
func (f *Form) InitForm(self Control, env *Env) {
	f.initPanel(self, env, nil)
	f.Name = "Form"
	f.Visible = false
	f.Draggable, f.Hover, f.Docked, f.Dockable = true, false, false, true
	f.Cursor, f.Group, f.DownSound = 0, 0xff, 0xff
	f.TitleIcon = -1
}

func (f *Form) isForm() {}

// Show is slot +0x20 (FUN_004687cc): visible and brought to the front.
func (f *Form) Show() {
	f.Visible = true
	f.Env.UI.Front(f.self, true)
}

// Hide is slot +0x24 (FUN_0046887c): hidden, and the form behind takes the
// front if this one had it.
func (f *Form) Hide() {
	f.Visible = false
	f.Env.UI.Front(f.self, false)
}

// Activate is slot +0x6c (FUN_00468a48).
func (f *Form) Activate(on bool) { f.Env.UI.Front(f.self, on) }

// LeftDown is slot +0x38 (FUN_00468898): a draggable form starts a drag.
func (f *Form) LeftDown(shift byte, x, y int) {
	if f.Draggable && !f.Blocked() {
		f.DragX, f.DragY = x-f.Left, y-f.Top
		f.Dragging = true
		f.self.Activate(true)
	}
	f.Component.LeftDown(shift, x, y)
}

// LeftUp is slot +0x30 (FUN_004688f8).
func (f *Form) LeftUp(shift byte, x, y int) {
	f.Component.LeftUp(shift, x, y)
	f.Dragging = false
	if f.Dockable && f.Top < 0x235-f.MarginT {
		f.Docked = false
	}
	f.DragX, f.DragY = 0, 0
}

// MouseMove is slot +0x48 (FUN_0046894c): dragging, within the screen.
func (f *Form) MouseMove(_ byte, x, y int) {
	if !f.Dragging {
		return
	}
	f.Left, f.Top = x-f.DragX, y-f.DragY
	if !f.Docked {
		if f.Top < 10 {
			f.Top = 0
		}
		if f.Bottom() > 0x21c {
			f.Top = 0x226 - f.Height
		}
		return
	}
	if lim := 0x226 - f.MarginT; lim < f.Top {
		f.Top = lim
	}
	if f.Left < 10 {
		f.Left = 0
	}
	if f.Right() > 0x316 {
		f.Left = 800 - f.Width
	}
}

// RightDown is slot +0x44 (FUN_004689d8).
func (f *Form) RightDown(shift byte, x, y int) { f.Component.RightDown(shift, x, y) }

// RightUp is slot +0x3c (FUN_004689f0): a dockable form toggles docking.
func (f *Form) RightUp(shift byte, x, y int) {
	f.Component.RightUp(shift, x, y)
	if !f.Dockable {
		return
	}
	f.Docked = !f.Docked
	if lim := 0x235 - f.MarginT; f.Top < lim {
		f.Top = lim
	} else {
		f.Top = 0x235 - f.Height
	}
}

// Update is slot +0x18 (FUN_00468a58): the form is kept on screen, drawn as
// a panel, then its title icon.
func (f *Form) Update(in *Input) {
	if !f.Visible {
		return
	}
	if f.Left < 10 {
		f.Left = 0
	}
	if f.Right() > 0x316 && f.Left > 0 {
		f.Left = 800 - f.Width
	}
	if f.Bottom() > 0x235 {
		f.Top = 0x23f - f.Height
	}
	if f.Top < 10 && f.Top != 0x23f-f.Height {
		f.Top = 0
	}
	if f.Dragging && !f.Docked {
		if f.Left < 5 {
			f.Left = 0
		}
		if f.Right() > 0x31b {
			f.Left = 800 - f.Width
		}
		if f.Top < 10 {
			f.Top = 0
		}
		if f.Bottom() > 0x235 {
			f.Top = 0x23f - f.Height
		}
	}
	f.Panel.Update(in)
	f.paintTitleIcon()
	if f.self != in.Hovered {
		f.Dragging = false
		f.DragX, f.DragY = 0, 0
	}
}

// paintTitleIcon is FUN_00468bb8.
func (f *Form) paintTitleIcon() {
	if f.TitleIcon != -1 {
		f.Env.Pics.Draw(f.Env.Screen, f.TitleIcon, f.Left+0xf, f.Top+7, true)
	}
}

// Hint is slot +0x1c (FUN_00468b48): only over the top 24 pixels.
func (f *Form) Hint() {
	in := f.Env.UI.Input
	a := f.Abs()
	if in.X >= a.X && in.X < a.X+f.Width && in.Y >= a.Y && in.Y < a.Y+0x18 {
		f.Component.Hint()
	}
}
