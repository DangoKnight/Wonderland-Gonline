// Package hud is the in-game interface drawn over the world: the status
// panel (TSe_MainStatus) so far.
package hud

import (
	"image"
	"math/rand"
	"time"

	"wonderland-go/client/wlo/login"
	"wonderland-go/client/wlo/role"
	"wonderland-go/client/wlo/seui"
	"wonderland-go/client/wlo/surface"
	"wonderland-go/client/wlo/world"
)

// MainStatus layout (constructor FUN_0025ff84, painter FUN_00260fac):
// offsets from the panel's corner, which starts at (2, 10) (+0x160,
// +0x164; the slide-in of FUN_00260b90 is not ported).
var (
	statusCorner = image.Pt(2, 10)
	elementAt    = image.Pt(0x71, 7)    // +0x178, an Icon_Element_<e>_1 button
	hpGaugeAt    = image.Pt(0x20, 0x25) // +0x1a4 Main_Hp
	spGaugeAt    = image.Pt(0x15, 0x26) // +0x1ac Main_Sp
	levelAt      = image.Pt(0, -6)      // +0x1b4
	hpAt         = image.Pt(5, 0x7b)    // +0x1bc
	spAt         = image.Pt(5, 0x8b)    // +0x1c4
	expAt        = image.Pt(5, 0x9b)    // +0x1cc
	goldAt       = image.Pt(5, 0xaa)    // +0x1d4
	jobAt        = image.Pt(3, 0xd)     // +0x1dc
	portraitAt   = image.Pt(0x51, 0x32) // +0x1e8, the portrait role
)

const (
	statusPicture  = "Main_Status_1"
	hpGaugePicture = "Main_Hp"
	spGaugePicture = "Main_Sp"
	fullGauge      = 100
	// The portrait blinks for 50 ms every 6 to 8 seconds (+0x2f8, +0x308).
	blinkEvery  = 6000 * time.Millisecond
	blinkSpread = 2000
	blinkFor    = 50 * time.Millisecond
)

// faceDrawer draws a role's face sprite in an action (role.Human).
type faceDrawer interface {
	DrawFace(dst *surface.Surface, x, y, action int, blinking bool)
}

// MainStatus is TSe_MainStatus, a fixed form shown over the world.
type MainStatus struct {
	seui.FixedForm
	Status   *login.Status
	Stats    *world.Stats
	Portrait login.RoleView // the panel's role object (+0x1e4), drawn with FUN_002586c8
	Now      func() time.Time

	Element *seui.ElementIcon // +0x178
	Recover *seui.FixedButton // +0x318 Btn_Allrecovery_1
	Discuss *seui.FixedButton // +0x314 Btn_Discussion_1, hidden outside its mode

	background, hpGauge, spGauge int
	element                      byte
	nextBlink, blinkEnd          time.Time
}

// Buttons of the panel (constructor FUN_0025ff84), in form coordinates.
const (
	statusButtonSize = 0x16
	statusButtonTop  = 0x6a
	discussLeft      = 0x10
	recoverLeft      = 0x27
	recoverHint      = "Restore HP/SP using Potions and Bottles"
	elementSize      = 0x19
)

// NewMainStatus is FUN_0025ff84: the pictures, the element icon and the
// buttons. The panel starts hidden; entering the world shows it.
func NewMainStatus(env *seui.Env, f *login.Formula, stats *world.Stats) *MainStatus {
	p := env.Pics
	m := &MainStatus{Status: login.NewStatus(env, f), Stats: stats, Now: time.Now,
		background: p.Find(statusPicture), hpGauge: p.Find(hpGaugePicture), spGauge: p.Find(spGaugePicture)}
	m.InitFixedForm(m, env)
	m.Name = "TSe_MainStatus"
	m.Draggable = false
	e := statusCorner.Add(elementAt)
	m.Element = seui.NewElementIcon(env, m)
	m.Element.Init("", 0, 0, 0, 0, 0, false, elementSize, elementSize, 0)
	m.Element.SetElement(0, e.X, false, 0, nil, -1, -1, e.Y)
	m.Discuss = seui.NewFixedButton(env, m)
	m.Discuss.Init("Btn_Discussion_1", discussLeft, statusButtonSize, statusButtonSize, 0, 0, true, statusButtonSize, statusButtonSize, statusButtonTop)
	m.Discuss.SetVisible(false)
	m.Recover = seui.NewFixedButton(env, m)
	m.Recover.Init("Btn_Allrecovery_1", recoverLeft, statusButtonSize, statusButtonSize, 0, 0, true, statusButtonSize, statusButtonSize, statusButtonTop)
	m.Recover.SetHint([]byte(recoverHint))
	return m
}

// percent is FUN_00261588's fill: the share of the maximum, at least 1
// while anything is left, 100 without a maximum.
func percent(v, maximum uint32) int {
	if maximum == 0 {
		return fullGauge
	}
	p := int(uint64(v) * fullGauge / uint64(maximum))
	if p == 0 && v != 0 {
		p = 1
	}
	return min(p, fullGauge)
}

// blinking keeps the portrait's blink clock.
func (m *MainStatus) blinking() bool {
	now := m.Now()
	if m.nextBlink.IsZero() {
		m.nextBlink = now.Add(blinkEvery + time.Duration(rand.Intn(blinkSpread))*time.Millisecond)
	}
	if !now.Before(m.nextBlink) {
		m.blinkEnd = now.Add(blinkFor)
		m.nextBlink = now.Add(blinkEvery + time.Duration(rand.Intn(blinkSpread))*time.Millisecond)
	}
	return now.Before(m.blinkEnd)
}

// Paint is FUN_00260fac. The element icon and buttons are children and
// draw after it.
func (m *MainStatus) Paint() {
	s, scr, pics := m.Stats, m.Env.Screen, m.Env.Pics
	at := func(p image.Point) image.Point { return statusCorner.Add(p) }
	c := at(image.Point{})
	pics.Draw(scr, m.background, c.X, c.Y, true)
	if s.Element != m.element {
		m.element = s.Element
		m.Element.SetElement(s.Element, -1, false, 0, nil, -1, -1, -1)
	}
	g := at(hpGaugeAt)
	pics.DrawGauge(scr, m.hpGauge, g.X, g.Y, percent(s.HP, s.MaxHP))
	g = at(spGaugeAt)
	pics.DrawGauge(scr, m.spGauge, g.X, g.Y, percent(uint32(s.SP), uint32(s.MaxSP)))
	if f, ok := m.Portrait.(faceDrawer); ok {
		p := at(portraitAt)
		f.DrawFace(scr, p.X, p.Y, role.FaceAction, m.blinking())
	}
	st := m.Status
	st.Level(s.Level, at(levelAt), true)
	st.Job(s.Job, at(jobAt), true, false)
	st.HP(s.HP, s.MaxHP, at(hpAt), true)
	st.SP(s.SP, s.MaxSP, at(spAt), true)
	st.Exp(s.Level, s.EXP, s.Rebirth != 0, at(expAt), true)
	st.Gold(s.Gold, at(goldAt), true)
}
