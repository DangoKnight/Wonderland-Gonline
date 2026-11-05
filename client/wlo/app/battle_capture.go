package app

import (
	"image"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
)

// The capture reaction supplies the hurt pose. Its target travels with the
// capturer's return displacement, using the same formation-scaled path and
// clock rather than an independent, unscaled reaction trajectory.
type battleCaptureDrag struct {
	actor, target  clientbattle.Cell
	motion         clientbattle.Motion
	started        time.Time
	returnDuration time.Duration // Failed captures release the monster back to its cell.
}

// Capture's zero-time event starts the reaction controller, not physical
// contact. The first authored keyframe reaches the monster; the second pulls
// it back. Keep the target stationary until that second keyframe starts.
const capturePullFrameTrigger = 2

func (d *battleCaptureDrag) pullStart() time.Time {
	return d.started.Add(d.motion.EventAt(clientbattle.MotionEvent{Trigger: capturePullFrameTrigger}))
}

// Keep the monster in front of the capturer's pulling pose (toward the
// enemy formation). Reflect the authored contact gap across the target,
// blending that correction along both legs to preserve their endpoints.
func (d *battleCaptureDrag) capturerPosition(now time.Time) image.Point {
	elapsed := min(max(time.Duration(0), now.Sub(d.started)), d.motion.ReturnStart())
	pull := d.pullStart().Sub(d.started)
	contact := d.motion.PositionAt(d.actor, d.target, pull)
	current := d.motion.PositionAt(d.actor, d.target, elapsed)
	end := d.motion.PositionAt(d.actor, d.target, d.motion.ReturnStart())
	base := d.actor.Position()
	if elapsed >= pull {
		base = end
	}
	denominator := contact.X - base.X
	if denominator == 0 {
		return current
	}
	gap := d.target.Position().Sub(contact)
	numerator := current.X - base.X
	return current.Add(image.Pt(2*gap.X*numerator/denominator, 2*gap.Y*numerator/denominator))
}

func (d *battleCaptureDrag) position(target clientbattle.Cell, now time.Time) image.Point {
	pull := d.pullStart()
	if now.Before(pull) {
		return target.Position()
	}
	contact := d.capturerPosition(pull)
	current := d.capturerPosition(now)
	delta := current.Sub(contact)
	if elapsed := now.Sub(d.started) - d.motion.ReturnStart(); elapsed > 0 && d.returnDuration > 0 {
		remaining := max(time.Duration(0), d.returnDuration-elapsed)
		delta.X = int(int64(delta.X) * int64(remaining) / int64(d.returnDuration))
		delta.Y = int(int64(delta.Y) * int64(remaining) / int64(d.returnDuration))
	}
	return target.Position().Add(delta)
}
