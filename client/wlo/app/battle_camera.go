package app

import (
	"image"
	"math"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
)

const (
	battleFieldWidth           = 1664 // FUN_0036fe90.
	battleFieldHeight          = 832
	battleZoomInsetX           = 100 // FUN_00386a0c, floats at 0x386c88/8c.
	battleZoomInsetY           = 75
	battleZoomAttackCategory   = 1 // FUN_003822d8 checks Skill.dat +0x1f.
	battleCameraFixed          = 1
	battleCameraPath           = 2
	battleCameraAnchorField    = 0
	battleCameraAnchorActor    = 1
	battleCameraAnchorTarget   = 2
	battleCameraLeftAdjust     = 18 // FUN_0037b730.
	battleCameraVerticalAdjust = 40
)

func battleCameraOrigin() image.Point { return image.Pt(battleSceneCameraX, battleSceneCameraY) }
func clampBattleCamera(p image.Point) image.Point {
	return image.Pt(max(0, min(battleFieldWidth-ScreenWidth, p.X)), max(0, min(battleFieldHeight-ScreenHeight, p.Y)))
}

// battleCamera keeps action camera/zoom state session-owned and advances it
// even when the workspace does not draw this session. Reactions never take
// camera ownership away from their attack.
func (c *Client) battleCamera(now time.Time) {
	v := &c.battle
	v.camera, v.zoom = battleCameraOrigin(), image.Rect(0, 0, ScreenWidth, ScreenHeight)
	for _, p := range v.playing {
		if len(p.action.Targets) == 0 || v.assets == nil {
			continue
		}
		m := v.assets.Motions[p.action.Skill]
		actor, target := p.action.Actor, battleTarget(p.action)
		elapsed := now.Sub(p.at)
		at := m.PositionAt(actor, target, elapsed)
		v.camera = motionCamera(m, actor, target, elapsed)
		if c.SettingsState != nil && !c.SettingsState.Local.Zoom {
			return
		}
		if c.SkillState == nil || c.SkillState.Catalog.Definitions[p.action.Skill].Attack != battleZoomAttackCategory {
			return
		}
		initial := abs(actor.Position().X - target.Position().X)
		remaining := abs(at.X - target.Position().X)
		if initial == 0 || remaining >= initial {
			return
		}
		// FUN_00386880: strength is progress toward the target relative to
		// the original horizontal separation. Approaching enlarges; returning
		// restores the full view. It never scales the UI.
		progress := 1 - float64(remaining)/float64(initial)
		x := int(math.RoundToEven(battleZoomInsetX * progress))
		y := int(math.RoundToEven(battleZoomInsetY * progress))
		v.zoom = image.Rect(x, y, ScreenWidth-x, ScreenHeight-y)
		return
	}
}

func motionCamera(m clientbattle.Motion, actor, target clientbattle.Cell, elapsed time.Duration) image.Point {
	origin := battleCameraOrigin()
	if len(m.Camera) == 0 {
		return origin
	}
	current := origin
	var previous time.Duration
	for _, p := range m.Camera {
		destination := origin
		switch m.CameraMode {
		case battleCameraFixed:
			switch p.Anchor {
			case battleCameraAnchorActor:
				destination = m.PositionAt(actor, target, elapsed).Add(origin).Sub(image.Pt(ScreenWidth/2, ScreenHeight/2))
			case battleCameraAnchorTarget:
				destination = target.Position().Add(origin).Sub(image.Pt(ScreenWidth/2, ScreenHeight/2))
			case battleCameraAnchorField:
				destination = origin
			default:
				// Effect anchors share their actor-relative coordinate space.
				i := int(p.Anchor) - battleCameraAnchorTarget - 1
				if i >= 0 && i < len(m.Effects) && len(m.Effects[i].Points) > 0 {
					e := m.Effects[i].Points[0]
					if actor.X > 2 {
						e.X = -e.X
					}
					destination = actor.Position().Add(origin).Add(e).Sub(image.Pt(ScreenWidth/2, ScreenHeight/2))
				}
			}
		case battleCameraPath:
			destination = image.Pt(p.At.X-battleCameraLeftAdjust, p.At.Y+battleCameraVerticalAdjust)
			if actor.X > 2 {
				destination.X = 2*origin.X - p.At.X + battleCameraLeftAdjust
			}
		}
		destination = clampBattleCamera(destination)
		distance := max(abs(destination.X-current.X), abs(destination.Y-current.Y))
		var travel time.Duration
		if m.CameraMode == battleCameraPath && p.Speed > 0 {
			travel = time.Duration(float64(distance) / p.Speed * float64(time.Millisecond))
		}
		end := previous + travel
		if p.Trigger > 0 && p.Trigger <= clientbattle.LastFrameTrigger {
			end = max(end, m.EventAt(clientbattle.MotionEvent{Trigger: p.Trigger}))
		} else {
			end += time.Duration(p.Trigger) * time.Millisecond
		}
		if elapsed < end {
			if travel <= 0 {
				return destination
			}
			f := min(1, max(0, float64(elapsed-previous)/float64(travel)))
			return image.Pt(current.X+int(math.RoundToEven(float64(destination.X-current.X)*f)), current.Y+int(math.RoundToEven(float64(destination.Y-current.Y)*f)))
		}
		current, previous = destination, end
	}
	return current
}

func (c *Client) zoomBattleScene() {
	r := c.battle.zoom
	if r.Empty() || r == image.Rect(0, 0, ScreenWidth, ScreenHeight) {
		return
	}
	// Reuse the target for the whole battle; cropping and scaling stay on the
	// renderer's device, including background workspace thumbnail draws.
	v := &c.battle
	if v.zoomCanvas == nil || v.zoomCanvas.W != r.Dx() || v.zoomCanvas.H != r.Dy() {
		if v.zoomCanvas != nil {
			v.zoomCanvas.Close()
		}
		v.zoomCanvas = c.Screen.NewCompatible(r.Dx(), r.Dy())
	}
	v.zoomCanvas.DrawRect(0, 0, r, c.Screen, false)
	c.Screen.DrawStretch(image.Rect(0, 0, ScreenWidth, ScreenHeight), v.zoomCanvas, false)
}
