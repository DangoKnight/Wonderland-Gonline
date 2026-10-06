package inventory

import (
	"image"
	"math"
	"time"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
)

const (
	compoundResultFrame           = 14  // FUN_0021cf04 releases the result at animation index 14.
	compoundCauldronX             = 238 // Native animation centre/bottom anchor.
	compoundCauldronY             = 162
	compoundResultX               = 260 // Native result centre/bottom launch anchor.
	compoundResultY               = 70
	compoundResultPixelsPerSecond = 120 // FUN_003c862c: elapsed milliseconds * 0.12.
)

type compoundResult struct {
	item      game.Item
	slot      byte
	confirmed bool
	launch    time.Time
}

// This first child paints last, above ingredient and bag controls.
type compoundPresentation struct {
	seui.Component
	form *CompoundForm
}

func (f *CompoundForm) initPresentation() {
	p := &compoundPresentation{form: f}
	p.InitComponent(p, f.Env, f)
	p.Init("", 0, 0, 0, 0, 0, false, 0, 0, 0)
	p.TabStop = false
	f.visual = p
}

// AC23:8 already committed inventory. Hide only that exact item's visual until
// the confirmed AC23:13 result reaches its bag anchor; never delay state updates.
func (f *CompoundForm) QueueResult(slot byte) {
	if !f.Visible || slot < 1 || int(slot) > game.BagSize {
		return
	}
	it := f.Inventory.State.Bag[slot-1]
	if it.Empty() {
		return
	}
	if !f.Now().Before(f.animationUntil) {
		f.sentAt = f.Now()
		f.animationUntil = f.sentAt.Add(compoundAnimationFrames * compoundAnimationInterval)
	}
	f.result = &compoundResult{item: it, slot: slot}
}
func (f *CompoundForm) resultHidden(slot byte) bool {
	return f.result != nil && f.result.slot == slot && f.Inventory.State.Bag[slot-1] == f.result.item
}
func (f *CompoundForm) resultPath() (image.Point, image.Point, time.Duration) {
	start := image.Pt(compoundResultX-cellSize/2, compoundResultY-cellSize)
	slot := f.Slots[f.result.slot-1]
	end := slot.Abs().Sub(f.Abs())
	distance := max(abs(end.X-start.X), abs(end.Y-start.Y))
	duration := time.Duration(float64(distance) / compoundResultPixelsPerSecond * float64(time.Second))
	return start, end, max(duration, time.Millisecond)
}
func (f *CompoundForm) advancePresentation() {
	if f.result == nil {
		return
	}
	if !f.Visible || !f.resultHidden(f.result.slot) {
		f.result = nil
		return
	}
	if !f.result.confirmed {
		if f.Now().Sub(f.sentAt) >= compoundReplyTimeout {
			f.result = nil
		}
		return
	}
	_, _, duration := f.resultPath()
	if !f.Now().Before(f.result.launch.Add(duration)) {
		f.result = nil
	}
}
func (p *compoundPresentation) Paint() {
	f := p.form
	a := f.Abs()
	if f.Junior.Visible {
		f.Env.Text.Draw(a.X+38, a.Y+53, 0, false, true, f.Env.Screen, []byte("Use Junior Alchemy"), 13, 150, inventoryTextPaper, inventoryTextInk, 0)
	}
	if f.TierButton.Visible {
		f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find("Icon_UseCompoundSkill"), a.X+18, a.Y+47, true)
	}
	icon := f.Env.Pics.Find("Compounding")
	w, h := f.Env.Pics.Size(icon)
	if w > 0 && h >= compoundAnimationFrames {
		fh := h / compoundAnimationFrames
		frame := 0
		if f.Now().Before(f.animationUntil) {
			frame = min(compoundAnimationFrames-1, max(0, int(f.Now().Sub(f.sentAt)/compoundAnimationInterval)))
		}
		f.Env.Pics.DrawRect(f.Env.Screen, icon, a.X+compoundCauldronX-w/2, a.Y+compoundCauldronY-fh, image.Rect(0, frame*fh, w, (frame+1)*fh), true)
	}
	if f.result == nil || !f.result.confirmed || f.Now().Before(f.result.launch) {
		return
	}
	start, end, duration := f.resultPath()
	fraction := min(1.0, max(0.0, float64(f.Now().Sub(f.result.launch))/float64(duration)))
	at := image.Pt(start.X+int(math.Round(float64(end.X-start.X)*fraction)), start.Y+int(math.Round(float64(end.Y-start.Y)*fraction)))
	f.Inventory.drawItem(f.result.item, a.X+at.X, a.Y+at.Y, false)
}
