package seui

import "image"

// Panel is TSe_panel (constructor FUN_0046ace0): a nine-slice frame cut
// from the clip by margins, with an optional centred icon.
type Panel struct {
	Component
	MarginL, MarginT int  // +0xf0, +0xf4
	MarginR, MarginB int  // +0xf8, +0xfc
	State            byte // +0x100 image row: 0 idle, 1 hover, 2 pressed
	Hover            bool // +0x101 the state follows the pointer
	TopLayer         bool // +0x108 drawn in the manager's second pass
	Icon             int  // +0x10c
	IconW, IconH     int  // +0xe4, +0xe0
	IconX, IconY     int  // +0xe8, +0xec source position
	Style            string
}

func NewPanel(env *Env, owner Control) *Panel {
	p := &Panel{}
	p.initPanel(p, env, owner)
	return p
}

func (p *Panel) initPanel(self Control, env *Env, owner Control) {
	p.initComponent(self, env, owner)
	p.Name = "panel"
	p.Visible = true
	p.SetMargins(6, 6, 6, 6)
	p.Icon = -1
	p.Hover = true
	p.Style = "NormalPanel"
}

func (p *Panel) onTopLayer() bool { return p.TopLayer }

// SetMargins is slot +0x70 (FUN_0046ad98) in the original argument order:
// left, top, bottom, right.
func (p *Panel) SetMargins(l, t, b, r int) {
	p.MarginL, p.MarginT, p.MarginB, p.MarginR = l, t, b, r
}

// SetIcon is slot +0x74 (FUN_0046adc0): name, source x, height, width,
// source y.
func (p *Panel) SetIcon(name string, x, h, w, y int) {
	p.Icon = p.Env.Pics.Find(name)
	p.IconH, p.IconW, p.IconX, p.IconY = h, w, x, y
}

// Update is slot +0x18 (FUN_0046af38).
func (p *Panel) Update(in *Input) {
	p.Component.Update(in)
	if !p.Enabled || !p.Hover {
		return
	}
	if in.Hovered == p.self && !p.Blocked() {
		if p.State != 2 {
			p.State = 1
		}
		return
	}
	p.State, p.Dragging = 0, false
}

// Paint is slot +0x10 (FUN_0046ae40): corners (+0x80), top (+0x84), bottom
// (+0x88), left (+0x8c), right (+0x90), centre (+0x7c), then the icon
// (+0x78).
func (p *Panel) Paint() {
	o := p.ParentOrigin()
	if p.Image != -1 {
		row := int(p.State)*p.Clip.Dy() + p.Clip.Min.Y
		clip := image.Rect(p.Clip.Min.X, row, p.Clip.Max.X, row+p.Clip.Dy())
		dst := image.Rect(o.X+p.Left, o.Y+p.Top, o.X+p.Left+p.Width, o.Y+p.Top+p.Height)
		p.ninePatch(dst, clip)
	}
	p.paintIcon(o)
}

// ninePatch draws the slices. Each edge and the centre are tiled with whole
// source spans and then the remainder; source rectangles are cut to the
// image as the blitter does.
func (p *Panel) ninePatch(dst, clip image.Rectangle) {
	l, t, r, b := p.MarginL, p.MarginT, p.MarginR, p.MarginB
	cw, ch := clip.Dx()-l-r, clip.Dy()-t-b
	w, h := dst.Dx()-l-r, dst.Dy()-t-b
	src := func(x0, y0, x1, y1 int) image.Rectangle {
		return image.Rect(clip.Min.X+x0, clip.Min.Y+y0, clip.Min.X+x1, clip.Min.Y+y1)
	}
	put := func(dx, dy int, sr image.Rectangle) {
		p.Env.Pics.DrawRect(p.Env.Screen, p.Image, dst.Min.X+dx, dst.Min.Y+dy, sr, true)
	}
	tile := func(n, unit int, f func(off, size int)) {
		if unit <= 0 {
			return
		}
		for i := range n / unit {
			f(i*unit, unit)
		}
		if rem := n % unit; rem > 0 {
			f(n/unit*unit, rem)
		}
	}
	// Corners: FUN_0046b11c.
	put(0, 0, src(0, 0, l, t))
	put(dst.Dx()-r, 0, src(clip.Dx()-r, 0, clip.Dx(), t))
	put(dst.Dx()-r, dst.Dy()-b, src(clip.Dx()-r, clip.Dy()-b, clip.Dx(), clip.Dy()))
	put(0, dst.Dy()-b, src(0, clip.Dy()-b, l, clip.Dy()))
	// Top and bottom: FUN_0046b96c, FUN_0046af8c.
	tile(w, cw, func(x, sw int) {
		put(l+x, 0, src(l, 0, l+sw, t))
		put(l+x, dst.Dy()-b, src(l, clip.Dy()-b, l+sw, clip.Dy()))
	})
	// Left and right: FUN_0046b2dc, FUN_0046b7f8.
	tile(h, ch, func(y, sh int) {
		put(0, t+y, src(0, t, l, t+sh))
		put(dst.Dx()-r, t+y, src(clip.Dx()-r, t, clip.Dx(), t+sh))
	})
	// Centre: FUN_0046b438.
	tile(w, cw, func(x, sw int) {
		tile(h, ch, func(y, sh int) { put(l+x, t+y, src(l, t, l+sw, t+sh)) })
	})
}

// paintIcon is slot +0x78 (FUN_0046aea4).
func (p *Panel) paintIcon(o image.Point) {
	if p.Icon == -1 {
		return
	}
	r := image.Rect(p.IconX, p.IconY, p.IconX+p.IconW, p.IconY+p.IconH)
	x := o.X + p.CenterX() - p.IconW/2
	y := o.Y + p.CenterY() - p.IconH/2
	p.Env.Pics.DrawRect(p.Env.Screen, p.Icon, x, y, r, true)
}

// HitTest is slot +0x68 (FUN_0046aadc): inside the rectangle, then the
// point is mapped back through the slices to a source pixel.
func (p *Panel) HitTest(x, y int) bool {
	o := p.ParentOrigin()
	box := image.Rect(o.X+p.Left, o.Y+p.Top, o.X+p.Left+p.Width, o.Y+p.Top+p.Height)
	if !image.Pt(x, y).In(box) {
		return false
	}
	if !p.PixelHit {
		return true
	}
	if p.Image == -1 {
		return false
	}
	rx, ry := x-o.X-p.Left, y-o.Y-p.Top
	sx, sy := 0, 0
	switch {
	case rx <= p.MarginL:
		sx = rx
	case rx <= p.Width-p.MarginR:
		sx = (rx-p.MarginL)%(p.Clip.Dx()-p.MarginL-p.MarginR) + p.MarginL
	case rx < p.Width:
		sx = rx - (p.Width - p.MarginR) + p.Clip.Dx() - p.MarginR
	}
	switch {
	case ry <= p.MarginT:
		sy = ry
	case ry <= p.Height-p.MarginB:
		// The original adds the bottom margin here, not the top.
		sy = (ry-p.MarginT)%(p.Clip.Dy()-p.MarginT-p.MarginB) + p.MarginB
	case ry < p.Height:
		sy = ry - (p.Height - p.MarginB) + p.Clip.Dy() - p.MarginB
	}
	return p.Env.Pics.Opaque(p.Image, sx, sy)
}
