package login

import (
	"time"

	"wonderland-go/client/wlo/seui"
	"wonderland-go/client/wlo/surface"
)

// Colour block size and its neutral digit (THuman +0xd5, FUN_00012d10(…,
// 0x2d, 4)).
const (
	ColorDigits  = 0x2d
	NeutralDigit = 4
)

// CreateHuman is the part of a THuman object (constructor FUN_00410e04)
// that character creation edits and TJK_RoleImage draws.
type CreateHuman struct {
	Type    byte              // +0x08 body type 1..4
	Head    byte              // +0xb4
	Look    uint16            // +0xb6
	Action  byte              // +0x121
	Colors  [ColorDigits]byte // +0xd5
	Edit    [3]int            // +0x104, +0x108, +0x10c: R, G, B digits being edited
	Items   [7]uint16         // item IDs by equipment slot 1..6 (+0x64 + slot·4, +4)
	Element byte              // +0x1f82
	// Frame is the animation frame (+0x11e) and FrameAt its last step,
	// kept by the RolePainter.
	Frame   int
	FrameAt time.Time
	// X, Y are the drawing point (+0x8c, +0x90).
	X, Y int
}

// NeutralColors is FUN_00012d10(+0xd5, 0x2d, 4).
func (h *CreateHuman) NeutralColors() {
	for i := range h.Colors {
		h.Colors[i] = NeutralDigit
	}
}

// RolePainter draws the creation sprites; the character renderer
// (client/wlo/role) implements it.
type RolePainter interface {
	// EquipSlot is FUN_003cdc98: the equipment slot of an item, or 0.
	EquipSlot(item uint16) byte
	// PaintBody is TJK_RoleImage's sprite drawing (FUN_002566bc) at the
	// human's drawing point.
	PaintBody(dst *surface.Surface, h *CreateHuman)
	// PaintPortrait is FUN_002586c8 with the blink state.
	PaintPortrait(dst *surface.Surface, h *CreateHuman, blinking bool)
}

// RoleImage is TJK_RoleImage (constructor FUN_002565e0): a body THuman
// (+0xc4) and a portrait THuman (+0xc8) drawn on a GrBasic.
type RoleImage struct {
	seui.GrBasic
	Body     CreateHuman // +0xc4
	Portrait CreateHuman // +0xc8
	Hovered  bool        // +0xc0
	// ShowPortrait (+0xd4) also draws the portrait.
	ShowPortrait bool
	Blinking     bool // the portrait's +0x11e
	Painter      RolePainter
}

func NewRoleImage(env *seui.Env, owner seui.Control) *RoleImage {
	r := &RoleImage{}
	seui.InitGrBasic(&r.GrBasic, r, env, owner)
	r.Body.Action, r.Body.Type = 4, 1
	r.Portrait.Action, r.Portrait.Type = 0, 0
	return r
}

// Paint is slot +0x10 (FUN_002566bc): the body sits at the bottom centre,
// then the portrait when shown.
func (r *RoleImage) Paint() {
	r.GrBasic.Paint()
	a := r.Abs()
	r.Body.X = a.X + int(uint32(r.Width)>>1) - 2
	r.Body.Y = a.Y + r.Height - 8
	if r.Painter == nil {
		return
	}
	r.Painter.PaintBody(r.Env.Screen, &r.Body)
	if r.ShowPortrait {
		r.Painter.PaintPortrait(r.Env.Screen, &r.Portrait, r.Blinking)
	}
}

// Update is slot +0x18 (FUN_002584d0).
func (r *RoleImage) Update(in *seui.Input) {
	r.GrBasic.Update(in)
	r.Hovered = in.Hovered == seui.Control(r)
}
