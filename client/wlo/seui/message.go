package seui

import (
	"image"
	"time"

	"wonderland-gonline/client/wlo/surface"
)

// Message presentation is shared by dialogue, timed notices and confirmations.
const (
	MessageOpenDuration        = 100 * time.Millisecond
	MessageHandoffDuration     = 80 * time.Millisecond
	MessageMotionInset         = 8
	MessageCloseDuration       = 50 * time.Millisecond
	messageClosingTravelPixels = 4
	messageExitLimit           = 16
)

func MessageTransitionSize(from, to image.Point, elapsed, duration time.Duration) image.Point {
	p := min(1, float64(max(0, elapsed))/float64(duration))
	p = p * (2 - p)
	return image.Pt(from.X+int(float64(to.X-from.X)*p), from.Y+int(float64(to.Y-from.Y)*p))
}

// MessageOpeningRect animates decoration only. Text and interactive controls
// retain their final positions and are usable from the first frame.
func MessageOpeningRect(target image.Rectangle, started, now time.Time) image.Rectangle {
	from := image.Pt(max(1, target.Dx()-2*MessageMotionInset), max(1, target.Dy()-2*MessageMotionInset))
	size := MessageTransitionSize(from, target.Size(), now.Sub(started), MessageOpenDuration)
	start := target.Min.Add(target.Size().Sub(size).Div(2))
	return image.Rectangle{Min: start, Max: start.Add(size)}
}

// PaintMessage keeps the control's layout/hit boxes stable while painting its
// opening frame. Work on a copy so child controls never inherit animated bounds.
func (p *Panel) PaintMessage(started, now time.Time) {
	frame := *p
	r := MessageOpeningRect(p.Rect(), started, now).Sub(p.ParentOrigin())
	frame.Left, frame.Top, frame.Width, frame.Height = r.Min.X, r.Min.Y, r.Dx(), r.Dy()
	frame.Paint()
}

type messageExit struct {
	owner   any
	image   *surface.Surface
	started time.Time
	now     func() time.Time
	overlay bool
}

// CloseMessage captures decoration and contents together on a compatible
// transparent target. It never reads GPU pixels or delays logical dismissal.
func (m *Manager) CloseMessage(owner any, now func() time.Time, paint func()) {
	m.CancelMessageExit(owner)
	if m.Env == nil || m.Env.Screen == nil {
		return
	}
	if len(m.messageExits) == messageExitLimit {
		m.messageExits[0].image.Close()
		m.messageExits = m.messageExits[1:]
	}
	target := m.Env.Screen.NewCompatible(m.Env.Screen.W, m.Env.Screen.H)
	func() {
		previous := m.Env.Screen
		m.Env.Screen = target
		defer func() { m.Env.Screen = previous }()
		paint()
	}()
	m.messageExits = append(m.messageExits, messageExit{owner: owner, image: target, started: now(), now: now})
}

// Overlay messages close in their original foreground pass, including the
// frame in which a timed notice expires. They must not vanish and reappear.
func (m *Manager) CloseOverlayMessage(owner any, now func() time.Time, paint func()) {
	m.CloseMessage(owner, now, paint)
	for i := range m.messageExits {
		if m.messageExits[i].owner == owner {
			m.messageExits[i].overlay = true
		}
	}
}

func (m *Manager) CancelMessageExit(owner any) {
	kept := m.messageExits[:0]
	for _, exit := range m.messageExits {
		if owner == nil || exit.owner == owner {
			exit.image.Close()
		} else {
			kept = append(kept, exit)
		}
	}
	clear(m.messageExits[len(kept):])
	m.messageExits = kept
}

func (m *Manager) updateMessageExits() {
	kept := m.messageExits[:0]
	for _, exit := range m.messageExits {
		if exit.now().Sub(exit.started) >= MessageCloseDuration {
			exit.image.Close()
		} else {
			kept = append(kept, exit)
		}
	}
	clear(m.messageExits[len(kept):])
	m.messageExits = kept
}

func (m *Manager) DrawMessageExits() {
	m.drawMessageExits(false)
}

func (m *Manager) DrawOverlayMessageExits() {
	m.drawMessageExits(true)
}

func (m *Manager) drawMessageExits(overlay bool) {
	m.updateMessageExits()
	for _, exit := range m.messageExits {
		if exit.overlay != overlay {
			continue
		}
		p := min(1, float64(max(0, exit.now().Sub(exit.started)))/float64(MessageCloseDuration))
		// Preserve native dither pixels and text. Rescaling the captured
		// checkerboard mask can alternate its coverage and look like blinking.
		travel := int(float64(messageClosingTravelPixels) * p * (2 - p))
		m.Env.Screen.Draw(0, -travel, exit.image, true)
	}
}

func paintMessageTree(c Control) {
	if !c.Base().Visible {
		return
	}
	c.Paint()
	for _, child := range c.Base().Children {
		paintMessageTree(child)
	}
}
