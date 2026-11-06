package inventory

import (
	"image"
	"time"
)

const pointAnimationFrames = 3
const pointAnimationInterval = 200 * time.Millisecond // FUN_00356f3c.

func (f *Form) paintPointAnimation(points uint16) {
	if f.Selected != 0 || points == 0 {
		f.pointAnimationAt = time.Time{}
		return
	}
	now := time.Now()
	if f.Now != nil {
		now = f.Now()
	}
	if f.pointAnimationAt.IsZero() {
		f.pointAnimationAt = now
	}
	frame := int(now.Sub(f.pointAnimationAt)/pointAnimationInterval) % pointAnimationFrames
	i := f.Env.Pics.Find("Btn_Point_1")
	w, h := f.Env.Pics.Size(i)
	if i < 0 || w == 0 || h == 0 {
		return
	}
	h /= pointAnimationFrames
	// FUN_0035172c: artwork overlays the static POINT label at (123,319).
	f.Env.Pics.DrawRect(f.Env.Screen, i, f.Left+123, f.Top+319, image.Rect(0, frame*h, w, (frame+1)*h), true)
}
