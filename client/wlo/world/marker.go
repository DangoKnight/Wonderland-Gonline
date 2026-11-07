package world

import (
	"image"
	"math"
	"time"
	"wonderland-gonline/client/wlo/surface"
)

// Marker preserves FUN_0049bbf4's reachable destination and brief lifetime,
// drawing continuous geometry instead of the skin's three Arrow frames.
// Arrow-key walks do not show it.
type Marker struct {
	shown         bool
	at            image.Point
	pending       bool
	started       time.Time
	pixels        *image.NRGBA
	revision      uint64
	held          bool
	bob           float64
	releaseBob    float64
	pointer       image.Point
	followPointer bool
	spin          float64
	releaseSpin   float64
	prism         *image.NRGBA
}

const (
	markerDuration       = 600 * time.Millisecond
	markerMorphDuration  = 260 * time.Millisecond
	markerSize           = 88 // Leave headroom for the larger held bob and halo.
	markerCenter         = float64(markerSize) / 2
	markerGroundY        = 56.0
	markerDropDistance   = 22.0
	markerBobPeriod      = time.Second
	markerBobAmplitude   = 5.5
	markerPrismHalfWidth = 4.2
	markerSpinPeriod     = 1600 * time.Millisecond
	markerGroundAspect   = .42
)

// Show starts or restarts the marker at a scene point.
func (m *Marker) Show(at image.Point) {
	m.at = at
	if m.shown && m.held {
		return
	}
	m.shown, m.pending = true, true
	m.followPointer = false
	m.releaseBob = 0
	m.releaseSpin = 0
}

// SetHeld keeps the destination circle visible while dragging. Releasing begins
// the circle-to-curves transition from its initial geometry, without a jump.
func (m *Marker) SetHeld(held bool, now time.Time) {
	if m.held == held {
		return
	}
	if !held {
		m.releaseBob = markerBob(now.Sub(m.started))
		m.releaseSpin = markerSpin(now.Sub(m.started))
	}
	m.held = held
	m.started, m.pending = now, false
}

func markerBob(elapsed time.Duration) float64 {
	return markerBobAmplitude * math.Sin(2*math.Pi*float64(elapsed%markerBobPeriod)/float64(markerBobPeriod))
}

func markerSpin(elapsed time.Duration) float64 {
	return 2 * math.Pi * float64(elapsed%markerSpinPeriod) / float64(markerSpinPeriod)
}

// MoveHeld tracks the pointer without restarting the animation or walking clock.
func (m *Marker) MoveHeld(at image.Point) {
	if m.shown && m.held {
		m.at = at
	}
}
func (m *Marker) Hide()           { m.shown, m.held, m.followPointer = false, false, false }
func (m *Marker) Shown() bool     { return m.shown }
func (m *Marker) At() image.Point { return m.at }

// frame uses elapsed time so animation speed is independent of render FPS.
func (m *Marker) frame(now time.Time) bool {
	if !m.shown {
		return false
	}
	if m.pending {
		m.started, m.pending = now, false
	}
	elapsed := max(time.Duration(0), now.Sub(m.started))
	if !m.held && elapsed >= markerDuration {
		m.Hide()
		return false
	}
	progress := float64(elapsed) / float64(markerDuration)
	m.spin = markerSpin(elapsed)
	if m.held {
		progress = 0
		m.bob = markerBob(elapsed)
	} else {
		m.spin += m.releaseSpin
		m.bob = m.releaseBob * (1 - progress*progress*(3-2*progress))
	}
	m.render(progress)
	return true
}

func (m *Marker) render(progress float64) {
	if m.pixels == nil {
		m.pixels = image.NewNRGBA(image.Rect(0, 0, markerSize, markerSize))
	}
	// Keep the cyan prism, with a held ground circle that peels into four
	// curved strokes on release. Both ends of every stroke sink into the center.
	eased := progress * progress * (3 - 2*progress)
	needleY := markerGroundY - markerDropDistance*(1-eased) + m.bob
	m.renderPrism(needleY, max(.01, 1-eased))
	opacity := min(1.0, (1-progress)*6)
	morph := min(1.0, progress*float64(markerDuration)/float64(markerMorphDuration))
	morph = morph * morph * (3 - 2*morph)
	collapse := max(0.0, (progress-.25)/.75)
	collapse = collapse * collapse * (3 - 2*collapse)
	// Split a circle into four quarter arcs. Each arc's leading end sinks
	// inward; then the entire stroke contracts until it disappears at the center.
	var groundDistances [markerSize * markerSize]float64
	for i := range groundDistances {
		groundDistances[i] = 100
	}
	const curveSteps = 20
	for quarter := 0; quarter < 4; quarter++ {
		point := func(u float64) (float64, float64) {
			a := float64(quarter)*math.Pi/2 + u*math.Pi/2*(1-.25*morph)
			r := 23 * (1 - .92*morph*(1-u)) * (1 - collapse)
			return markerCenter + r*math.Cos(a), markerGroundY + r*markerGroundAspect*math.Sin(a)
		}
		ax, ay := point(0)
		for step := 1; step <= curveSteps; step++ {
			bx, by := point(float64(step) / curveSteps)
			for y := max(0, int(math.Floor(min(ay, by)-3))); y < min(markerSize, int(math.Ceil(max(ay, by)+3))); y++ {
				for x := max(0, int(math.Floor(min(ax, bx)-3))); x < min(markerSize, int(math.Ceil(max(ax, bx)+3))); x++ {
					dx, dy := bx-ax, by-ay
					t := max(0.0, min(1.0, ((float64(x)+.5-ax)*dx+(float64(y)+.5-ay)*dy)/(dx*dx+dy*dy)))
					d := math.Hypot(float64(x)+.5-ax-t*dx, float64(y)+.5-ay-t*dy) - .65
					i := y*markerSize + x
					groundDistances[i] = min(groundDistances[i], d)
				}
			}
			ax, ay = bx, by
		}
	}
	for y := 0; y < markerSize; y++ {
		for x := 0; x < markerSize; x++ {
			ground := groundDistances[y*markerSize+x]
			distance := ground
			coverage := max(0.0, min(1.0, .5-distance))
			halo := max(0.0, 1-max(0.0, distance)/2) * .22
			i := y*m.pixels.Stride + x*4
			m.pixels.Pix[i], m.pixels.Pix[i+1], m.pixels.Pix[i+2] = 40, 205, 230
			groundAlpha := max(coverage, halo)
			prismAlpha := float64(m.prism.Pix[i+3]) / 255
			alpha := prismAlpha + groundAlpha*(1-prismAlpha)
			if alpha > 0 {
				for channel := 0; channel < 3; channel++ {
					m.pixels.Pix[i+channel] = byte((float64(m.prism.Pix[i+channel])*prismAlpha + float64(m.pixels.Pix[i+channel])*groundAlpha*(1-prismAlpha)) / alpha)
				}
			}
			m.pixels.Pix[i+3] = byte(255 * opacity * alpha)
		}
	}
	m.revision++
}

func (w *World) drawMarker(cx, cy int, now time.Time) {
	w.syncHeldMarker(cx, cy)
	m := &w.Marker
	if !m.frame(now) {
		return
	}
	_ = w.Env.Screen.DrawSprite(m.at.X-cx-markerSize/2, m.at.Y-cy-int(markerGroundY),
		surface.DynamicSprite{Identity: m.pixels, Revision: m.revision}, func() (*image.NRGBA, error) { return m.pixels, nil })
}

// TrackHeldMarker retains screen coordinates so camera movement between input
// and drawing cannot displace the held cue. Walking still uses its own clock.
func (w *World) TrackHeldMarker(x, y int) {
	if !w.Marker.shown || !w.Marker.held {
		return
	}
	w.Marker.pointer, w.Marker.followPointer = image.Pt(x, y), true
	w.SyncHeldMarker()
}

// SyncHeldMarker also freezes release at the pointer's current scene position.
func (w *World) SyncHeldMarker() {
	cx, cy := w.Camera()
	w.syncHeldMarker(cx, cy)
}

func (w *World) syncHeldMarker(cx, cy int) {
	m := &w.Marker
	if !m.shown || !m.held || !m.followPointer {
		return
	}
	at := m.pointer.Add(image.Pt(cx, cy))
	if w.Scene.Walkable(at.X, at.Y) {
		m.MoveHeld(at)
	} else {
		w.MarkWalk()
	}
}

// MarkWalk keeps blocked clicks at the reachable route endpoint or player feet.
func (w *World) MarkWalk() {
	at := image.Pt(w.Player.X, w.Player.Y)
	if end, ok := w.walker.destination(); ok {
		at = end
	}
	w.Marker.Show(at)
}
