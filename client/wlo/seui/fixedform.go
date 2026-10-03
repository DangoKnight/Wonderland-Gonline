package seui

// FixedForm is TSe_FixedForm (constructor FUN_00474ee4): a component that
// the manager treats as a form. It has no frame of its own; derived forms
// paint their own background.
type FixedForm struct {
	Component
	DragX, DragY int  // +0xd0, +0xd4
	Draggable    bool // +0xd8
	TitleIcon    int  // +0xdc
	KeepOnScreen bool // +0xe0
}

// InitFixedForm initialises an embedded FixedForm; derived forms call it
// from their constructors.
func (f *FixedForm) InitFixedForm(self Control, env *Env) {
	f.initComponent(self, env, nil)
	f.Name = "TSe_FixedForm"
	f.Visible = false
	f.Draggable = true
	f.Cursor, f.Group, f.DownSound = 0, 0xff, 0xff
	f.TitleIcon = -1
	f.KeepOnScreen = true
}

func (f *FixedForm) isForm() {}

// Show is slot +0x20 (FUN_00474ec8).
func (f *FixedForm) Show() {
	f.Component.Show()
	f.Env.UI.Front(f.self, true)
}

// Hide is slot +0x24 (FUN_00474f78).
func (f *FixedForm) Hide() {
	f.GrBasic.Hide()
	f.Env.UI.Front(f.self, false)
}

// Activate is slot +0x6c (FUN_0047509c).
func (f *FixedForm) Activate(on bool) { f.Env.UI.Front(f.self, on) }

// LeftDown is slot +0x38 (FUN_00474f94). Only a press on the form itself
// starts a drag; the creation forms have no size, so they never do.
func (f *FixedForm) LeftDown(shift byte, x, y int) {
	f.Component.LeftDown(shift, x, y)
	if f.Draggable && !f.Blocked() {
		f.DragX, f.DragY = x-f.Left, y-f.Top
		f.Dragging = true
		f.self.Activate(true)
	}
}

// LeftUp is slot +0x30 (FUN_00474ff0).
func (f *FixedForm) LeftUp(shift byte, x, y int) {
	f.Component.LeftUp(shift, x, y)
	f.Dragging = false
	f.DragX, f.DragY = 0, 0
}

// MouseMove is slot +0x48 (FUN_00475024).
func (f *FixedForm) MouseMove(_ byte, x, y int) {
	if !f.Dragging {
		return
	}
	f.Left, f.Top = x-f.DragX, y-f.DragY
	if f.KeepOnScreen {
		f.keep()
	}
}

// keep holds a dragged form on screen (FUN_00475024, FUN_004750ac).
func (f *FixedForm) keep() {
	if f.Left < 10 {
		f.Left = 0
	}
	if f.Right() > 0x316 {
		f.Left = 800 - f.Width
	}
	if f.Top < 10 {
		f.Top = 0
	}
	if f.Bottom() > 0x226 {
		f.Top = 0x230 - f.Height
	}
}

// Update is slot +0x18 (FUN_004750ac): a component update, then the title
// icon (FUN_004751b0).
func (f *FixedForm) Update(in *Input) {
	if !f.Visible {
		return
	}
	if f.Dragging && f.KeepOnScreen {
		f.keep()
	}
	f.Component.Update(in)
	if f.TitleIcon != -1 {
		f.Env.Pics.Draw(f.Env.Screen, f.TitleIcon, f.Left+0xf, f.Top+7, true)
	}
	if f.self != in.Hovered {
		f.Dragging = false
		f.DragX, f.DragY = 0, 0
	}
}

// Hint is slot +0x1c (FUN_00475140): only over the top 24 pixels.
func (f *FixedForm) Hint() {
	in := f.Env.UI.Input
	a := f.Abs()
	if in.X >= a.X && in.X < a.X+f.Width && in.Y >= a.Y && in.Y < a.Y+0x18 {
		f.Component.Hint()
	}
}
