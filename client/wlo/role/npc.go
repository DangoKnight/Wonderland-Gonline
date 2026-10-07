package role

import (
	"image"
	"time"

	"wonderland-gonline/client/wlo/sprites"
	"wonderland-gonline/client/wlo/surface"
)

// NPC sprites. An NPC object keeps body type 0, so FUN_00433318 draws one
// sprite of the 001 family, 1000 + its look (+0xb6), in the action +0x121
// and the object's colours.
const (
	npcFamily     = "001"
	npcSpriteBase = 1000
	npcLookRange  = 1000 // FUN_004265a4 keeps the template's look modulo 1000
	npcColorParts = 4    // template colour values fill groups 1..4
	// Talk faces (FUN_002586c8, body 0): the template's face sprite
	// (Npc.dat +0x58) from the 008 family, action 0, frame 1 blinking.
	npcFaceFamily = "008"
	npcFaceAction = 0
)

// NPC draws one map NPC.
type NPC struct {
	Lib *Library
	Now func() time.Time

	sprite int
	colors Colors
	// Lit draws the NPC highlighted, as while the pointer is over it.
	Lit         bool
	heightScale byte
	frame       int
	frameAt     time.Time
	hold        int  // the fixed frame, < 0 to animate
	wrap        bool // hold counts frames that wrap at the action's count
}

// Hold fixes the drawn frame: a prop's +0x11f or a movie keyframe's frame
// (clamped to the action's last), or with wrap a movie actor's animation
// counter. A negative frame animates by the NPC's own clock.
func (n *NPC) Hold(frame int, wrap bool) { n.hold, n.wrap = frame, wrap }

// shown is the frame drawn of count frames.
func (n *NPC) shown(count int) int {
	switch {
	case n.hold >= 0 && n.wrap:
		return n.hold % count
	case n.hold >= 0:
		return min(n.hold, count-1)
	}
	return n.frame % count
}

// NewNPC is FUN_004265a4's appearance part: the template's look and its
// four colour values over a neutral block.
func NewNPC(lib *Library, look uint16, colors [npcColorParts]uint32) *NPC {
	n := &NPC{Lib: lib, Now: time.Now, sprite: npcSpriteBase + int(look)%npcLookRange, colors: NeutralColors(), hold: -1}
	for i, v := range colors {
		n.colors.Set(int32(v), allParts, i+1)
	}
	n.frameAt = n.Now()
	return n
}

// SetLit turns the hover highlight on or off.
func (n *NPC) SetLit(on bool) { n.Lit = on }

// Sprite is the drawn sprite ID.
func (n *NPC) Sprite() int { return n.sprite }

// Draw draws the NPC's feet at (x, y). The frame advances every 100 ms
// (230 ms standing) and wraps at the action's frame count, as for players,
// unless a frame is held (FUN_0030120c's frame argument).
func (n *NPC) Draw(dst *surface.Surface, x, y, action int) {
	arc, key := n.Lib.lookup(npcFamily, n.sprite)
	if arc == nil {
		return
	}
	s := arc.sprite(key)
	if s == nil {
		return
	}
	count := s.frameCount(action)
	if count == 0 {
		return
	}
	if now := n.Now(); now.Sub(n.frameAt) > intervalFor(action) {
		n.frameAt = now
		n.frame++
	}
	n.frame %= count
	if f := s.frame(action, n.shown(count)); f != nil {
		colors := &n.colors
		if n.Lit {
			lit := n.colors.lifted(litSteps)
			colors = &lit
		}
		s.drawColoredScaled(dst, f, x, y, n.sprite, colors, n.rasterScale())
	}
}

// Bounds is the screen rectangle of the frame Draw shows at (x, y); empty
// while the sprite is not available.
func (n *NPC) Bounds(x, y, action int) image.Rectangle {
	arc, key := n.Lib.lookup(npcFamily, n.sprite)
	if arc == nil {
		return image.Rectangle{}
	}
	s := arc.sprite(key)
	if s == nil {
		return image.Rectangle{}
	}
	count := s.frameCount(action)
	if count == 0 {
		return image.Rectangle{}
	}
	f := s.frame(action, n.shown(count))
	if f == nil {
		return image.Rectangle{}
	}
	scale := n.rasterScale()
	return image.Rect(0, 0, f.Width*scale, f.Height*scale).Add(image.Pt(x+f.OffsetX*scale, y+f.OffsetY*scale))
}

// FrameSize is the size of the action's first frame (FUN_002fe570 leaves
// it in the library's +8 and +0xc); zero while the sprite is missing.
func (n *NPC) FrameSize(action int) (w, h int) {
	arc, key := n.Lib.lookup(npcFamily, n.sprite)
	if arc == nil {
		return 0, 0
	}
	s := arc.sprite(key)
	if s == nil || s.frameCount(action) == 0 {
		return 0, 0
	}
	if f := s.frame(action, 0); f != nil {
		return f.Width, f.Height
	}
	return 0, 0
}

// DrawFace draws the talk window's face sprite with the NPC's colours.
func (n *NPC) DrawFace(dst *surface.Surface, x, y int, face uint16, blinking bool) {
	if face == 0 {
		return
	}
	arc, key := n.Lib.lookup(npcFaceFamily, int(face))
	if arc == nil {
		return
	}
	s := arc.sprite(key)
	if s == nil {
		return
	}
	count := s.frameCount(npcFaceAction)
	if count == 0 {
		return
	}
	frame := 0
	if blinking {
		frame = 1 % count
	}
	if f := s.frame(npcFaceAction, frame); f != nil {
		s.drawColored(dst, f, x, y, int(face), &n.colors)
	}
}

// FirstAnchorY is the anchor Y of the sprite's first frame, which
// FUN_002fe570 leaves in the library (+0x3c) when the template is applied;
// false while the sprite is not available.
func (n *NPC) FirstAnchorY() (int, bool) {
	arc, key := n.Lib.lookup(npcFamily, n.sprite)
	if arc == nil {
		return 0, false
	}
	s := arc.sprite(key)
	if s == nil || len(s.Frames) == 0 {
		return 0, false
	}
	return s.Frames[0].AnchorY, true
}

// mountFrame supplies native saddle geometry. FixedFirst affects placement
// only: the mount itself continues to animate normally.
func (n *NPC) mountFrame(action int, first bool) *sprites.Frame {
	arc, key := n.Lib.lookup(npcFamily, n.sprite)
	if arc == nil {
		return nil
	}
	s := arc.sprite(key)
	if s == nil || s.frameCount(action) == 0 {
		return nil
	}
	frame := n.shown(s.frameCount(action))
	if first {
		frame = 0
	}
	return s.frame(action, frame)
}

// Npc.dat +0x38 (memory +0x3c) selects the native 2x map raster scale.
// Portraits remain at their original scale.
func (n *NPC) SetHeightScale(scale byte) { n.heightScale = scale }
func (n *NPC) rasterScale() int {
	if n.heightScale == 1 {
		return 2
	}
	return 1
}

// DrawIcon uses the dedicated small portrait referenced by Npc.dat +0x10,
// copied to +0xb2 by FUN_004265a4. It is 007-family artwork, separate from
// the 008-family dialogue portrait. (x,y) is the icon's top-left corner.
func (n *NPC) DrawIcon(dst *surface.Surface, x, y int, icon uint16) {
	if icon == 0 {
		return
	}
	const smallPortraitFamily = "007"
	arc, key := n.Lib.lookup(smallPortraitFamily, int(icon))
	if arc == nil {
		return
	}
	s := arc.sprite(key)
	if s == nil || s.frameCount(0) == 0 {
		return
	}
	if frame := s.frame(0, 0); frame != nil {
		s.drawColored(dst, frame, x-frame.OffsetX, y-frame.OffsetY, int(icon), &n.colors)
	}
}
