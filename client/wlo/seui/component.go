package seui

// Component is TSe_Component (constructor FUN_0046e724).
type Component struct {
	GrBasic
	TabStop    bool // +0xb9
	Enabled    bool // +0xba
	OnTop      bool // +0xbb drawn while the manager is modal
	FocusIndex int  // +0xc0
	Shown      bool // +0xc4
	Cursor     byte // +0xc5, the cursor state while hovered (2 the hand; forms 0)
	Group      byte // +0xc6
	DownSound  byte // +0xc7; mouse-down plays wav0151 unless 0xff
	UpSound    bool // +0xc8 right-up plays wav0154 when RightSound is set
	RightDown_ bool // +0xc9 right-down plays wav0151 unless RightSound
	ClickSound bool // +0xca left-up plays wav0154; mutes the mouse-down sound
	RightSound bool // +0xcb
}

func (c *Component) initComponent(self Control, env *Env, owner Control) {
	c.initBasic(self, env, owner)
	c.Name = "TSe_Component"
	c.TabStop, c.Enabled, c.Shown, c.FocusIndex = true, true, true, 0
	c.Cursor, c.Group = 2, 0xff
}

// NewComponent creates a plain TSe_Component.
func NewComponent(env *Env, owner Control) *Component {
	c := &Component{}
	c.initComponent(c, env, owner)
	return c
}

// CursorState is the cursor state while the pointer is over the control
// (+0xc5, read by FUN_00467148).
func (c *Component) CursorState() byte { return c.Cursor }

// Blocked is FUN_0046e544: a modal form is open and is not this control's
// root.
func (c *Component) Blocked() bool {
	ui := c.Env.UI
	if ui == nil || ui.Modal == nil {
		return false
	}
	return c.Root() != ui.Modal
}

// Update is slot +0x18 (FUN_0046e67c): like TSe_GrBasic's, but focus is only
// tracked while not blocked.
func (c *Component) Update(in *Input) {
	if !c.Visible {
		return
	}
	if c.self.HitTest(in.X, in.Y) {
		in.Hovered = c.self
	}
	if !c.Blocked() {
		c.Focused = in.Focused == c.self
	}
	c.self.Paint()
	c.updateChildren(in)
}

// Hint is slot +0x1c (FUN_0046e958).
func (c *Component) Hint() {
	if c.Env.HintOwner != c.self {
		c.GrBasic.Hint()
	}
}

// LeftDown is slot +0x38 (FUN_0046e570): the root form is activated and the
// press sound played.
func (c *Component) LeftDown(shift byte, x, y int) {
	c.GrBasic.LeftDown(shift, x, y)
	if !c.live() {
		return
	}
	if !c.Blocked() {
		c.Root().Activate(true)
	}
	if c.DownSound != 0xff && !c.ClickSound {
		c.Env.play(`sound\wav0151.wav`)
	}
}

// LeftUp is slot +0x30 (FUN_0046e7d4).
func (c *Component) LeftUp(shift byte, x, y int) {
	c.GrBasic.LeftUp(shift, x, y)
	if c.ClickSound {
		c.Env.play(`sound\wav0154.wav`)
	}
}

// RightDown is slot +0x44 (FUN_0046e970).
func (c *Component) RightDown(shift byte, x, y int) {
	c.GrBasic.RightDown(shift, x, y)
	if !c.live() {
		return
	}
	if !c.Blocked() {
		c.Root().Activate(true)
	}
	if c.RightDown_ && !c.RightSound {
		c.Env.play(`sound\wav0151.wav`)
	}
}

// RightUp is slot +0x3c (FUN_0046ea40).
func (c *Component) RightUp(shift byte, x, y int) {
	c.GrBasic.RightUp(shift, x, y)
	if c.UpSound && c.RightSound {
		c.Env.play(`sound\wav0154.wav`)
	}
}

// Activate is slot +0x6c (FUN_0046e538): nothing for components.
func (c *Component) Activate(bool) {}

// Show is slot +0x20 (FUN_0046e53c).
func (c *Component) Show() { c.Visible = true }

// SetEnv is FUN_0046e640: the manager is recorded on the control and all
// of its descendants.
func SetEnv(c Control, env *Env) {
	c.Base().Env = env
	for _, k := range c.Base().Children {
		SetEnv(k, env)
	}
}

// FirstFocusable is FUN_0046e8b0: the remembered child if it can take
// focus, otherwise the first visible tab stop.
func (c *Component) FirstFocusable() Control {
	tab := func(k Control) bool {
		if !k.Base().Visible {
			return false
		}
		if t, ok := k.(interface{ tabStop() bool }); ok {
			return t.tabStop()
		}
		return false
	}
	if len(c.Children) > 0 {
		if c.FocusIndex < len(c.Children) && tab(c.Children[c.FocusIndex]) {
			return c.Children[c.FocusIndex]
		}
		for i, k := range c.Children {
			if tab(k) {
				c.FocusIndex = i
				return k
			}
		}
	}
	return c.self
}

func (c *Component) tabStop() bool { return c.TabStop }
