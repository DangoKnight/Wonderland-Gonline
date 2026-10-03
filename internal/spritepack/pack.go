// Package spritepack is the client's runtime sprite format: one directory
// per original archive holding pack.json and RGBA PNG atlas pages.
//
// A pack describes each sprite by its archive slot (the index the original
// client's lookups use) and its ID, its frames as rectangles on atlas
// pages with draw offsets already resolved, and its animations by action
// index. Packs are built from the editable export (data/sprites) or from
// the original jma/Jxa archives; the editable export itself is readable as
// a pack without repacking.
package spritepack

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// Version is the pack.json schema version.
	Version = 1
	// FileName is the pack index inside an archive directory.
	FileName = "pack.json"
	// DefaultFrameMS is the client's animation step (FUN_00411f54(…, 100)).
	DefaultFrameMS = 100
	// GroundOffset is how far below the drawing point the original renderer
	// puts the bottom of a frame's canvas (FUN_002fe8e8).
	GroundOffset = 0x20
	// MaxPageSide bounds atlas pages.
	MaxPageSide = 4096

	maxSprites    = 1 << 16
	maxFrames     = 1 << 20
	maxCanvasSide = 1 << 16
)

// Pack is pack.json.
type Pack struct {
	Version int    `json:"version"`
	Archive string `json:"archive"`
	FrameMS int    `json:"frame_ms"`
	// SourceBytes is the size of the original .jma file, when known. The
	// client derives its content level from 001's size (FUN_004a3b50).
	SourceBytes int64    `json:"source_bytes,omitempty"`
	Pages       []string `json:"pages"`
	// IndexPages parallel Pages: 8-bit grey PNGs of palette indices at the
	// same rectangles, "" for a page without indexed frames. Omitted when
	// no frame is indexed.
	IndexPages []string `json:"index_pages,omitempty"`
	Sprites    []Sprite `json:"sprites"`
}

// Sprite is one sprite of the archive.
type Sprite struct {
	// Index is the archive slot; the client's sprite lookup resolves IDs
	// to slots (FUN_00302934), so it must stay stable.
	Index int `json:"index"`
	// ID is the numeric sprite name ("2200.jmp" is 2200), or -1.
	ID   int    `json:"id"`
	Name string `json:"name"`
	// Palette is the sprite's 256-entry RGB palette as 1,536 hex digits,
	// for recolouring indexed frames; empty when the sprite has none.
	Palette    string      `json:"palette,omitempty"`
	Frames     []Frame     `json:"frames"`
	Animations []Animation `json:"animations"`
}

// Frame is one image of a sprite.
type Frame struct {
	Name string `json:"name,omitempty"`
	// Page and Rect locate the pixels; a zero-sized frame draws nothing.
	Page int  `json:"page"`
	Rect Rect `json:"rect"`
	// Indexed frames also have palette indices at Rect on the page's
	// index page; edited frames without them draw as painted.
	Indexed bool `json:"indexed,omitempty"`
	// OffsetX and OffsetY place the frame's top-left corner relative to
	// the drawing point (a character's feet).
	OffsetX int `json:"offset_x"`
	OffsetY int `json:"offset_y"`
	// The original canvas and anchor, kept for editing.
	CanvasWidth  int `json:"canvas_width"`
	CanvasHeight int `json:"canvas_height"`
	AnchorX      int `json:"anchor_x"`
	AnchorY      int `json:"anchor_y"`
}

// Rect is a page rectangle.
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Animation lists frame indexes for one action; -1 draws nothing.
type Animation struct {
	Action int    `json:"action"`
	Name   string `json:"name,omitempty"`
	Frames []int  `json:"frames"`
}

// Offsets resolves the original renderer's placement: the canvas is
// centred on the drawing point and its bottom sits GroundOffset below it;
// the anchor is the frame's position inside the canvas.
func Offsets(canvasW, canvasH, anchorX, anchorY int) (int, int) {
	return anchorX - canvasW/2, anchorY - (canvasH - GroundOffset)
}

// SpriteID parses a sprite name such as "2200.jmp"; other names give -1.
func SpriteID(name string) int {
	stem := strings.TrimSuffix(strings.ToLower(name), ".jmp")
	if id, err := strconv.Atoi(stem); err == nil && id >= 0 {
		return id
	}
	return -1
}

var errPack = errors.New("invalid sprite pack")

// Validate checks bounds and references before the pack is used.
func (p *Pack) Validate() error {
	if p.Version != Version {
		return fmt.Errorf("%w: version %d", errPack, p.Version)
	}
	if p.FrameMS <= 0 {
		p.FrameMS = DefaultFrameMS
	}
	if len(p.Sprites) > maxSprites {
		return fmt.Errorf("%w: %d sprites", errPack, len(p.Sprites))
	}
	for _, page := range p.Pages {
		if !SafePath(page) {
			return fmt.Errorf("%w: page path %q", errPack, page)
		}
	}
	if len(p.IndexPages) != 0 && len(p.IndexPages) != len(p.Pages) {
		return fmt.Errorf("%w: %d index pages for %d pages", errPack, len(p.IndexPages), len(p.Pages))
	}
	for _, page := range p.IndexPages {
		if page != "" && !SafePath(page) {
			return fmt.Errorf("%w: index page path %q", errPack, page)
		}
	}
	seen := map[int]bool{}
	frames := 0
	for _, s := range p.Sprites {
		if s.Index < 0 || s.Index >= maxSprites || seen[s.Index] {
			return fmt.Errorf("%w: sprite index %d", errPack, s.Index)
		}
		seen[s.Index] = true
		frames += len(s.Frames)
		if frames > maxFrames {
			return fmt.Errorf("%w: too many frames", errPack)
		}
		for _, f := range s.Frames {
			r := f.Rect
			if r.W == 0 || r.H == 0 {
				continue
			}
			if f.Page < 0 || f.Page >= len(p.Pages) || r.X < 0 || r.Y < 0 || r.W < 0 || r.H < 0 ||
				r.W > MaxPageSide || r.H > MaxPageSide || r.X > MaxPageSide-r.W || r.Y > MaxPageSide-r.H {
				return fmt.Errorf("%w: frame of %s", errPack, s.Name)
			}
			if f.Indexed && (len(p.IndexPages) == 0 || p.IndexPages[f.Page] == "" || s.Palette == "") {
				return fmt.Errorf("%w: indexed frame of %s has no index page or palette", errPack, s.Name)
			}
			if f.CanvasWidth < 0 || f.CanvasHeight < 0 || f.CanvasWidth > maxCanvasSide || f.CanvasHeight > maxCanvasSide {
				return fmt.Errorf("%w: canvas of %s", errPack, s.Name)
			}
		}
		if s.Palette != "" {
			if _, err := DecodePalette(s.Palette); err != nil {
				return fmt.Errorf("%w: palette of %s", errPack, s.Name)
			}
		}
		for _, a := range s.Animations {
			for _, i := range a.Frames {
				if i < -1 || i >= len(s.Frames) {
					return fmt.Errorf("%w: animation %d of %s", errPack, a.Action, s.Name)
				}
			}
		}
	}
	return nil
}

// SafePath accepts relative PNG paths that stay inside the pack directory.
func SafePath(path string) bool {
	clean := filepath.Clean(filepath.FromSlash(path))
	return path != "" && !filepath.IsAbs(clean) && clean != ".." &&
		!strings.HasPrefix(clean, ".."+string(filepath.Separator)) &&
		!strings.ContainsAny(path, "\\:") && strings.EqualFold(filepath.Ext(clean), ".png")
}

// Read loads and validates dir/pack.json.
func Read(dir string) (*Pack, error) {
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return nil, err
	}
	var p Pack
	if err = json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	if err = p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// EncodePalette writes a palette as hex RGB triples.
func EncodePalette(p [256][3]uint8) string {
	b := make([]byte, 0, len(p)*3)
	for _, c := range p {
		b = append(b, c[0], c[1], c[2])
	}
	return hex.EncodeToString(b)
}

// DecodePalette reads EncodePalette's form.
func DecodePalette(s string) ([256][3]uint8, error) {
	var p [256][3]uint8
	b, err := hex.DecodeString(s)
	if err != nil {
		return p, err
	}
	if len(b) != len(p)*3 {
		return p, fmt.Errorf("%w: palette has %d bytes", errPack, len(b))
	}
	for i := range p {
		p[i] = [3]uint8{b[3*i], b[3*i+1], b[3*i+2]}
	}
	return p, nil
}
