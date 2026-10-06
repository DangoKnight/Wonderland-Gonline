// Package movie plays the client's scripted scenes: the .sty files (TMovie,
// loader FUN_0033e18c, frame update FUN_003406c4, drawing FUN_00340404) that
// event kind 5 starts (FUN_00304fd0 case 5). A movie is a run of stages;
// in each stage its actors (the player, NPCs and pictures) and its camera
// move toward their next keyframe, a line of dialogue may be said, and the
// stage may wait; when every part is done the next stage begins.
//
// The movies come from the export data/media/sty/style_data.json. Its
// field split does not follow the native records (a keyframe's byte flag
// is followed by unaligned ints, and an NPC's first keyframe begins inside
// the exported header), so each section is joined back into its bytes and
// read with the record layouts of the loader.
package movie

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"wonderland-gonline/internal/clientfs"

	"wonderland-gonline/client/wlo/login"
)

const styleExport = "style_data.json"

// PlayerTemplate is the actor or speaker that stands for the player.
const PlayerTemplate = 0xffff

// Keep is the keyframe value that keeps an actor's facing or pose.
const Keep = 0xff

// Keyframe is a 45-byte actor record (read to +0x94 + k·0x2d).
type Keyframe struct {
	Facing    int // +0x00, 255 keeps
	X, Y      int // +0x04, +0x08: the point to walk to
	Shown     bool
	Speed     int // +0x0d: the speed class (speeds)
	Pose      int // +0x11 (+0xa5): the role's pose (+0x1f20), 255 keeps
	Field15   int
	Frame     int // +0x19: the frame shown (1-based) instead of animating, 0 to animate
	Field1d   int
	Sound     int  // +0x21: 1-based index into the sound list, 0 for none
	SoundLoop bool // +0x25
	Channel   int  // +0x26
}

const keyframeBytes = 0x2d

// Actor is a movie character: the player (template 0xffff) or an NPC of an
// Npc.dat template, present from stage Start for Count keyframes (+0x2dbc,
// +0x2dcc).
type Actor struct {
	Template uint32
	Start    int
	Count    int
	Facing   int // +0x2dd0
	Keys     []Keyframe
	Extra    int32 // NPCs' +0x2e00
}

// Point is a 33-byte camera or picture keyframe (+0x34 / +0x3c + k·0x21).
type Point struct {
	X, Y  int
	Shown bool
	Speed int
	Frame int // +0xd: a picture's fixed frame (1-based), 0 to animate
	Raw   [0x21]byte
}

const pointBytes = 0x21

// Camera is the background track: the scene it shows and where the screen's
// centre goes in each of its keyframes (+0x2134, +0x2144).
type Camera struct {
	Scene string // "Map10001": the map whose scene is drawn
	Start int
	Count int
	Keys  []Point
}

// Picture is an animated picture placed in the scene (+0xe8 + i·4).
type Picture struct {
	Name   string
	Header [8]int32
	Keys   []Point
	Suffix int32
}

// A picture's header, as the loader (FUN_0033e18c) reads it: the stage
// span (+0x213c, +0x214c), the colour key (+0x215c), the light level
// (+0x2164; 255 draws with the key alone), the frame grid (+0x2c rows,
// +0x30 columns), the frame time (+0x2158, ms) and the repeats (+0x2168).
const (
	pictureStart = iota
	pictureCount
	pictureKey
	pictureLevel
	pictureRows
	pictureCols
	pictureInterval
	pictureRepeats
)

// Start and Count are a picture's stage span.
func (p *Picture) Start() int { return int(p.Header[pictureStart]) }
func (p *Picture) Count() int { return int(p.Header[pictureCount]) }

// Key is the picture's transparent RGB565 colour.
func (p *Picture) Key() uint16 { return uint16(p.Header[pictureKey]) }

// Level is the light level; LevelKeyed draws with the colour key only.
func (p *Picture) Level() int { return int(p.Header[pictureLevel]) }

// LevelKeyed is the level of a plain colour-keyed picture.
const LevelKeyed = 0xff

// Grid is the frame grid: rows of frames stacked vertically, columns side
// by side (the draw uses one column).
func (p *Picture) Grid() (rows, cols int) {
	return max(int(p.Header[pictureRows]), 1), max(int(p.Header[pictureCols]), 1)
}

// Line is a line of dialogue (+0x164 + i·4): said at Stage after Delay ms
// by Speaker (an Npc.dat template, or 0xffff for the player).
//
// lineInts is a record's size in ints; the export's line rows begin
// lineShift ints into the unused record 0.
const (
	lineInts  = 4
	lineShift = 2
)

type Line struct {
	Stage   int
	Delay   int
	Speaker uint32
	Talk    uint16
}

// Stage is one stage's timeline entry (+0x2f8, +0x2fc).
type Stage struct {
	Wait   int  // ms the stage lasts at least
	Effect byte // +4: the screen effect operand (Effect*)
	ShakeA byte // +8: the first shake operand (+0xf9c, Shake*)
	ShakeB byte // +0xc: the second (+0xf9d)
	Music  int  // +0x10: 1-based sound table entry, 0 for none
	// Tint is effect 9's fill colour (+0x38..+0x44): alpha, red, green,
	// blue.
	Tint [4]byte
}

// Movie is one .sty file.
type Movie struct {
	Name   string
	Player *Actor // nil when the player does not appear
	NPCs   []Actor
	Images []Picture
	Camera Camera
	Lines  []Line
	Stages []Stage // index = stage, 0..Last
	Last   int     // +0xf94
}

var styles struct {
	sync.Mutex
	path string
	raw  map[string]json.RawMessage
}

// Load reads movie id (<id>.sty) from the export.
func Load(a login.Assets, id int) (*Movie, error) {
	raw, err := entry(a, strconv.Itoa(id)+".sty")
	if err != nil {
		return nil, err
	}
	return decode(raw)
}

func entry(a login.Assets, name string) (json.RawMessage, error) {
	styles.Lock()
	defer styles.Unlock()
	path := a.MediaPath("sty", styleExport)
	if styles.path != path {
		b, err := clientfs.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var doc struct {
			Styles map[string]json.RawMessage `json:"styles"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", styleExport, err)
		}
		styles.path, styles.raw = path, doc.Styles
	}
	r, ok := styles.raw[name]
	if !ok {
		return nil, fmt.Errorf("movie %s not found", name)
	}
	return r, nil
}

// The export's shapes.
type exFrame struct {
	Ints []int32 `json:"unknown_i32_fields"`
	U44  byte    `json:"unknown_u8_offset_44"`
	U32  byte    `json:"unknown_u8_offset_32"`
}

type exActor struct {
	Header []int32   `json:"header_i32_fields"`
	Frames []exFrame `json:"frames"`
}

type exTrack struct {
	Name   string    `json:"name"`
	Header []int32   `json:"header_i32_fields"`
	Frames []exFrame `json:"frames"`
	Suffix int32     `json:"unknown_i32_suffix"`
}

type exStyle struct {
	Source   string    `json:"source"`
	Counts   []int     `json:"section_counts"`
	Special  *exActor  `json:"special_sprite"`
	Sprites  []exActor `json:"sprites"`
	Images   []exTrack `json:"images"`
	Back     exTrack   `json:"background"`
	Timeline struct {
		Waits    []int32 `json:"stage_i32_values"`
		Operands []struct {
			B0  byte   `json:"unknown_u8_0"`
			B1  byte   `json:"unknown_u8_1"`
			B2  byte   `json:"unknown_u8_2"`
			U3  uint32 `json:"unknown_u32_3"`
			B7  byte   `json:"unknown_u8_7"`
			B8  byte   `json:"unknown_u8_8"`
			B9  byte   `json:"unknown_u8_9"`
			B10 byte   `json:"unknown_u8_10"`
		} `json:"operands"`
	} `json:"timeline"`
	Extra [][]int32 `json:"extra_records"`
}

func appendInts(b []byte, v []int32) []byte {
	for _, x := range v {
		b = binary.LittleEndian.AppendUint32(b, uint32(x))
	}
	return b
}

func decode(raw json.RawMessage) (*Movie, error) {
	var s exStyle
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	m := &Movie{Name: s.Source}
	// The player takes part when the first section counts one.
	if s.Special != nil {
		var err error
		if m.Player, err = actorOf(*s.Special, false); err != nil {
			return nil, fmt.Errorf("%s player: %w", s.Source, err)
		}
	}
	for _, sp := range s.Sprites {
		a, err := actorOf(sp, true)
		if err != nil {
			return nil, fmt.Errorf("%s NPC: %w", s.Source, err)
		}
		m.NPCs = append(m.NPCs, *a)
	}
	for _, im := range s.Images {
		p := Picture{Name: im.Name, Suffix: im.Suffix}
		copy(p.Header[:], im.Header)
		p.Keys = points(im.Frames)
		m.Images = append(m.Images, p)
	}
	m.Camera = Camera{Scene: s.Back.Name, Keys: points(s.Back.Frames)}
	if len(s.Back.Header) >= 2 {
		m.Camera.Start, m.Camera.Count = int(s.Back.Header[0]), int(s.Back.Header[1])
	}
	for i, w := range s.Timeline.Waits {
		st := Stage{Wait: int(w)}
		if i < len(s.Timeline.Operands) {
			o := s.Timeline.Operands[i]
			st.Effect, st.ShakeA, st.ShakeB, st.Music = o.B0, o.B1, o.B2, int(o.U3)
			st.Tint = [4]byte{o.B7, o.B8, o.B9, o.B10}
		}
		m.Stages = append(m.Stages, st)
	}
	if len(s.Counts) == 6 {
		m.Last = s.Counts[5]
	}
	lines := 0
	if len(s.Counts) == 6 {
		lines = s.Counts[4]
	}
	// The loader reads lines+1 records of four ints (speaker, talk, stage at
	// +0xc, delay at +0x10) and plays records 1..lines; the export's rows
	// start two ints into record 0, so record i begins at 4i − 2.
	var flat []int32
	for _, r := range s.Extra {
		flat = append(flat, r...)
	}
	for i := 1; i <= lines; i++ {
		k := lineInts*i - lineShift
		if k+lineInts > len(flat) {
			break
		}
		r := flat[k:]
		m.Lines = append(m.Lines, Line{Speaker: uint32(r[0]), Talk: uint16(r[1]), Stage: int(r[2]), Delay: int(r[3])})
	}
	return m, nil
}

// actorOf joins an actor's exported pieces into its bytes: four header
// ints, the keyframes, and for NPCs a trailing int.
func actorOf(e exActor, npc bool) (*Actor, error) {
	if len(e.Header) < 4 {
		return nil, fmt.Errorf("short header")
	}
	b := appendInts(nil, e.Header[4:])
	for _, f := range e.Frames {
		b = append(appendInts(b, f.Ints), f.U44)
	}
	a := &Actor{Template: uint32(e.Header[0]), Start: int(e.Header[1]), Count: int(e.Header[2]), Facing: int(e.Header[3])}
	n := a.Count + 1
	if len(b) < n*keyframeBytes {
		return nil, fmt.Errorf("%d bytes for %d keyframes", len(b), n)
	}
	for k := range n {
		a.Keys = append(a.Keys, keyframe(b[k*keyframeBytes:]))
	}
	if rest := b[n*keyframeBytes:]; npc && len(rest) >= 4 {
		a.Extra = int32(binary.LittleEndian.Uint32(rest))
	}
	return a, nil
}

func keyframe(b []byte) Keyframe {
	i := func(o int) int { return int(int32(binary.LittleEndian.Uint32(b[o:]))) }
	return Keyframe{Facing: i(0), X: i(4), Y: i(8), Shown: b[0xc] != 0, Speed: i(0xd), Pose: i(0x11),
		Field15: i(0x15), Frame: i(0x19), Field1d: i(0x1d), Sound: i(0x21), SoundLoop: b[0x25] != 0, Channel: i(0x26)}
}

func points(frames []exFrame) []Point {
	var b []byte
	for _, f := range frames {
		b = append(appendInts(b, f.Ints), f.U32)
	}
	var out []Point
	for ; len(b) >= pointBytes; b = b[pointBytes:] {
		p := Point{X: int(int32(binary.LittleEndian.Uint32(b))), Y: int(int32(binary.LittleEndian.Uint32(b[4:]))),
			Shown: b[8] != 0, Speed: int(int32(binary.LittleEndian.Uint32(b[9:]))),
			Frame: int(int32(binary.LittleEndian.Uint32(b[0xd:])))}
		copy(p.Raw[:], b)
		out = append(out, p)
	}
	return out
}
