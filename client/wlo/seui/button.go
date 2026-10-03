package seui

import (
	"image"
	"time"
)

// FixedButton is TSe_FixedButton (constructor FUN_0046bb34): a three-row
// image whose row is the state: 0 idle, 1 under the pointer, 2 pressed.
type FixedButton struct {
	Component
	State     byte   // +0xd0
	FontSize  byte   // +0xe4: 0 8x15, 1 10x19, 2 12x24
	CharW     int    // +0xe8
	CharH     int    // +0xec
	Pressed   bool   // +0xf1
	Toggle    bool   // +0xf2
	Toggled   bool   // +0xf3
	Icon      int    // +0xf4 centred icon, -1 for none
	IconH     int    // +0xd4
	IconW     int    // +0xd8
	IconX     int    // +0xdc source position
	IconY     int    // +0xe0
	IconDown  bool   // +0xf0 the icon's third row is used
	Sticky    bool   // +0xf8 stays pressed after a click
	TextStyle byte   // +0xf9
	Caption   []byte // +0x48
	TextX     int    // +0x40
	TextY     int    // +0x44
	// Frame animation (FUN_0046bd0c): while Animating, the button shows
	// frames AnimFirst..AnimLast of AnimImage instead of its state row.
	Animating bool          // +0x100
	AnimImage int           // +0xfc
	animAt    time.Time     // +0x108
	AnimFrame byte          // +0x110
	AnimCount byte          // +0x111 frames in the picture
	AnimX     int           // +0x114 source position
	AnimY     int           // +0x118
	AnimW     int           // +0x11c
	AnimH     int           // +0x120 frame height
	AnimEvery time.Duration // +0x128
	AnimFirst byte          // +0x130
	AnimLast  byte          // +0x131
	Now       func() time.Time
}

func NewFixedButton(env *Env, owner Control) *FixedButton {
	b := &FixedButton{}
	b.initComponent(b, env, owner)
	b.setFontSize(0)
	b.Enabled = true
	b.Name = "TSe_FixedButton"
	b.Icon, b.TextStyle = -1, 2
	b.AnimImage = -1
	b.Now = time.Now
	return b
}

// SetAnimation is FUN_0046bd0c in the original argument order: picture,
// source x, last frame, first frame, interval, frame height, frame width,
// source y. Frames are numbered from 1; 0 means the picture's last frame
// (for last) or 1 (for first).
func (b *FixedButton) SetAnimation(name string, x int, last, first byte, every time.Duration, frameH, frameW, y int) {
	b.AnimImage = b.Env.Pics.Find(name)
	if b.AnimImage == -1 {
		b.clearAnimation()
	}
	_, h := b.Env.Pics.Size(b.AnimImage)
	if frameH > 0 {
		b.AnimCount = byte(h / frameH)
	}
	b.AnimEvery, b.AnimX, b.AnimY, b.AnimW, b.AnimH = every, x, y, frameW, frameH
	b.AnimFirst = first
	if first == 0 || b.AnimCount < first {
		b.AnimFirst = 1
	}
	b.AnimLast = last
	if last == 0 || b.AnimCount < last {
		b.AnimLast = b.AnimCount
	}
	if b.AnimLast < b.AnimFirst {
		b.AnimLast = b.AnimFirst
	}
}

// clearAnimation is FUN_0046bac8.
func (b *FixedButton) clearAnimation() {
	b.AnimImage, b.Animating, b.animAt = -1, false, time.Time{}
	b.AnimFrame, b.AnimCount = 0, 0
	b.AnimEvery, b.AnimX, b.AnimY, b.AnimW, b.AnimH = 0, 0, 0, 0, 0
	b.AnimFirst, b.AnimLast = 0, 0
}

// StartAnimation is FUN_0046bce0.
func (b *FixedButton) StartAnimation() {
	b.Animating, b.animAt, b.AnimFrame = true, b.Now(), b.AnimFirst
}

// StopAnimation is FUN_0046bd04.
func (b *FixedButton) StopAnimation() { b.Animating = false }

// setFontSize is FUN_0046be94.
func (b *FixedButton) setFontSize(size byte) {
	b.FontSize = size
	switch size {
	case 0:
		b.CharW, b.CharH = 8, 0xf
	case 1:
		b.CharW, b.CharH = 10, 0x13
	case 2:
		b.CharW, b.CharH = 0xc, 0x18
	}
}

// SetToggle is FUN_0046be2c.
func (b *FixedButton) SetToggle(on bool) { b.Toggle = on }

// SetCaption is FUN_0046be3c with FUN_0046be58's centring.
func (b *FixedButton) SetCaption(s []byte) {
	b.Caption = s
	b.TextY = b.Top + (b.Height-b.CharH)/2
	b.TextX = b.Left + (b.Width-len(s)*b.CharW)/2
}

// Click is FUN_0046bc9c: a programmatic click.
func (b *FixedButton) Click() {
	if !b.Root().Base().Visible || !b.Enabled {
		return
	}
	b.State = 2
	if b.OnClick != nil {
		b.OnClick()
	}
	if b.OnClickTag != nil {
		b.OnClickTag(b.Tag)
	}
}

// LeftDown is slot +0x38 (FUN_0046bbc4).
func (b *FixedButton) LeftDown(shift byte, x, y int) {
	b.Component.LeftDown(shift, x, y)
	if !b.Blocked() && b.self.HitTest(x, y) {
		b.Pressed, b.State = true, 2
	}
}

// LeftUp is slot +0x30 (FUN_0046bc14): a release after a press clicks.
func (b *FixedButton) LeftUp(shift byte, x, y int) {
	if !b.live() || !b.Enabled || b.Blocked() || !b.Pressed {
		return
	}
	b.Component.LeftUp(shift, x, y)
	b.State = 1
	if b.Sticky {
		b.State = 2
	}
	if b.Toggle {
		b.Toggled = !b.Toggled
	}
}

// Update is slot +0x18 (FUN_0046c01c).
func (b *FixedButton) Update(in *Input) {
	if !b.Visible {
		return
	}
	if b.self.HitTest(in.X, in.Y) {
		in.Hovered = b.self
	}
	if !b.Blocked() {
		b.Focused = in.Focused == b.self
	}
	if in.Hovered == b.self && !b.Blocked() {
		if b.State != 2 {
			b.State = 1
			if b.Sticky {
				b.State = 2
			}
		}
	} else {
		if !b.Sticky {
			if b.Icon == -1 {
				b.State = 0
			}
		} else {
			b.State = 2
		}
		b.Pressed, b.Dragging = false, false
	}
	if b.Animating {
		if now := b.Now(); now.Sub(b.animAt) >= b.AnimEvery {
			b.animAt = now
			b.AnimFrame++
			if b.AnimFrame > b.AnimLast {
				b.AnimFrame = b.AnimFirst
			}
		}
	}
	if b.Toggle && b.Toggled {
		b.State = 2
	}
	b.self.Paint()
	b.paintIcon(b.Abs())
	b.updateChildren(in)
}

// paintIcon is FUN_0046e478. CenterX and CenterY already include Left and
// Top, and the original adds the control's screen position as well, so a
// control away from its parent's origin draws its icon offset by its own
// position again.
func (b *FixedButton) paintIcon(abs image.Point) {
	if b.Icon == -1 {
		return
	}
	y := b.IconY
	if b.IconDown {
		y += 2 * b.IconH
	}
	r := image.Rect(b.IconX, y, b.IconX+b.IconW, y+b.IconH)
	x := b.CenterX() - int(uint32(b.IconW)>>1) + abs.X
	yy := b.CenterY() - int(uint32(b.IconH)>>1) + abs.Y
	b.Env.Pics.DrawRect(b.Env.Screen, b.Icon, x, yy, r, true)
}

// Paint is slot +0x10 (FUN_0046bee8): the state's row of the clip, then
// the caption.
func (b *FixedButton) Paint() {
	o := b.ParentOrigin()
	if b.Animating {
		if b.AnimImage != -1 {
			y := (int(b.AnimFrame)-1)*b.AnimH + b.AnimY
			r := image.Rect(b.AnimX, y, b.AnimX+b.AnimW, y+b.AnimH)
			b.Env.Pics.DrawRect(b.Env.Screen, b.AnimImage, o.X+b.Left, o.Y+b.Top, r, true)
		}
	} else if b.Image != -1 {
		h := b.Clip.Dy()
		r := image.Rect(b.Clip.Min.X, int(b.State)*h+b.Clip.Min.Y, b.Clip.Min.X+b.Clip.Dx(), int(b.State)*h+b.Clip.Min.Y+h)
		b.Env.Pics.DrawRect(b.Env.Screen, b.Image, o.X+b.Left, o.Y+b.Top, r, true)
	}
	if len(b.Caption) > 0 {
		b.Env.Text.Draw(o.X+b.TextX, o.Y+b.TextY, 0, false, true, b.Env.Screen, b.Caption, b.CharH, b.Width, b.Color2, b.Color, b.TextStyle)
	}
}
