package world

import (
	"image"
	"math"
	"testing"
	"time"
)

func TestCameraSmoothFollowAndEdges(t *testing.T) {
	w := &World{Scene: &Scene{Width: 1664, Height: 1280}, Player: Player{X: 800, Y: 600}}
	now := time.Unix(0, 0)
	w.Step(now)
	w.Player.X += 100
	now = now.Add(16 * time.Millisecond)
	w.Step(now)
	x, _ := w.Camera()
	if x <= 400 || x >= 500 {
		t.Fatalf("camera did not ease toward player: %d", x)
	}
	// Queries must never advance the filter (input and draw agree).
	for i := 0; i < 10; i++ {
		if cx, _ := w.Camera(); cx != x {
			t.Fatal("camera changed during query")
		}
	}
	previous := x
	for i := 0; i < 60; i++ {
		now = now.Add(time.Second / 60)
		w.Step(now)
		x, _ = w.Camera()
		if x < previous || x > 500 {
			t.Fatal("camera overshot or reversed while settling")
		}
		previous = x
	}
	if x != 500 {
		t.Fatal("camera did not settle")
	}
	w.Relocate(image.Pt(1200, 1000))
	w.Step(now)
	for i := 0; i < 80; i++ {
		w.Player.X += 2
		w.Player.Y += 2
		now = now.Add(time.Second / 60)
		w.Step(now)
		x, y := w.Camera()
		if x > 844 || y > 680 {
			t.Fatal("smoothed camera crossed map edge")
		}
	}
	w.Relocate(image.Pt(500, 400))
	if x, y := w.Camera(); x != 100 || y != 100 {
		t.Fatal("teleport retained old camera")
	}
}

func TestCameraSmoothingUsesElapsedTime(t *testing.T) {
	measure := func(step time.Duration) float64 {
		w := &World{Scene: &Scene{Width: 3000, Height: 2000}, Player: Player{X: 800, Y: 600}}
		now := time.Unix(0, 0)
		w.Step(now)
		w.Player.X += 100
		for elapsed := step; elapsed <= 150*time.Millisecond; elapsed += step {
			w.Step(now.Add(elapsed))
		}
		return w.followCamera.x
	}
	if math.Abs(measure(10*time.Millisecond)-measure(25*time.Millisecond)) > 1e-9 {
		t.Fatal("camera smoothing depends on update rate")
	}
}
