package app

import (
	"image"
	"testing"
)

func TestSessionScrollWheelDragAndCapture(t *testing.T) {
	w := &Workspace{Sessions: make([]*Session, 9)}
	for i := range w.Sessions {
		w.Sessions[i] = &Session{}
	}
	w.Pointer(ScreenWidth+20, 60, false, false, -0.25)
	if w.scrollTarget != 27 || w.scrollPosition != 0 {
		t.Fatal("fractional wheel input lost or jumped", w.scrollTarget, w.scrollPosition)
	}
	w.stepScroll()
	if w.scrollPosition <= 0 || w.scrollPosition >= w.scrollTarget {
		t.Fatal("wheel did not ease")
	}
	before := w.scrollPosition
	w.Pointer(ScreenWidth+20, 60, false, false, -0.25)
	w.stepScroll()
	if w.scrollTarget != 54 || w.scrollPosition <= before {
		t.Fatal("wheel accumulation failed")
	}
	for i := 0; i < 40; i++ {
		w.stepScroll()
	}
	track, thumb := w.scrollbarRects()
	x, y := thumb.Min.X+1, thumb.Min.Y+4
	if !w.Pointer(x, y, true, true, 0) || !w.scrollDragging {
		t.Fatal("thumb not captured")
	}
	w.Pointer(x, track.Max.Y+100, false, true, 0)
	if w.scrollPosition != w.maxScrollPixels() {
		t.Fatal("drag did not clamp to bottom")
	}
	if !w.Pointer(100, 100, false, false, 0) || w.scrollDragging {
		t.Fatal("release leaked to game")
	}
	if w.Pointer(100, 100, false, false, 0) {
		t.Fatal("capture persisted")
	}
	w.scrollTo(0, true)
	_, thumb = w.scrollbarRects()
	w.Pointer(x, thumb.Max.Y+2, true, false, 0)
	if w.scrollTarget <= 0 || w.scrollPosition != 0 {
		t.Fatal("track click did not page smoothly")
	}
}

func TestSessionScrollGeometryAndCollapsedPanel(t *testing.T) {
	w := &Workspace{Sessions: make([]*Session, 5)}
	w.scrollTo(54, true)
	first := w.cardRect(0)
	if first.Min.Y != sessionListTop-54 {
		t.Fatal("fractional card offset incorrect", first)
	}
	track, _ := w.scrollbarRects()
	if !first.Intersect(track).Empty() {
		t.Fatal("cards overlap scrollbar")
	}
	if !w.addVisible() {
		t.Fatal("partial add card should be reachable")
	}
	w.Collapsed = true
	w.slide = sessionSlideTicks
	before := w.scrollTarget
	w.Pointer(ScreenWidth+20, 60, false, false, -1)
	if w.scrollTarget != before {
		t.Fatal("hidden panel scrolled")
	}
	w.Collapsed = false
	w.slide = 0
	w.Sessions = w.Sessions[:4]
	w.stepScroll()
	for i := 0; i < 40; i++ {
		w.stepScroll()
	}
	if w.scrollPosition != 0 || w.scrollTarget != 0 {
		t.Fatal("removed sessions left stale scroll")
	}
	if !image.Pt(w.addRect().Min.X+1, w.addRect().Min.Y+1).In(w.listRect()) {
		t.Fatal("add card not restored")
	}
}
