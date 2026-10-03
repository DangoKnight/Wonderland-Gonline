package world

import (
	"image"
	"strconv"
	"time"
)

// The walk marker (FUN_0049bbf4, drawn by the main loop after the map):
// a mouse walk (0x4a1d60's click and held-button branches) shows it at the
// walk's destination (+0x84, +0x88: the route's last waypoint, or the
// clicked point when no route was found) and restarts it. Every 120 ms it
// steps to the next picture, Arrow2, Arrow3, Arrow4 of the skin, and hides
// after Arrow4. A 64 × 64 cut is drawn centred on the destination. A step
// that comes within 120 ms of the previous one keeps the previous picture
// until the interval has passed, as the original does. Walks with the
// arrow keys show no marker.
type Marker struct {
	shown   bool // DAT_00828ac4
	stage   int  // DAT_00828ac5
	at      image.Point
	pic     int       // DAT_00828ad4
	changed time.Time // DAT_00828ac8
}

const (
	markerEvery     = 0x78 * time.Millisecond
	markerLast      = 4
	markerSize      = 0x40
	markerPicture   = "Arrow"
	markerNoPicture = -1
)

// Show starts the marker at a scene point.
func (m *Marker) Show(at image.Point) {
	m.shown, m.stage, m.at = true, 1, at
}

// Hide takes the marker down.
func (m *Marker) Hide() { m.shown = false }

// Shown reports whether the marker is up.
func (m *Marker) Shown() bool { return m.shown }

// At is the marker's scene point.
func (m *Marker) At() image.Point { return m.at }

// draw steps and paints the marker with the camera at (cx, cy).
func (w *World) drawMarker(cx, cy int, now time.Time) {
	m := &w.Marker
	if !m.shown {
		return
	}
	pics := w.Env.Pics
	if !m.changed.IsZero() && now.Sub(m.changed) <= markerEvery {
		// Not yet due: the current picture stays.
	} else if m.stage == markerLast {
		m.stage, m.shown = 1, false
		return
	} else {
		m.stage++
		m.pic = pics.Find(markerPicture + strconv.Itoa(m.stage))
		m.changed = now
	}
	if m.pic == markerNoPicture {
		return
	}
	pics.DrawRect(w.Env.Screen, m.pic, m.at.X-cx-markerSize/2, m.at.Y-cy-markerSize/2, image.Rect(0, 0, markerSize, markerSize), true)
}

// MarkWalk shows the marker for a mouse walk toward a scene point: at the
// planned route's end when routed, otherwise at the point itself, kept
// within the scene.
func (w *World) MarkWalk(x, y int, routed bool) {
	at := image.Pt(min(max(x, 0), w.Scene.Width-1), min(max(y, 0), w.Scene.Height-1))
	if end, ok := w.walker.destination(); ok && routed {
		at = end
	}
	w.Marker.Show(at)
}
