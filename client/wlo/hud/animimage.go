package hud

import (
	"image"
	"time"

	"wonderland-gonline/client/wlo/seui"
)

// AnimImage is TAnimateImage (the base of TEmotionImage and TActionImage;
// update FUN_00280b58, paint FUN_0028087c): a picture of square frames
// stacked vertically. At rest it shows its third frame (+0x138 = 2), or the
// first when it has fewer than three; under the pointer it is drawn over
// a highlight (TColor $F5BF89 at alpha 0x50) and steps through its frames
// every 200 ms (+0x148).
type AnimImage struct {
	seui.FixedButton
	frame  int       // +0x138
	stepAt time.Time // +0x140
	Now    func() time.Time
}

const (
	animRestFrame = 2
	animEvery     = 200 * time.Millisecond
	animHighlight = 0xf5bf89
	animAlpha     = 0x50
)

// NewAnimImage creates an AnimImage owned by owner.
func NewAnimImage(env *seui.Env, owner seui.Control) *AnimImage {
	a := &AnimImage{Now: time.Now}
	a.InitFixedButton(a, env, owner)
	return a
}

func (a *AnimImage) frames() int {
	w, h := a.Env.Pics.Size(a.Image)
	if w == 0 {
		return 0
	}
	return h / w
}

// Update is FUN_00280b58.
func (a *AnimImage) Update(in *seui.Input) {
	a.FixedButton.Update(in)
	hovered := in.Hovered == seui.Control(a)
	n := a.frames()
	switch {
	case n < 3:
		a.frame = 0
	case !hovered:
		a.frame = animRestFrame
	}
	if hovered && n > 0 {
		now := a.Now()
		if now.Sub(a.stepAt) > animEvery {
			a.frame = (a.frame + 1) % n
			a.stepAt = now
		}
	}
}

// Paint is FUN_0028087c.
func (a *AnimImage) Paint() {
	if a.Image < 0 {
		return
	}
	at := a.Abs()
	if a.Env.UI.Input.Hovered == seui.Control(a) {
		a.Env.Screen.FillAlpha(image.Rect(at.X, at.Y, at.X+a.Width, at.Y+a.Height), animHighlight, animAlpha)
	}
	h := a.Clip.Dy()
	a.Env.Pics.DrawRect(a.Env.Screen, a.Image, at.X, at.Y, image.Rect(0, a.frame*h, a.Clip.Dx(), a.frame*h+h), true)
}
