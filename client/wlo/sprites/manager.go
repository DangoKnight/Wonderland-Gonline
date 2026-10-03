// Package sprites is the client's sprite manager: it loads sprite packs
// (internal/spritepack) by archive name and hands out frames as CPU images
// for the 16-bit renderer and as Ebitengine images, with animation players.
//
// Each archive is searched in order in the pack directories (built packs,
// pack.json) and the editable export directories (editable.json, read as a
// pack without repacking). The original jma/Jxa files are not read; build
// packs from them with cmd/sprite-build.
package sprites

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"wonderland-go/internal/spritepack"
)

// maxCachedPageBytes bounds decoded CPU pages; older pages are dropped
// (frames reload their page on demand).
const maxCachedPageBytes = 256 << 20

// ErrNotFound reports an archive that no source provides.
var ErrNotFound = errors.New("sprite archive not found")

// Manager loads archives on demand. It is safe for concurrent use.
type Manager struct {
	// PackDirs hold built packs: <dir>/<archive>/pack.json.
	PackDirs []string
	// EditableDirs hold the editable export: <dir>/<archive>/editable.json.
	EditableDirs []string

	mu       sync.Mutex
	archives map[string]*Archive
	missing  map[string]bool
	cached   int
	pages    []*page // decoded pages in load order, for eviction
	natives  []*nativeSource
	closed   bool
}

// Archive is one loaded archive.
type Archive struct {
	Name    string
	FrameMS int
	// SourceBytes is the original .jma file's size, or 0 when unknown.
	SourceBytes int64
	m           *Manager
	dir         string
	pages       []*page
	sprites     map[int]*Sprite
	native      *nativeSource // editable export's originals, for indices
}

// Sprite is one sprite of an archive.
type Sprite struct {
	Index int
	ID    int
	Name  string
	// Palette is the original RGB palette of indexed frames, or nil.
	Palette    *[256][3]uint8
	Frames     []Frame
	Animations []Animation
	derived    bool // indices matched against sprites.json (native.go)
}

// Frame is one image with its draw offset from the drawing point.
type Frame struct {
	Name             string
	OffsetX, OffsetY int
	Width, Height    int
	AnchorY          int // the bitmap's top on its canvas (frame record +0x12)
	page             *page
	index            *page       // palette indices, nil for a frame drawn as painted
	indices          *image.Gray // indices matched from sprites.json (native.go)
	rect             image.Rectangle
}

// Animation is the frame list of one action; -1 entries draw nothing.
type Animation struct {
	Action int
	Name   string
	Frames []int
}

type page struct {
	path string
	cpu  image.Image // *image.NRGBA, or *image.Gray for index pages; nil until loaded
	size int
	gpu  *ebiten.Image
}

// NewManager searches the given directories; empty strings are ignored.
func NewManager(packDirs, editableDirs []string) *Manager {
	keep := func(dirs []string) []string {
		var out []string
		for _, d := range dirs {
			if d != "" {
				out = append(out, d)
			}
		}
		return out
	}
	return &Manager{PackDirs: keep(packDirs), EditableDirs: keep(editableDirs),
		archives: map[string]*Archive{}, missing: map[string]bool{}}
}

// Archive returns a loaded archive, or ErrNotFound.
func (m *Manager) Archive(name string) (*Archive, error) {
	key := strings.ToLower(name)
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.archives[key]; ok {
		return a, nil
	}
	if m.missing[key] {
		return nil, ErrNotFound
	}
	a, err := m.load(key)
	if err != nil {
		m.missing[key] = true
		return nil, err
	}
	m.archives[key] = a
	return a, nil
}

func (m *Manager) load(name string) (*Archive, error) {
	for _, d := range m.PackDirs {
		dir := filepath.Join(d, name)
		if p, err := spritepack.Read(dir); err == nil {
			return m.fromPack(name, dir, p), nil
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
	}
	for _, d := range m.EditableDirs {
		dir := filepath.Join(d, name)
		if p, err := spritepack.EditablePack(dir, name); err == nil {
			a := m.fromPack(name, dir, p)
			a.attachNative(dir)
			return a, nil
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
	}
	return nil, ErrNotFound
}

func (m *Manager) fromPack(name, dir string, p *spritepack.Pack) *Archive {
	a := &Archive{Name: name, FrameMS: p.FrameMS, SourceBytes: p.SourceBytes, m: m, dir: dir, sprites: map[int]*Sprite{}}
	for _, path := range p.Pages {
		a.pages = append(a.pages, &page{path: filepath.Join(dir, filepath.FromSlash(path))})
	}
	index := make([]*page, len(p.Pages))
	for i, path := range p.IndexPages {
		if path != "" {
			index[i] = &page{path: filepath.Join(dir, filepath.FromSlash(path))}
		}
	}
	for _, s := range p.Sprites {
		a.sprites[s.Index] = a.convert(s, a.pages, index)
	}
	return a
}

func (a *Archive) convert(s spritepack.Sprite, pages, index []*page) *Sprite {
	out := &Sprite{Index: s.Index, ID: s.ID, Name: s.Name}
	if s.Palette != "" {
		if pal, err := spritepack.DecodePalette(s.Palette); err == nil {
			out.Palette = &pal
		}
	}
	for _, f := range s.Frames {
		fr := Frame{Name: f.Name, OffsetX: f.OffsetX, OffsetY: f.OffsetY, Width: f.Rect.W, Height: f.Rect.H, AnchorY: f.AnchorY}
		if f.Rect.W > 0 && f.Rect.H > 0 {
			fr.page = pages[f.Page]
			fr.rect = image.Rect(f.Rect.X, f.Rect.Y, f.Rect.X+f.Rect.W, f.Rect.Y+f.Rect.H)
			if f.Indexed && out.Palette != nil {
				fr.index = index[f.Page]
			}
		}
		out.Frames = append(out.Frames, fr)
	}
	for _, an := range s.Animations {
		out.Animations = append(out.Animations, Animation{Action: an.Action, Name: an.Name, Frames: an.Frames})
	}
	return out
}

// Sprite returns the sprite at an archive slot, or nil.
func (a *Archive) Sprite(index int) *Sprite {
	a.m.mu.Lock()
	defer a.m.mu.Unlock()
	s := a.sprites[index]
	if s != nil {
		a.derive(s)
	}
	return s
}

// Animation returns an action's animation, or nil.
func (s *Sprite) Animation(action int) *Animation {
	if s == nil || action < 0 || action >= len(s.Animations) {
		return nil
	}
	return &s.Animations[action]
}

// Frame returns frame n of an animation, or nil for a missing frame.
func (s *Sprite) Frame(an *Animation, n int) *Frame {
	if s == nil || an == nil || n < 0 || n >= len(an.Frames) {
		return nil
	}
	i := an.Frames[n]
	if i < 0 || i >= len(s.Frames) {
		return nil
	}
	return &s.Frames[i]
}

func (m *Manager) loadPage(p *page) (image.Image, error) {
	if p.cpu != nil {
		return p.cpu, nil
	}
	f, err := os.Open(p.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	if cfg.Width > spritepack.MaxPageSide || cfg.Height > spritepack.MaxPageSide {
		return nil, fmt.Errorf("%s exceeds %d pixels", p.path, spritepack.MaxPageSide)
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, err
	}
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	var out image.Image
	size := 0
	switch v := img.(type) {
	case *image.NRGBA:
		out, size = v, len(v.Pix)
	case *image.Gray:
		out, size = v, len(v.Pix)
	default:
		n := image.NewNRGBA(img.Bounds())
		draw.Draw(n, n.Bounds(), img, img.Bounds().Min, draw.Src)
		out, size = n, len(n.Pix)
	}
	for m.cached+size > maxCachedPageBytes && len(m.pages) > 0 {
		old := m.pages[0]
		m.pages = m.pages[1:]
		m.cached -= old.size
		old.cpu = nil
	}
	p.cpu, p.size = out, size
	m.cached += size
	m.pages = append(m.pages, p)
	return out, nil
}

// Image returns the frame's pixels for CPU drawing (straight alpha), or
// nil for a frame without pixels.
func (f *Frame) Image(m *Manager) (*image.NRGBA, error) {
	if f == nil || f.page == nil {
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	img, err := m.loadPage(f.page)
	if err != nil {
		return nil, err
	}
	pg, ok := img.(*image.NRGBA)
	if !ok || !f.rect.In(pg.Bounds()) {
		return nil, fmt.Errorf("frame outside %s", f.page.path)
	}
	return pg.SubImage(f.rect).(*image.NRGBA), nil
}

// Indices returns the frame's palette indices (for recolouring with the
// sprite's Palette), or nil for a frame without them.
func (f *Frame) Indices(m *Manager) (*image.Gray, error) {
	if f == nil {
		return nil, nil
	}
	if f.indices != nil {
		return f.indices, nil
	}
	if f.index == nil {
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	img, err := m.loadPage(f.index)
	if err != nil {
		return nil, err
	}
	pg, ok := img.(*image.Gray)
	if !ok || !f.rect.In(pg.Bounds()) {
		return nil, fmt.Errorf("frame outside %s", f.index.path)
	}
	return pg.SubImage(f.rect).(*image.Gray), nil
}

// Ebiten returns the frame as a sub-image of its page's texture, or nil
// for a frame without pixels. Pages are uploaded once and kept.
func (f *Frame) Ebiten(m *Manager) (*ebiten.Image, error) {
	if f == nil || f.page == nil {
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if f.page.gpu == nil {
		pg, err := m.loadPage(f.page)
		if err != nil {
			return nil, err
		}
		f.page.gpu = ebiten.NewImageFromImage(pg)
	}
	return f.page.gpu.SubImage(f.rect).(*ebiten.Image), nil
}

// Draw draws the frame on an Ebitengine image with its offset from the
// drawing point (x, y); op may be nil and its geometry is applied after
// the placement.
func (f *Frame) Draw(m *Manager, dst *ebiten.Image, x, y float64, op *ebiten.DrawImageOptions) error {
	img, err := f.Ebiten(m)
	if err != nil || img == nil {
		return err
	}
	o := &ebiten.DrawImageOptions{}
	if op != nil {
		*o = *op
	}
	o.GeoM.Reset()
	o.GeoM.Translate(x+float64(f.OffsetX), y+float64(f.OffsetY))
	if op != nil {
		o.GeoM.Concat(op.GeoM)
	}
	dst.DrawImage(img, o)
	return nil
}

// Player steps through an animation on the archive's clock.
type Player struct {
	Sprite    *Sprite
	Action    int
	Loop      bool
	FrameTime time.Duration
	elapsed   time.Duration
	frame     int
}

// NewPlayer plays an action of a sprite at the archive's frame time.
func NewPlayer(a *Archive, s *Sprite, action int) *Player {
	ms := spritepack.DefaultFrameMS
	if a != nil && a.FrameMS > 0 {
		ms = a.FrameMS
	}
	return &Player{Sprite: s, Action: action, Loop: true, FrameTime: time.Duration(ms) * time.Millisecond}
}

// SetAction switches to an action from its first frame.
func (p *Player) SetAction(action int) {
	if action != p.Action {
		p.Action, p.frame, p.elapsed = action, 0, 0
	}
}

// Update advances the clock; it reports whether the frame changed.
func (p *Player) Update(dt time.Duration) bool {
	an := p.Sprite.Animation(p.Action)
	if an == nil || len(an.Frames) == 0 || p.FrameTime <= 0 {
		return false
	}
	p.elapsed += dt
	changed := false
	for p.elapsed >= p.FrameTime {
		p.elapsed -= p.FrameTime
		switch {
		case p.frame+1 < len(an.Frames):
			p.frame++
			changed = true
		case p.Loop:
			p.frame, changed = 0, true
		}
	}
	return changed
}

// Index is the position in the animation.
func (p *Player) Index() int { return p.frame }

// Frame is the current frame, or nil.
func (p *Player) Frame() *Frame {
	return p.Sprite.Frame(p.Sprite.Animation(p.Action), p.frame)
}
