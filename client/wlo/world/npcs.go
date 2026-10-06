package world

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"os"
	"sync"
	"wonderland-gonline/internal/clientruntime"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/surface"
	native "wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientassets"
)

// Map NPCs. The map loader (FUN_003090f4 → FUN_00484a50) reads the map's
// NPC records from eve.Emg; FUN_003030ec then creates one character object
// per record, applies its Npc.dat template (FUN_004265a4) and stands it at
// the record's position, facing and visibility. The server later moves,
// shows or hides them with AC22:4 (FUN_0038cf30).
const (
	npcExport = "npc_data.json"
	// Facing: records store 1..8, the object keeps (9 - r) mod 8
	// (FUN_00484a50); FUN_00430474(…, 8) then stands it, action 8 + facing.
	facingCount    = 8
	standingAction = 8
	// Record flag bit 0: shown at load (+0x139), otherwise hidden (+0x138).
	npcFlagShown = 1 << 0
	// AC22:4 visibility kinds (+0x1eec).
	actorShown  = 1
	actorHidden = 2
	// AC22:4 entries: click ID, state, X, Y, kind, duration, action.
	actorRecordBytes = 14
	actorStateOffset = 2
	// frameFree is the state that animates a prop (+0x11f = 0xff).
	frameFree = 0xff
)

// Template kinds (Npc.dat +0x0b). Props are FUN_00482424's kinds: AC22:4
// sets their frame (+0x11f) from its state, so a chest stays closed (0)
// or open (1). The name overlay (FUN_004147f8) skips the unnamed kinds.
const (
	kindProp     = 6
	kindUnnamedA = 8
	kindObject   = 9
	kindObjectB  = 10
)

// Prop reports FUN_00482424: the template's sprite shows a fixed frame.
func (t NPCTemplate) Prop() bool {
	return t.Kind == kindProp || t.Kind == kindObject || t.Kind == kindObjectB
}

// Unnamed reports the kinds whose names are not drawn (FUN_004147f8, while
// the player's +0x1f16 is 0).
func (t NPCTemplate) Unnamed() bool {
	return t.Kind == kindProp || t.Kind == kindUnnamedA || t.Kind == kindObject
}

// Sprite drop (FUN_002fe8e8): templates on the tall name base (+0x5b = 1)
// draw their sprite 0x44 lower, and 0x20 more on the doubled scale
// (+0x3c = 1); the name and shadow stay at the feet.
const (
	spriteDropTall   = 0x44
	spriteDropScaled = 0x20
)

// SpriteDrop is how far below the feet the sprite is drawn.
func (t NPCTemplate) SpriteDrop() int {
	if t.HeightPreset != 1 {
		return 0
	}
	if t.HeightScale == 1 {
		return spriteDropTall + spriteDropScaled
	}
	return spriteDropTall
}

// NPCTemplate is the part of an Npc.dat record the map view uses, as
// FUN_004265a4 copies it (memory offsets are disk offsets + 4): the name
// (+0x08, drawn from the object's +9), the kind (+0x0f, the object's
// +0x4e), the look (+0x12), four colour values (+0x16..+0x22), the shadow
// kind (+0x4e) and the two bytes that choose the name height and the
// sprite's drop (+0x3c, +0x5b), and the talk window's face sprite (+0x5c,
// FUN_002586c8).
type NPCTemplate struct {
	Element      byte
	Skills       [3]uint16
	Name         string
	Kind         byte
	Look         uint16
	Face         uint16
	TalkLow      byte // +0x5a: 1 stands the talk window's body lower
	Colors       [4]uint32
	Shadow       byte
	HeightScale  byte
	HeightPreset byte
	// Sound is the wav#### a prop plays as it opens (+0x60, FUN_00408054).
	Sound uint16
}

// NPCPainter draws an NPC's sprite at its feet.
type NPCPainter interface {
	Draw(dst *surface.Surface, x, y, action int)
	// FirstAnchorY is the first frame's anchor Y, false until loaded.
	FirstAnchorY() (int, bool)
	// Bounds is the screen rectangle Draw covers at (x, y).
	Bounds(x, y, action int) image.Rectangle
}

// Name height (+0x114, FUN_004265a4): a base minus the first frame's
// anchor Y, doubled for HeightScale 1. Some NPC IDs on the 0xa6 base have
// fixed heights from a table (FUN_0018b544 and others) that is not
// ported.
const (
	nameHeightBase      = 0x6a
	nameHeightTallBase  = 0xa6
	nameHeightTallLimit = 0x32 // below it the name sits 0x19 higher, else 10
	nameLiftLow         = 0x19
	nameLiftTall        = 10
	npcNameInk          = 0xffff // the NPC class's name colour (0x4154fe)
	npcNameStyle        = 2
)

// nameHeight is +0x114 for a template whose sprite's first frame has the
// given anchor Y.
func nameHeight(t NPCTemplate, anchorY int) int {
	if t.HeightScale > 1 {
		return 0
	}
	base := nameHeightBase
	if t.HeightPreset == 1 {
		base = nameHeightTallBase
	} else if t.HeightPreset != 0 {
		return 0
	}
	h := max(base-anchorY, 0)
	if t.HeightScale == 1 {
		h *= 2
	}
	return h
}

// nameTop is the name's Y relative to the feet (0x41489e, 0x415fca).
func nameTop(height int) int {
	if height < nameHeightTallLimit {
		return -height - nameLiftLow
	}
	return -height - nameLiftTall
}

// Ground shadows (FUN_0030120c, 0x301d97): the template's shadow kind
// picks a cut of the Shadow picture or the whole Monster_Shadow picture.
const (
	shadowSmall   = 0
	shadowMonster = 1
	// ShadowPicture and MonsterShadowPicture name the pictures in pic\images.BMg.
	ShadowPicture        = "Shadow"
	MonsterShadowPicture = "Monster_Shadow"
)

var (
	smallShadowRect            = image.Rect(0x31, 0x58, 0x50, 0x69)
	smallShadowX, smallShadowY = -0xf, -4
	monsterShadowX             = -0x2f
	monsterShadowY             = -0x12
)

// NPC is one map NPC object.
type NPC struct {
	ClickID  uint16
	Template uint32
	Info     NPCTemplate
	X, Y     int
	Action   int
	Shown    bool
	Painter  NPCPainter
	// Fixed holds a prop on Frame (+0x11f); otherwise the sprite animates.
	// With Wrap, Frame is a movie actor's animation counter.
	Fixed, Wrap bool
	Frame       int
	// Depth moves the NPC's place among the depth-sorted figures (record
	// +0x1180): a coconut up a palm sorts in front of the palm.
	Depth int
}

// SortY is the NPC's key in the depth-sorted list (FUN_0041d13c).
func (n *NPC) SortY() int { return n.Y + n.Depth }

// recordDepth is the eve record's sort offset, the int the loader
// (FUN_00484a50) copies from +0x6b4 to +0x1180: the last two of the
// record's trailing words.
func recordDepth(w [4]uint16) int { return int(int32(uint32(w[2]) | uint32(w[3])<<16)) }

// paint hands the NPC's fixed frame to its painter.
func (n *NPC) paint() {
	if h, ok := n.Painter.(interface{ Hold(int, bool) }); ok {
		if n.Fixed {
			h.Hold(n.Frame, n.Wrap)
		} else {
			h.Hold(-1, false)
		}
	}
}

// standFacing converts a record's facing to its standing action.
func standFacing(r byte) int {
	if r < 1 || r > facingCount {
		return standingAction // the reader leaves +0xd44 at 0
	}
	return standingAction + int(facingCount+1-int(r))%facingCount
}

var npcData struct {
	sync.Mutex
	path      string
	templates map[uint32]NPCTemplate
	evePath   string
	maps      map[uint16]eveMap
}

type eveMap struct {
	scene uint16
	hex   string
}

// NPCTemplates reads the Npc.dat export, once.
func NPCTemplates(a login.Assets) (map[uint32]NPCTemplate, error) {
	npcData.Lock()
	defer npcData.Unlock()
	path := a.DataPath(npcExport)
	if npcData.path == path {
		return npcData.templates, nil
	}
	var doc struct {
		Records []struct {
			Name struct {
				Text string `json:"text"`
			} `json:"name"`
			Fields struct {
				ID           uint32 `json:"id"`
				Element      byte   `json:"element"`
				Skill1       uint16 `json:"skill_1"`
				Skill2       uint16 `json:"skill_2"`
				Skill3       uint16 `json:"skill_3"`
				Kind         byte   `json:"type"`
				Look         uint16 `json:"unknown_u16_offset_14"`
				Color1       uint32 `json:"unknown_u32_offset_18"`
				Color2       uint32 `json:"unknown_u32_offset_22"`
				Color3       uint32 `json:"unknown_u32_offset_26"`
				Color4       uint32 `json:"unknown_u32_offset_30"`
				HeightScale  byte   `json:"unknown_u8_offset_56"`
				Shadow       byte   `json:"unknown_u8_offset_74"`
				HeightPreset byte   `json:"unknown_u8_offset_87"`
				Face         uint16 `json:"unknown_u16_offset_88"`
				TalkLow      byte   `json:"unknown_u8_offset_86"`
				Sound        uint16 `json:"unknown_u16_offset_92"`
			} `json:"fields"`
		} `json:"records"`
	}
	if err := readJSON(path, &doc); err != nil {
		return nil, err
	}
	out := make(map[uint32]NPCTemplate, len(doc.Records))
	for _, r := range doc.Records {
		f := r.Fields
		out[f.ID] = NPCTemplate{Element: f.Element, Skills: [3]uint16{f.Skill1, f.Skill2, f.Skill3}, Name: r.Name.Text, Kind: f.Kind, Look: f.Look, Colors: [4]uint32{f.Color1, f.Color2, f.Color3, f.Color4},
			Face: f.Face, TalkLow: f.TalkLow, Sound: f.Sound, Shadow: f.Shadow, HeightScale: f.HeightScale, HeightPreset: f.HeightPreset}
	}
	npcData.path, npcData.templates = path, out
	return out, nil
}

// MapRecord decodes one map's eve.Emg record from the export.
func MapRecord(a login.Assets, mapID uint16) (native.Map, error) {
	var compiled []byte
	if err := clientruntime.Read(a.DataPath(clientruntime.EventPath(mapID)), &compiled); err == nil {
		scenes, err := MapScenes(a)
		if err != nil {
			return native.Map{}, err
		}
		return native.ParseEVEMap(mapID, scenes[mapID], compiled)
	} else if !os.IsNotExist(err) {
		return native.Map{}, err
	}

	npcData.Lock()
	path := a.DataPath(eveExport)
	if npcData.evePath != path {
		var doc struct {
			Maps []struct {
				ID         uint16 `json:"id"`
				Scene      uint16 `json:"scene"`
				DecodedHex string `json:"decoded_hex"`
			} `json:"maps"`
		}
		if err := readJSON(path, &doc); err != nil {
			npcData.Unlock()
			return native.Map{}, err
		}
		npcData.maps = make(map[uint16]eveMap, len(doc.Maps))
		for _, m := range doc.Maps {
			npcData.maps[m.ID] = eveMap{m.Scene, m.DecodedHex}
		}
		npcData.evePath = path
	}
	m, ok := npcData.maps[mapID]
	npcData.Unlock()
	if !ok {
		return native.Map{}, fmt.Errorf("map %d has no event record", mapID)
	}
	raw, err := hex.DecodeString(m.hex)
	if err != nil {
		return native.Map{}, fmt.Errorf("map %d: %w", mapID, err)
	}
	return native.ParseEVEMap(mapID, m.scene, raw)
}

// MapNPCs is FUN_003030ec's NPC part: one object per record, painted by
// newPainter from its template. Records without a template still exist
// (AC22:4 can address them) but draw nothing.
func MapNPCs(rec native.Map, templates map[uint32]NPCTemplate, newPainter func(NPCTemplate) NPCPainter) map[uint16]*NPC {
	out := make(map[uint16]*NPC, len(rec.NPCs))
	for _, r := range rec.NPCs {
		n := &NPC{ClickID: r.ClickID, Template: r.Template, X: int(r.X), Y: int(r.Y),
			Action: standFacing(r.Rotation), Shown: r.Flags&npcFlagShown != 0,
			Depth: recordDepth(r.UnknownWords)}
		if t, ok := templates[r.Template]; ok {
			n.Info = t
			if newPainter != nil {
				n.Painter = newPainter(t)
			}
		}
		out[r.ClickID] = n
	}
	return out
}

// ApplyActorPositions is AC22:4 (FUN_0038cf30) for map NPCs: each entry
// moves an NPC and sets its visibility. p is the packet after its command
// and subcommand bytes. A prop takes the state as its frame; the duration
// and the other NPCs' action fields drive animations that are not ported
// yet.
func (w *World) ApplyActorPositions(p []byte) {
	for ; len(p) >= actorRecordBytes; p = p[actorRecordBytes:] {
		n := w.NPCs[binary.LittleEndian.Uint16(p)]
		if n == nil {
			continue
		}
		n.X, n.Y = int(binary.LittleEndian.Uint16(p[4:])), int(binary.LittleEndian.Uint16(p[6:]))
		if n.Info.Prop() {
			f := p[actorStateOffset]
			n.Fixed, n.Frame = f != frameFree, int(f)
		}
		switch p[8] {
		case actorShown:
			n.Shown = true
		case actorHidden:
			n.Shown = false
		}
	}
}

// Hover marks the NPC under a screen point as hovered (drawn lit, as the
// pick of FUN_002fe8e8 → FUN_004106b0 and the map draw do); a point off
// every NPC, or (-1, -1), clears it.
func (w *World) Hover(x, y int) { w.hovered = w.NPCAt(x, y) }

// Hovered is the NPC under the pointer, nil for none.
func (w *World) Hovered() *NPC { return w.hovered }

// NPCAt is the shown NPC drawn under a screen point, the front one
// (largest sort key) when several overlap; nil for none.
func (w *World) NPCAt(x, y int) *NPC {
	cx, cy := w.Camera()
	var hit *NPC
	for _, n := range w.NPCs {
		if !n.Shown || n.Painter == nil || (hit != nil && n.SortY() < hit.SortY()) {
			continue
		}
		n.paint()
		if image.Pt(x, y).In(n.Painter.Bounds(n.X-cx, n.Y-cy+n.Info.SpriteDrop(), n.Action)) {
			hit = n
		}
	}
	return hit
}

// drawSmallShadow draws the Shadow cut at feet (sx, sy). FUN_0030120c
// draws it under players (+0xb1 is 1 or 2) and under NPCs of shadow kind 0.
func (w *World) drawSmallShadow(sx, sy int) {
	DrawShadow(w.Env.Screen, w.Env.Pics, shadowSmall, sx, sy)
}

// drawShadow draws an NPC's ground shadow at its feet (sx, sy).
func (w *World) drawShadow(n *NPC, sx, sy int) {
	DrawShadow(w.Env.Screen, w.Env.Pics, n.Info.Shadow, sx, sy)
}

// DrawShadow is FUN_0030120c's ground shadow of a template's shadow kind
// (Npc.dat offset 74) at feet (sx, sy).
func DrawShadow(dst *surface.Surface, pics *picdb.DB, kind byte, sx, sy int) {
	switch kind {
	case shadowSmall:
		if i := pics.Find(ShadowPicture); i >= 0 {
			pics.DrawRect(dst, i, sx+smallShadowX, sy+smallShadowY, smallShadowRect, true)
		}
	case shadowMonster:
		if i := pics.Find(MonsterShadowPicture); i >= 0 {
			pics.Draw(dst, i, sx+monsterShadowX, sy+monsterShadowY, true)
		}
	}
}

// drawName draws an NPC's name centred above it (0x415fca): white, in the
// outlined style, lifted by the template's name height.
func (w *World) drawName(n *NPC, sx, sy int) {
	if n.Info.Name == "" || n.Info.Unnamed() {
		return
	}
	anchor, ok := n.Painter.FirstAnchorY()
	if !ok {
		return
	}
	name := clientassets.Big5Text(n.Info.Name)
	width := len(name) * charW
	y := sy + nameTop(nameHeight(n.Info, anchor))
	w.Env.Text.Draw(sx-width/2, y, 0, false, true, w.Env.Screen, name, 0, width+charW, 0, npcNameInk, npcNameStyle)
}

// AC22:1 (FUN_00408054): click ID, then the frame. A prop whose frame goes
// from 0 to 1 plays its template's sound.
const (
	actorStateBytes = 3
	propClosed      = 0
	propOpened      = 1
)

// ApplyActorState is AC22:1 after its subcommand: it sets the NPC's fixed
// frame (+0x11f) and returns the sound to play as a prop opens, 0 for none.
func (w *World) ApplyActorState(p []byte) uint16 {
	if len(p) < actorStateBytes {
		return 0
	}
	n := w.NPCs[binary.LittleEndian.Uint16(p)]
	if n == nil {
		return 0
	}
	state := p[2]
	old := frameFree
	if n.Fixed {
		old = n.Frame
	}
	n.Fixed, n.Wrap, n.Frame = state != frameFree, false, int(state)
	if n.Info.Sound != 0 && old == propClosed && state == propOpened {
		return n.Info.Sound
	}
	return 0
}
