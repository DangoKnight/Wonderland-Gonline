package world

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"sync"

	"wonderland-go/client/wlo/login"
	"wonderland-go/client/wlo/surface"
	native "wonderland-go/internal/assets"
	"wonderland-go/internal/clientassets"
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
)

// NPCTemplate is the part of an Npc.dat record the map view uses, as
// FUN_004265a4 copies it (memory offsets are disk offsets + 4): the name
// (+0x08, drawn from the object's +9), the look (+0x12), four colour
// values (+0x16..+0x22), the shadow kind (+0x4e) and the two bytes that
// choose the name height (+0x3c, +0x5b), and the talk window's face
// sprite (+0x5c, FUN_002586c8).
type NPCTemplate struct {
	Name         string
	Look         uint16
	Face         uint16
	TalkLow      byte // +0x5a: 1 stands the talk window's body lower
	Colors       [4]uint32
	Shadow       byte
	HeightScale  byte
	HeightPreset byte
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
			} `json:"fields"`
		} `json:"records"`
	}
	if err := readJSON(path, &doc); err != nil {
		return nil, err
	}
	out := make(map[uint32]NPCTemplate, len(doc.Records))
	for _, r := range doc.Records {
		f := r.Fields
		out[f.ID] = NPCTemplate{Name: r.Name.Text, Look: f.Look, Colors: [4]uint32{f.Color1, f.Color2, f.Color3, f.Color4},
			Face: f.Face, TalkLow: f.TalkLow, Shadow: f.Shadow, HeightScale: f.HeightScale, HeightPreset: f.HeightPreset}
	}
	npcData.path, npcData.templates = path, out
	return out, nil
}

// MapRecord decodes one map's eve.Emg record from the export.
func MapRecord(a login.Assets, mapID uint16) (native.Map, error) {
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
			Action: standFacing(r.Rotation), Shown: r.Flags&npcFlagShown != 0}
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
// and subcommand bytes. The state, duration and action fields drive
// animations that are not ported yet.
func (w *World) ApplyActorPositions(p []byte) {
	for ; len(p) >= actorRecordBytes; p = p[actorRecordBytes:] {
		n := w.NPCs[binary.LittleEndian.Uint16(p)]
		if n == nil {
			continue
		}
		n.X, n.Y = int(binary.LittleEndian.Uint16(p[4:])), int(binary.LittleEndian.Uint16(p[6:]))
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
// (largest feet Y) when several overlap; nil for none.
func (w *World) NPCAt(x, y int) *NPC {
	cx, cy := w.Camera()
	var hit *NPC
	for _, n := range w.NPCs {
		if !n.Shown || n.Painter == nil || (hit != nil && n.Y < hit.Y) {
			continue
		}
		if image.Pt(x, y).In(n.Painter.Bounds(n.X-cx, n.Y-cy, n.Action)) {
			hit = n
		}
	}
	return hit
}

// drawSmallShadow draws the Shadow cut at feet (sx, sy). FUN_0030120c
// draws it under players (+0xb1 is 1 or 2) and under NPCs of shadow kind 0.
func (w *World) drawSmallShadow(sx, sy int) {
	pics := w.Env.Pics
	if i := pics.Find(ShadowPicture); i >= 0 {
		pics.DrawRect(w.Env.Screen, i, sx+smallShadowX, sy+smallShadowY, smallShadowRect, true)
	}
}

// drawShadow draws an NPC's ground shadow at its feet (sx, sy).
func (w *World) drawShadow(n *NPC, sx, sy int) {
	pics := w.Env.Pics
	switch n.Info.Shadow {
	case shadowSmall:
		w.drawSmallShadow(sx, sy)
	case shadowMonster:
		if i := pics.Find(MonsterShadowPicture); i >= 0 {
			pics.Draw(w.Env.Screen, i, sx+monsterShadowX, sy+monsterShadowY, true)
		}
	}
}

// drawName draws an NPC's name centred above it (0x415fca): white, in the
// outlined style, lifted by the template's name height.
func (w *World) drawName(n *NPC, sx, sy int) {
	if n.Info.Name == "" {
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
