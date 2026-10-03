package movie

import (
	"image"
	"time"
)

// Stage effects. Entering a stage applies its three operands (FUN_003406c4
// after +0xf90 advances): the screen effect, two shake tracks and the
// music. The effect sets the overlay (+0xf9a) the draw paints over the
// scene (FUN_00340404 → FUN_003346e8) or a fade of the scene's light
// (DAT_0072a090, FUN_0033d14c) that holds the movie until it completes.
// The shakes (FUN_0033d514, FUN_0033d9ac) move the camera every frame
// (+4000, +0xfa4) or zoom the main form's view (+0x580).
const (
	EffectReset   = 0 // no overlay, full light
	EffectDark    = 1 // the light goes to 0
	EffectFadeOut = 2 // the light fades to 0, 5 a frame
	EffectLeaves  = 3 // overlay 1
	EffectSnow    = 4 // overlay 2
	EffectRain    = 5 // overlay 3
	EffectKeep    = 6
	EffectRed     = 7  // overlay 7
	EffectGrey    = 8  // overlay 8
	EffectTint    = 9  // overlay 9, the stage's colour
	EffectSteam   = 10 // overlay 10
)

// Overlays (+0xf9a). 7, 8 and 9 fill the screen with ro_ARGB colours
// through ro_Clipper_Alpha_Fill; 1, 2, 3 and 10 are the weather layer's
// kinds (weather.Leaves, Snow, Rain, Steam), which the movie's draw runs
// in place of the map's weather.
const (
	OverlayNone   = 0
	OverlayLeaves = 1
	OverlaySnow   = 2
	OverlayRain   = 3
	OverlayRed    = 7
	OverlayGrey   = 8
	OverlayTint   = 9
	OverlaySteam  = 10
)

// Shake operands.
const (
	ShakeOff       = 0
	ShakeQuake     = 1 // a seven-step jolt, one step a frame
	ShakeSettle    = 2 // smaller steps at shrinking intervals
	ShakeHideScene = 3 // the background is not drawn (+0xf99 = 0)
	ZoomIn         = 4 // the view narrows a step every 30 ms
	ZoomOut        = 5
	ZoomFull       = 6
)

const (
	lightFull       = 0xff
	lightStep       = 5
	settleGapStart  = 0xb4 // ms
	settleGapStep   = 8
	zoomInterval    = 30 * time.Millisecond
	ZoomSteps       = 9
	ZoomStepW       = 0x14
	ZoomStepH       = 0xf
	overlayRedA     = 0x7d
	overlayRedR     = 0xfa
	overlayRedGB    = 0x7d
	overlayGreyA    = 0xaf
	overlayGreyRGB  = 0x7d
	shakeQuakeSteps = 7
)

// The jolts: camera offsets for each step (FUN_0033d514).
var (
	quakeSteps  = [shakeQuakeSteps]image.Point{{5, -5}, {-5, 5}, {5, 2}, {-5, -5}, {0, 0}, {5, 5}, {0, -2}}
	settleSteps = [shakeQuakeSteps]image.Point{{2, -2}, {-2, 2}, {3, 0}, {-2, -2}, {0, 0}, {5, 5}, {0, 0}}
)

// GameFrame is the original's frame period: a 30 ms multimedia timer
// (timeSetEvent at 0x495b13) posts the frame message (FUN_00495a8c →
// FUN_004a1d60). Effects counted in frames (the fade's 5 a frame, the
// jolts' step a frame) advance on this clock, so they keep the original's
// pace while the client draws at the display's rate.
const GameFrame = 30 * time.Millisecond

// maxCatchUp bounds the game frames run after a stall.
const maxCatchUp = 10

// effects is the stage effect state.
type effects struct {
	frameAt   time.Time // the last game frame
	Overlay   byte      // +0xf9a
	Light     int       // DAT_0072a090: 255 full, 0 dark
	HideScene bool      // +0xf99 cleared
	Shake     image.Point
	Zoom      int // main form +0x580
	// MusicChanged is +0xfb0: the map's music returns when the movie ends.
	MusicChanged bool

	fading   bool // +0xf9b
	step     int  // +0xfa8
	settleAt time.Time
	gap      int // +0xfac
	zoomAt   time.Time
}

// ShakeOffset is the camera offset to draw with: the jolt while a stage
// runs one (1 or 2); the video shows the scene at rest in the storm's
// zoom-only stages, whose entry leaves the last jolt in place.
func (p *Player) ShakeOffset() image.Point {
	st := p.stage()
	for _, v := range []byte{st.ShakeA, st.ShakeB} {
		if v == ShakeQuake || v == ShakeSettle {
			return p.Shake
		}
	}
	return image.Point{}
}

// Fill is the overlay's full-screen colour (alpha, red, green, blue), false
// when the overlay paints nothing.
func (p *Player) Fill() (a, r, g, b byte, ok bool) {
	switch p.Overlay {
	case OverlayRed:
		return overlayRedA, overlayRedR, overlayRedGB, overlayRedGB, true
	case OverlayGrey:
		return overlayGreyA, overlayGreyRGB, overlayGreyRGB, overlayGreyRGB, true
	case OverlayTint:
		if p.Stage <= p.M.Last && p.Stage < len(p.M.Stages) {
			t := p.M.Stages[p.Stage].Tint
			return t[0], t[1], t[2], t[3], true
		}
	}
	return 0, 0, 0, 0, false
}

func (p *Player) stage() Stage {
	if p.Stage < len(p.M.Stages) {
		return p.M.Stages[p.Stage]
	}
	return Stage{}
}

// applyEffects is the stage entry's operand switches and music.
func (p *Player) applyEffects(now time.Time) {
	st := p.stage()
	switch st.Effect {
	case EffectReset:
		p.Overlay, p.Light = OverlayNone, lightFull
	case EffectDark:
		p.Light = 0
	case EffectFadeOut:
		p.Light, p.fading = lightFull, true
	case EffectLeaves:
		p.Overlay = OverlayLeaves
	case EffectSnow:
		p.Overlay = OverlaySnow
	case EffectRain:
		p.Overlay = OverlayRain
	case EffectSteam:
		p.Overlay = OverlaySteam
	case EffectRed, EffectGrey, EffectTint:
		p.Overlay = st.Effect
	}
	for _, v := range []byte{st.ShakeA, st.ShakeB} {
		switch v {
		case ShakeOff, ShakeQuake:
			p.Shake = image.Point{}
		case ShakeSettle:
			p.Shake, p.gap, p.settleAt = image.Point{}, settleGapStart, now
		}
	}
	// Stage 1 of a movie without music restarts the map's; otherwise the
	// stage's entry of the sound table plays.
	if p.Stage == 1 && len(p.M.Stages) > 1 && p.M.Stages[0].Music == 0 && st.Music == 0 {
		if p.MapMusic != nil {
			p.MapMusic()
		}
	} else if st.Music > 0 && p.Music != nil {
		p.Music(st.Music)
		p.MusicChanged = true
	}
}

// gameFrames runs the game frames due by now: the fade, then (once it has
// completed) the shakes. It reports whether a fade still holds the movie.
func (p *Player) gameFrames(now time.Time) bool {
	if p.frameAt.IsZero() {
		p.frameAt = now
	}
	for n := 0; now.Sub(p.frameAt) >= GameFrame; n++ {
		p.frameAt = p.frameAt.Add(GameFrame)
		if n >= maxCatchUp {
			p.frameAt = now
			break
		}
		if p.stepFade() {
			continue
		}
		p.stepShakes(p.frameAt)
	}
	return p.fading
}

// stepFade is FUN_0033d14c; it reports whether the movie must wait.
func (p *Player) stepFade() bool {
	if !p.fading {
		return false
	}
	switch p.stage().Effect {
	case EffectDark:
		if p.Light == lightFull {
			p.fading = false
		}
		p.Light = min(p.Light+lightStep, lightFull)
	case EffectFadeOut:
		if p.Light == 0 {
			p.fading = false
		}
		p.Light = max(p.Light-lightStep, 0)
	default:
		p.fading = false
	}
	return p.fading
}

// stepShakes runs the shake tracks that are on (+0xf9c, +0xf9d) for one
// frame.
func (p *Player) stepShakes(now time.Time) {
	st := p.stage()
	for _, v := range []byte{st.ShakeA, st.ShakeB} {
		if v != ShakeOff {
			p.shake(v, now)
		}
	}
}

func (p *Player) shake(v byte, now time.Time) {
	switch v {
	case ShakeQuake:
		p.Shake = quakeSteps[p.step]
		p.step = (p.step + 1) % shakeQuakeSteps
	case ShakeSettle:
		if now.Sub(p.settleAt) < time.Duration(p.gap)*time.Millisecond {
			return
		}
		p.settleAt = now
		p.gap = max(p.gap-settleGapStep, 0)
		p.Shake = settleSteps[p.step]
		p.step = (p.step + 1) % shakeQuakeSteps
	case ShakeHideScene:
		p.HideScene = true
	case ZoomIn, ZoomOut:
		if now.Sub(p.zoomAt) < zoomInterval {
			return
		}
		p.zoomAt = now
		if v == ZoomIn {
			p.Zoom = min(p.Zoom+1, ZoomSteps)
		} else {
			p.Zoom = max(p.Zoom-1, 0)
		}
	case ZoomFull:
		p.Zoom = ZoomSteps
	}
}
