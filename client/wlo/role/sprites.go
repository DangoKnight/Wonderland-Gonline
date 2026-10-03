// Package role ports the character drawing of aLogin.exe's THuman objects
// (VMT 0x40fa0c): the archive lookup rules, the layered player body and the
// frame placement. Frames come from the sprite manager (client/wlo/sprites),
// which reads built sprite packs or the editable export.
// Addresses are from the WLRI build decompile.
package role

import (
	"image/color"
	"log"
	"path/filepath"
	"strings"

	"wonderland-go/client/wlo/sprites"
	"wonderland-go/client/wlo/surface"
)

const idsPerFamily = 1000

// archive is one archive with its first sprite ID (+0x18).
type archive struct {
	first int
	arc   *sprites.Archive
	m     *sprites.Manager
}

// sprite is one sprite of an archive.
type sprite struct {
	*sprites.Sprite
	m        *sprites.Manager
	reported bool
}

// Library resolves sprite IDs to archive slots and loads them through the
// manager.
type Library struct {
	Sprites *sprites.Manager
	opened  map[string]*archive
	missing map[string]bool
}

// NewLibrary reads editable sprites from the decompiled data root.
func NewLibrary(root string) *Library {
	return NewLibraryWith(sprites.NewManager(
		nil,
		[]string{filepath.Join(root, "sprites")}))
}

// NewLibraryWith uses an existing manager.
func NewLibraryWith(m *sprites.Manager) *Library {
	return &Library{Sprites: m, opened: map[string]*archive{}, missing: map[string]bool{}}
}

func (l *Library) open(name string, first int) *archive {
	key := strings.ToLower(name)
	if a, ok := l.opened[key]; ok {
		return a
	}
	if l.missing[key] {
		return nil
	}
	arc, err := l.Sprites.Archive(key)
	if err != nil {
		if err != sprites.ErrNotFound {
			log.Printf("sprites %s: %v", key, err)
		}
		l.missing[key] = true
		return nil
	}
	a := &archive{first: first, arc: arc, m: l.Sprites}
	l.opened[key] = a
	return a
}

// firstID is an archive's first sprite ID from spriteArchives.
func firstID(name string) int {
	for _, a := range spriteArchives {
		if strings.EqualFold(a.Name, name) {
			return a.First
		}
	}
	return 0
}

// lookup is FUN_0045ac44: for a family in lookupGroups, the last patch
// whose first ID is at most the ID (or the ID less one) replaces the base,
// and the ID is shifted when the family's shift is still in effect. Other
// families are drawn from their own archive. It returns the archive and
// the ID to use inside it.
//
// The WLRI build names patch archives (002e_1, 002w_01, …) that neither
// its own install nor ours contains; those installs carry the patch
// sprites in the base archive instead. Taken literally the original would
// draw nothing for them, so a chosen patch that is not installed falls
// back to the base archive with the base's own ID mapping.
func (l *Library) lookup(base string, id int) (*archive, int) {
	g, ok := lookupGroups[base]
	if !ok {
		return l.open(base, firstID(base)), id
	}
	chosen, shift := base, g.Shift
	for _, m := range g.Members {
		cmp := id
		if m.MinusOne {
			cmp = id - 1
		}
		if f := firstID(m.Name); f > 0 && f <= cmp {
			chosen = m.Name
			if m.Reset {
				shift = 0
			}
		}
	}
	if a := l.open(chosen, firstID(chosen)); a != nil || chosen == base {
		return a, id + shift
	}
	return l.open(base, firstID(base)), id + g.Shift
}

// index is FUN_00302934: the sprite's entry in its archive.
func (a *archive) index(id int) int {
	if a.first > 0 {
		return id - a.first
	}
	return id % idsPerFamily
}

func (a *archive) sprite(id int) *sprite {
	s := a.arc.Sprite(a.index(id))
	if s == nil {
		return nil
	}
	return &sprite{Sprite: s, m: a.m}
}

// frameCount is FUN_002fded4: the frames of an action.
func (s *sprite) frameCount(action int) int {
	if an := s.Animation(action); an != nil {
		return len(an.Frames)
	}
	return 0
}

// frame is an action's frame (FUN_002fe570), or nil.
func (s *sprite) frame(action, n int) *sprites.Frame {
	return s.Frame(s.Animation(action), n)
}

// drawColored draws an indexed frame through the sprite's palette shifted
// by the colour block (FUN_002fe8e8); frames without indices, such as
// edited ones, draw as painted.
func (s *sprite) drawColored(dst *surface.Surface, f *sprites.Frame, x, y, id int, colors *Colors) {
	if colors == nil || s.Palette == nil {
		s.draw(dst, f, x, y)
		return
	}
	ix, err := f.Indices(s.m)
	if err != nil || ix == nil {
		s.draw(dst, f, x, y)
		return
	}
	pal := colors.palette(id, s.Palette)
	lut, opaque := lookup(&pal)
	left, top := x+f.OffsetX, y+f.OffsetY
	b := ix.Bounds()
	for row := 0; row < f.Height; row++ {
		dy := top + row
		if dy < 0 || dy >= dst.H {
			continue
		}
		for col := 0; col < f.Width; col++ {
			dx := left + col
			if dx < 0 || dx >= dst.W {
				continue
			}
			if i := ix.Pix[ix.PixOffset(b.Min.X+col, b.Min.Y+row)]; opaque[i] {
				dst.Pix[dy*dst.W+dx] = lut[i]
			}
		}
	}
}

// draw blits a frame at its offset from (x, y) onto the 16-bit surface.
// The offsets resolve FUN_002fe8e8's placement (spritepack.Offsets).
// Partially transparent pixels, possible in edited PNGs, blend with the
// framebuffer.
func (s *sprite) draw(dst *surface.Surface, f *sprites.Frame, x, y int) {
	pixels, err := f.Image(s.m)
	if err != nil {
		if !s.reported {
			log.Printf("sprite %s: %v", s.Name, err)
			s.reported = true
		}
		return
	}
	if pixels == nil {
		return
	}
	left, top := x+f.OffsetX, y+f.OffsetY
	b := pixels.Bounds()
	for row := 0; row < f.Height; row++ {
		dy := top + row
		if dy < 0 || dy >= dst.H {
			continue
		}
		for col := 0; col < f.Width; col++ {
			dx := left + col
			if dx < 0 || dx >= dst.W {
				continue
			}
			c := pixels.NRGBAAt(b.Min.X+col, b.Min.Y+row)
			if c.A == 0 {
				continue
			}
			if c.A < 255 {
				c = blendSpritePixel(c, surface.Expand(dst.Pix[dy*dst.W+dx]))
			}
			dst.Pix[dy*dst.W+dx] = surface.RGB565(c.R, c.G, c.B)
		}
	}
}

// Blend straight PNG alpha against the RGB565 framebuffer in RGB space.
func blendSpritePixel(source color.NRGBA, destination color.RGBA) color.NRGBA {
	alpha := uint32(source.A)
	inverse := 255 - alpha
	mix := func(s, d uint8) uint8 { return uint8((uint32(s)*alpha + uint32(d)*inverse + 127) / 255) }
	return color.NRGBA{R: mix(source.R, destination.R), G: mix(source.G, destination.G), B: mix(source.B, destination.B), A: 255}
}
