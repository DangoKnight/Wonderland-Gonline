package role

import (
	"strconv"
	"strings"
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/assets"
)

// Layered player drawing (FUN_00433318) as character selection uses it: a
// standing body with its head, equipment and costume in the character's
// colours, with a server-confirmed vehicle underneath in the world.
// The layer order is FUN_004465f0 (order.go).
const (
	equipSlots       = 6
	noSprite         = 1000                   // an item without a look for this body
	headSpriteOffset = 100                    // head N is sprite N + 1000·(type+1) + 100
	frameInterval    = 100 * time.Millisecond // FUN_00411f54(…, 100)
	// idleInterval is FUN_004122ec's other rate: a role at rest (+0x123
	// set, as the walk's end FUN_004126b8 and most standing paths do)
	// advances a frame every 230 ms instead of 100.
	idleInterval = 0xe6 * time.Millisecond
	// Standing actions are 8..15 (8 + facing); walking ones 0..7.
	standingFirst, standingLast = 8, 15
	layerPositions              = 7
	// Item record bytes (disk offsets of the client's object fields).
	itemFlagsOffset = 0x198 // +0x19c: 0x10 and 0x40 hide the head; also picks order variants
	itemHandOffset  = 0x19f // +0x1a3: the hand byte (order groups, costumes)
	hideHeadHard    = 0x10
	hideHeadSoft    = 0x40
	// Slot-6 item ID that hides every layer but the costume's.
	hiddenCostume = 0x5400
	// Costume item types (disk offset 15) drawn on layer 6.
	costumeBody      = 12
	costumeHat       = 13
	costumeBodyExtra = 16
)

// Equipment slots and their archive suffixes (switch in FUN_00433318).
var slotArchive = [equipSlots + 1]string{1: "c", 2: "e", 3: "w", 4: "a", 5: "s"}

// Human is one THuman object of the selection screen.
const vehicleSpriteFamily = "006"

type Human struct {
	vehicleSprite uint16
	vehicleSeated bool
	Lib           *Library
	Items         map[uint16]assets.NativeItem
	Now           func() time.Time

	body, head byte
	colors     Colors
	equip      [equipSlots + 1]uint16
	frame      int
	frameAt    time.Time
	lastAction int // a new action starts from its first frame
	// hold is a movie's frame (Hold), < 0 for the body's own clock.
	hold     int
	holdWrap bool
}

func NewHuman(lib *Library, items map[uint16]assets.NativeItem) *Human {
	return &Human{Lib: lib, Items: items, Now: time.Now, hold: -1}
}

// SetCharacter is the part of FUN_004013d0 that fills the body object:
// body type, head, colours (both values, groups 1 and 5; the portrait
// object copies the block) and the equipment by item slot.
func (h *Human) SetCharacter(c *login.CharacterSlot) {
	h.body, h.head = c.Body, c.Head
	h.colors = NeutralColors()
	h.colors.Set(int32(c.Color1), allParts, 1)
	h.colors.Set(int32(c.Color2), allParts, 5)
	h.equip = [equipSlots + 1]uint16{}
	for _, id := range c.Equipment[1:] {
		it, ok := h.Items[id]
		if !ok || id == 0 {
			continue
		}
		if s := it.Definition.EquipSlot; s >= 1 && s <= equipSlots {
			h.equip[s] = id
		}
	}
	h.frame, h.frameAt = 0, h.Now()
}

// familyName is the body type's archive prefix: 001 for type 0, then
// 002..005.
func familyName(body byte) string { return "00" + strconv.Itoa(int(body)+1) }

func baseID(body byte) int { return idsPerFamily * (int(body) + 1) }

// layerSprite draws one sprite of a family at the action and frame.
func (h *Human) layerSprite(dst *surface.Surface, base string, id int, action, frame, x, y int) {
	arc, key := h.Lib.lookup(base, id)
	if arc == nil {
		return
	}
	s := arc.sprite(key)
	if s == nil {
		return
	}
	n := s.frameCount(action)
	if n == 0 {
		return
	}
	if f := s.frame(action, frame%n); f != nil {
		// FUN_002fe8e8 adds 0x44 to the weapon archives before canvas placement.
		if strings.HasSuffix(base, "w") {
			y += weaponGroundOffset
		}
		s.drawColored(dst, f, x, y, id, &h.colors)
	}
}

func (h *Human) item(slot int) (assets.NativeItem, bool) {
	if h.equip[slot] == 0 {
		return assets.NativeItem{}, false
	}
	it, ok := h.Items[h.equip[slot]]
	return it, ok
}

// SetVehicle installs the Item.dat vehicle look from a server-confirmed mount.
func (h *Human) SetVehicle(sprite uint16) { h.vehicleSprite = sprite; h.vehicleSeated = false }

// SetVehiclePose distinguishes seated water riders from other vehicle classes.
func (h *Human) SetVehiclePose(sprite uint16, seated bool) {
	h.vehicleSprite = sprite
	h.vehicleSeated = seated && sprite != 0
}

// DrawBody is FUN_00412c50 → FUN_00433318 for a player: the base body,
// then seven layers in table order. direction is the action (+0x121).
func (h *Human) DrawBody(dst *surface.Surface, x, y int, direction int32) {
	if h.body == 0 || h.body > 4 {
		return
	}
	vehicleAction := int(direction)
	action := vehicleAction
	if h.vehicleSeated {
		action = vehicleRiderAction(direction)
	}

	fam := familyName(h.body)
	base := baseID(h.body)
	// The frame advances every 100 ms (230 ms standing) and wraps at the
	// base sprite's count; the falling group (26, 27) plays once and holds
	// its last frame, lying (FUN_004122ec with FUN_00427150 = 0x1a).
	if action != h.lastAction {
		h.lastAction, h.frame, h.frameAt = action, 0, h.Now()
	}
	if now := h.Now(); h.hold < 0 && now.Sub(h.frameAt) > intervalFor(action) {
		h.frameAt = now
		h.frame++
	}
	if arc, key := h.Lib.lookup(fam, base); arc != nil {
		if s := arc.sprite(key); s != nil && s.frameCount(action) > 0 {
			n := s.frameCount(action)
			switch {
			case h.hold >= 0 && h.holdWrap:
				h.frame = h.hold % n
			case h.hold >= 0:
				h.frame = min(h.hold, n-1)
			case fallAction(action):
				h.frame = min(h.frame, n-1)
			default:
				h.frame %= n
			}
		}
	}
	if h.vehicleSprite != 0 {
		if arc, key := h.Lib.lookup(vehicleSpriteFamily, int(h.vehicleSprite)); arc != nil {
			if s := arc.sprite(key); s != nil && s.frameCount(vehicleAction) > 0 {
				if f := s.frame(vehicleAction, h.frame%s.frameCount(vehicleAction)); f != nil {
					vehicleY := y
					if h.vehicleSeated && seatedVehicleGroundCorrection(h.vehicleSprite) {
						vehicleY += seatedVehicleGroundOffset
					}
					s.draw(dst, f, x, vehicleY)
				}
			}
		}
	}
	if h.vehicleSeated && h.vehicleSprite == raftVehicleSprite {
		offset := raftRiderOffsets[h.body-1][int(direction)%nativeFacingDirections]
		x, y = x+offset[0], y+offset[1]
	}
	h.layerSprite(dst, fam, base, action, h.frame, x, y)

	// The body sprite is the costume's when the slot-6 item is a costume
	// (FUN_00432c9c), otherwise the slot-2 armour's.
	costume := h.equip[equipSlots]
	costumeSprite := h.sprite(costume)
	category := costumeCategory(costumeSprite, costume, h.byteOf(costume, itemHandOffset))
	bodySprite := h.sprite(h.equip[2])
	if category != 0 {
		bodySprite = costumeSprite
	}
	// Items with flag 0x10 or 0x40 hide the head.
	showHead := true
	for slot := 1; slot <= equipSlots; slot++ {
		f := h.byteOf(h.equip[slot], itemFlagsOffset)
		if f&hideHeadHard != 0 {
			showHead = false
			break
		}
		if f&hideHeadSoft != 0 {
			showHead = false
		}
	}
	in := orderInput{Body: h.body, Action: byte(action), Frame: h.frame,
		Flags: h.byteOf(h.equip[2], itemFlagsOffset), Hand: h.byteOf(costume, itemHandOffset), Sprite: bodySprite}
	for pos := layerPositions; pos >= 1; pos-- {
		layer := layerAt(in, pos)
		if costume == hiddenCostume && layer != equipSlots {
			continue
		}
		if layer == 0 {
			if showHead {
				h.layerSprite(dst, fam+"h", base+headSpriteOffset+int(h.head), action, h.frame, x, y)
			}
			continue
		}
		// Native mounted poses hide hand weapons except type 6 (FUN_00433318).
		if layer == weaponSlot && h.vehicleSeated && h.typeOf(h.equip[layer]) != mountedVisibleWeaponType {
			continue
		}
		id := h.equip[layer]
		if id == 0 {
			continue
		}
		sprite := h.sprite(id)
		if sprite == noSprite {
			continue
		}
		// The visibility check FUN_00442c58 is not ported. The armour
		// extras (FUN_00448c8c, FUN_00448a8c) need the human's state
		// (+0xb1) to be 1 or 2; the selection's humans keep 0.
		if layer != equipSlots {
			h.layerSprite(dst, fam+slotArchive[layer], sprite, action, h.frame, x, y)
			continue
		}
		if category == 0 {
			continue
		}
		switch h.typeOf(id) {
		case costumeBody, costumeBodyExtra:
			h.layerSprite(dst, fam+slotArchive[2], bodySprite, action, h.frame, x, y)
		case costumeHat:
			h.layerSprite(dst, fam+slotArchive[1], sprite, action, h.frame, x, y)
		}
	}
}

// sprite is an item's sprite for the body type, 0 without an item.
func (h *Human) sprite(id uint16) int {
	if it, ok := h.Items[id]; ok && id != 0 && h.body >= 1 && h.body <= 4 {
		return int(it.Sprites[h.body-1])
	}
	return 0
}

// byteOf is a byte of an item's record, 0 without an item (record 0 of
// the table is empty).
func (h *Human) byteOf(id uint16, off int) byte {
	if it, ok := h.Items[id]; ok && id != 0 && off < len(it.Record) {
		return it.Record[off]
	}
	return 0
}

func (h *Human) typeOf(id uint16) byte {
	if it, ok := h.Items[id]; ok && id != 0 {
		return byte(it.Definition.Type)
	}
	return 0
}

// Portrait sprites (FUN_002586c8): the "f" archive of the body family,
// sprite head + 1000·(type+1) + 601, at the portrait object's action 2
// (set by FUN_004013d0, mapped by FUN_0042646c) with the blink state as
// the frame.
// Face sprites (<family>f, 601 + head): action 0 is the status panel's
// face (TSe_MainStatus sets +0x121 to 0), action 2 the selection's boxed
// portrait. Frame 1 has the eyes closed.
const (
	portraitSpriteOffset = 601
	portraitAction       = 2
	FaceAction           = 0
	// PortraitAction is the face action the talk window and character
	// selection use (+0x121 = 2).
	PortraitAction = portraitAction
)

// DrawPortrait is FUN_002586c8 for body types 1..4, as the character
// selection draws it.
func (h *Human) DrawPortrait(dst *surface.Surface, x, y int, blinking bool) {
	h.DrawFace(dst, x, y, portraitAction, blinking)
}

// FaceWidth is the width of the face sprite's first frame in an action
// (the library's +8 after FUN_002fe570), 0 when missing. The talk window
// places the large art (action 3) by it.
func (h *Human) FaceWidth(action int) int {
	if h.body == 0 || h.body > 4 {
		return 0
	}
	arc, key := h.Lib.lookup(familyName(h.body)+"f", baseID(h.body)+portraitSpriteOffset+int(h.head))
	if arc == nil {
		return 0
	}
	s := arc.sprite(key)
	if s == nil || s.frameCount(action) == 0 {
		return 0
	}
	if f := s.frame(action, 0); f != nil {
		return f.Width
	}
	return 0
}

// BodyWidth is the width of the base body sprite's first frame in an
// action: FUN_00437cd4 (the library's +8 after loading the body sprite),
// which the talk window's wide form uses to place the large art.
func (h *Human) BodyWidth(action int) int {
	if h.body == 0 || h.body > 4 {
		return 0
	}
	arc, key := h.Lib.lookup(familyName(h.body), baseID(h.body))
	if arc == nil {
		return 0
	}
	s := arc.sprite(key)
	if s == nil || s.frameCount(action) == 0 {
		return 0
	}
	if f := s.frame(action, 0); f != nil {
		return f.Width
	}
	return 0
}

// TalkArtAction is the face sprite's large art, drawn by the talk window's
// wide form (FUN_0034d990 sets +0x121 to 3).
const TalkArtAction = 3

// DrawFace is FUN_002586c8 with the role's action (+0x121).
func (h *Human) DrawFace(dst *surface.Surface, x, y, action int, blinking bool) {
	if h.body == 0 || h.body > 4 {
		return
	}
	frame := 0
	if blinking {
		frame = 1
	}
	h.DrawFaceFrame(dst, x, y, action, frame)
}

// DrawFaceFrame draws one frame of the face sprite's action (+0x121, the
// frame +0x11e): the chat log's speaker icon is action 4's second frame,
// the head turned three-quarters.
func (h *Human) DrawFaceFrame(dst *surface.Surface, x, y, action, frame int) {
	if h.body == 0 || h.body > 4 {
		return
	}
	h.layerSprite(dst, familyName(h.body)+"f", baseID(h.body)+portraitSpriteOffset+int(h.head), action, frame, x, y)
}

// FaceHit reports whether (px, py) falls on an opaque pixel of the face
// frame DrawFaceFrame draws at (x, y): the sprite draw's own hit test,
// which sets the role's +0x80 for the chat log's speaker icons.
func (h *Human) FaceHit(x, y, action, frame, px, py int) bool {
	if h.body == 0 || h.body > 4 {
		return false
	}
	arc, key := h.Lib.lookup(familyName(h.body)+"f", baseID(h.body)+portraitSpriteOffset+int(h.head))
	if arc == nil {
		return false
	}
	s := arc.sprite(key)
	if s == nil {
		return false
	}
	n := s.frameCount(action)
	if n == 0 {
		return false
	}
	f := s.frame(action, frame%n)
	if f == nil {
		return false
	}
	pixels, err := f.Image(s.m)
	if err != nil || pixels == nil {
		return false
	}
	cx, cy := px-x-f.OffsetX, py-y-f.OffsetY
	if cx < 0 || cy < 0 || cx >= f.Width || cy >= f.Height {
		return false
	}
	b := pixels.Bounds()
	return pixels.NRGBAAt(b.Min.X+cx, b.Min.Y+cy).A != 0
}

// Hold draws a movie's frame: a keyframe's fixed frame (clamped to the
// action's last) or, with wrap, the actor's animation counter. A negative
// frame returns the body to its own clock.
func (h *Human) Hold(frame int, wrap bool) { h.hold, h.holdWrap = frame, wrap }

// fallAction reports the falling group, which holds its last frame.
func fallAction(action int) bool { return action == fallFirst || action == fallFirst+1 }

const fallFirst = 0x1a

// intervalFor is FUN_004122ec's frame interval for an action.
func intervalFor(action int) time.Duration {
	if action >= standingFirst && action <= standingLast {
		return idleInterval
	}
	return frameInterval
}

// FUN_00445950: water vehicle riders use directional seated actions 46..53.
// The vehicle itself retains walking/standing actions 0..15.
const waterRiderActionBase = 46
const nativeFacingDirections = 8

func vehicleRiderAction(direction int32) int {
	return waterRiderActionBase + int(direction)%nativeFacingDirections
}

// FUN_002fe8e8 and FUN_00154d20: water vehicle canvases share the weapon
// ground correction. These are renderer offsets, not changes to exported PNGs.
const (
	weaponGroundOffset        = 0x44
	seatedVehicleGroundOffset = 0x44
	raftVehicleSprite         = 6005
	weaponSlot                = 3
	mountedVisibleWeaponType  = 6
)

// FUN_00154100, vehicle kind 4: X table at 0x4baafc and Y at 0x4babb0.
// Each row is a body type; columns are the eight native seated directions.
var raftRiderOffsets = [4][8][2]int{
	{{0, -12}, {-10, -7}, {-17, -2}, {-10, 8}, {0, 13}, {10, 8}, {17, -2}, {10, -7}},
	{{0, -12}, {-10, -7}, {-17, -2}, {-10, 8}, {0, 13}, {10, 8}, {17, -2}, {10, -7}},
	{{0, -12}, {-10, -2}, {-17, -2}, {-5, 8}, {0, 8}, {5, 8}, {17, -2}, {10, -2}},
	{{0, -12}, {-10, -7}, {-17, -2}, {-10, 8}, {0, 13}, {10, 8}, {17, -2}, {10, -7}},
}

// Native vehicle classes 4,5,7,10,12,15 have +68 canvas correction.
// These WLRI look IDs come from Item.dat; submarine class 16 has no correction.
func seatedVehicleGroundCorrection(sprite uint16) bool {
	switch sprite {
	case 6005, 6006, 6009, 6015, 6016, 6023:
		return true
	}
	return false
}
