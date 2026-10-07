package seui

// Manager is the form manager (PTR_DAT_004ca108, constructor 0x466f00).
type Manager struct {
	Env     *Env
	Forms   []Control // +0x04 draw order; the last is in front
	Others  []Control // +0x08
	Modal   Control   // +0x10
	Active  Control   // +0x14
	OnlyTop bool      // +0x18 draw only controls marked OnTop
	Input   *Input    // +0x1c
	Color   uint16    // +0x26 skin colour applied to every form
}

func NewManager(env *Env, in *Input) *Manager {
	m := &Manager{Env: env, Input: in}
	env.UI = m
	return m
}

// Add is FUN_00466f64: register a form, give it the manager and the skin
// colour.
func (m *Manager) Add(f Control) {
	m.Forms = append(m.Forms, f)
	m.Others = append(m.Others, f)
	SetEnv(f, m.Env)
	f.SetColor(m.Color)
}

// Remove is FUN_00466f98.
func (m *Manager) Remove(f Control) {
	in := m.Input
	for _, p := range []*Control{&in.Hovered, &in.Focused, &in.Pressed, &in.Captured} {
		if *p == f {
			*p = nil
		}
	}
	m.Forms = without(m.Forms, f)
	m.Others = without(m.Others, f)
}

func without(list []Control, f Control) []Control {
	for i, k := range list {
		if k == f {
			return append(list[:i:i], list[i+1:]...)
		}
	}
	return list
}

// SetColor is FUN_0046762c: the skin colour for every form.
func (m *Manager) SetColor(c uint16) {
	m.Color = c
	for _, f := range m.Forms {
		f.SetColor(c)
	}
}

// Front is FUN_00467058. With front set the form becomes active and moves
// to the end of the draw list; every other form and fixed form is told it
// is inactive. Otherwise, if the form is the frontmost, the next visible
// form behind it is brought forward.
func (m *Manager) Front(f Control, front bool) {
	if !front {
		if n := len(m.Forms); n > 0 && m.Forms[n-1] == f {
			for i := n - 2; i >= 0; i-- {
				if m.Forms[i].Base().Visible {
					m.Front(m.Forms[i], true)
					return
				}
			}
		}
		return
	}
	m.Active = f
	for i, k := range m.Forms {
		if k == f {
			m.Forms = append(append(m.Forms[:i:i], m.Forms[i+1:]...), f)
			break
		}
	}
	for _, k := range m.Forms[:len(m.Forms)-1] {
		if _, ok := k.(FormLike); ok {
			k.Activate(false)
		}
	}
}

// FormLike marks classes derived from TSe_Form or TSe_FixedForm.
type FormLike interface{ isForm() }

// SetModal is FUN_00467518.
func (m *Manager) SetModal(on bool, f Control) {
	if on {
		m.Front(f, true)
		m.Modal = f
		return
	}
	m.Modal = nil
}

// Tick is FUN_0046753c: every form's slot +0x14.
func (m *Manager) Tick() {
	for _, f := range m.Forms {
		f.Tick()
	}
}

// HideAll is FUN_004675ec.
func (m *Manager) HideAll() {
	for _, f := range m.Forms {
		f.Base().Visible = false
	}
}

// stayOnTop reports panels drawn in the second pass (+0x108).
type stayOnTop interface{ onTopLayer() bool }

// Draw is FUN_00467148: every form's update in draw order, stay-on-top
// panels after the others, then the hovered control's hint. The cursor
// image selection that follows in the original is not ported.
func (m *Manager) Draw() {
	in := m.Input
	for _, f := range m.Forms {
		if t, ok := f.(stayOnTop); ok && t.onTopLayer() {
			continue
		}
		if m.OnlyTop {
			if c, ok := f.(interface{ onTop() bool }); !ok || !c.onTop() {
				continue
			}
		}
		f.Update(in)
	}
	for _, f := range m.Forms {
		if t, ok := f.(stayOnTop); ok && t.onTopLayer() {
			f.Update(in)
		}
	}
	if in.Hovered != nil {
		in.Hovered.Hint()
	}
}

func (c *Component) onTop() bool { return c.OnTop }

// Mouse buttons as DelphiX reports them.
const (
	ButtonLeft  = 0
	ButtonRight = 1
)

// MouseDown is FUN_0040f5d0. It reports whether the interface took the
// press. Only the framework's own classes are dispatched; the original's
// furniture and vendor item classes are not ported.
func (m *Manager) MouseDown(button, shift byte, x, y int) bool {
	// Let transient menus observe outside presses, including clicks on the world.
	for _, form := range m.Forms {
		if form.Base().Visible {
			if observer, ok := form.(interface{ PointerDown(int, int) }); ok {
				observer.PointerDown(x, y)
			}
		}
	}
	in := m.Input
	h := in.Hovered
	in.Focused = h
	if h == nil {
		return false
	}
	switch button {
	case ButtonLeft:
		if _, ok := h.(*ScrollButton); ok {
			in.Captured = h
		}
		if _, ok := h.(interface{ isComponent() }); ok {
			h.Activate(false)
			h.LeftDown(shift, x, y)
		}
		return true
	case ButtonRight:
		h.RightDown(shift, x, y)
		return true
	}
	return false
}

func (c *Component) isComponent() {}

// MouseMove is FUN_0040f70c.
func (m *Manager) MouseMove(shift byte, x, y int, held bool) {
	in := m.Input
	in.X, in.Y = x, y
	if h := in.Hovered; h != nil {
		h.MouseMove(shift, x, y)
	}
	if c := in.Captured; c != nil && c != in.Hovered {
		c.CapturedMove(shift, x, y)
		if !held {
			in.Captured = nil
		}
	}
}

// MouseUp is FUN_0040f7e4.
func (m *Manager) MouseUp(button, shift byte, x, y int) {
	in := m.Input
	if button == ButtonLeft && in.Pressed != nil {
		in.Pressed.LeftUp(shift, x, y)
	}
	if h := in.Hovered; h != nil {
		switch button {
		case ButtonLeft:
			h.LeftUp(shift, x, y)
		case ButtonRight:
			h.RightUp(shift, x, y)
		}
	}
	if c := in.Captured; c != nil && c != in.Hovered {
		switch button {
		case ButtonLeft:
			c.LeftUpOutside(shift, x, y)
		case ButtonRight:
			c.RightUpOutside(shift, x, y)
		}
	}
	in.Captured = nil
}

// DblClick is FUN_0040f4f4's first part: the hovered control's slot +0x2c.
func (m *Manager) DblClick() {
	if h := m.Input.Hovered; h != nil {
		h.DblClick()
	}
}

// KeyDown and KeyUp are FUN_0040f560 and FUN_0040f598: the focused
// control's slots +0x60 and +0x64.
func (m *Manager) KeyDown(key uint16, shift byte) {
	if f := m.Input.Focused; f != nil {
		f.KeyDown(key, shift)
	}
}

func (m *Manager) KeyUp(key uint16, shift byte) {
	if f := m.Input.Focused; f != nil {
		f.KeyUp(key, shift)
	}
}

// Char routes a typed character to the focused editor (FUN_0040f4f4's
// second part, FUN_0046942c).
func (m *Manager) Char(c byte) {
	if e, ok := m.Input.Focused.(interface{ Char(byte) }); ok {
		e.Char(c)
	}
}

// Focus is FUN_0040f974: the control becomes both hovered and focused.
func (m *Manager) Focus(c Control) {
	m.Input.Hovered, m.Input.Focused = c, c
}

// Wheel routes the native wheel slots through the hovered control's owners.
// Modal forms keep the wheel from reaching controls behind them.
func (m *Manager) Wheel(up bool) bool {
	for c := m.Input.Hovered; c != nil; c = c.Base().Parent {
		if m.Modal != nil && c.Base().Root() != m.Modal {
			return false
		}
		if w, ok := c.(interface {
			WheelUp()
			WheelDown()
		}); ok {
			if up {
				w.WheelUp()
			} else {
				w.WheelDown()
			}
			return true
		}
	}
	return false
}
