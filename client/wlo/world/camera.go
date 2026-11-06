package world

import (
	"math"
	"time"
)

const (
	cameraFollowTime           = 75 * time.Millisecond
	cameraFollowSnapDistance   = 160.0
	cameraFollowIdleReset      = 250 * time.Millisecond
	cameraFollowSettleDistance = .05
)

type cameraFollow struct {
	x, y                  float64
	at                    time.Time
	ready                 bool
	viewWidth, viewHeight int
}

// CameraPosition retains the fractional follow position for scene composition.
// Camera remains the shared integer coordinate system for picking/native draws.
func (w *World) CameraPosition() (float64, float64) {
	if w.CameraAt == nil && w.followCamera.ready {
		return w.followCamera.x, w.followCamera.y
	}
	x, y := w.Camera()
	return float64(x), float64(y)
}

// Advance once with simulation, never from Camera queries. All picking,
// occlusion, effects and rendering therefore share the same camera position.
// Exponential following eases starts/stops without overshoot at map boundaries.
func (w *World) stepCamera(now time.Time) {
	if w.Scene == nil {
		return
	}
	c := &w.followCamera
	if w.CameraAt != nil {
		c.ready = false
		return
	}
	tx, ty := w.cameraTarget()
	x, y := float64(tx), float64(ty)
	elapsed := now.Sub(c.at)
	if !c.ready || elapsed < 0 || elapsed > cameraFollowIdleReset || c.viewWidth != w.ViewWidth || c.viewHeight != w.ViewHeight || math.Hypot(x-c.x, y-c.y) > cameraFollowSnapDistance {
		*c = cameraFollow{x: x, y: y, at: now, ready: true, viewWidth: w.ViewWidth, viewHeight: w.ViewHeight}
		return
	}
	if elapsed == 0 {
		return
	}
	fraction := -math.Expm1(-float64(elapsed) / float64(cameraFollowTime))
	c.x += (x - c.x) * fraction
	c.y += (y - c.y) * fraction
	if math.Abs(x-c.x) < cameraFollowSettleDistance {
		c.x = x
	}
	if math.Abs(y-c.y) < cameraFollowSettleDistance {
		c.y = y
	}
	c.at = now
}
