package spritepack

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"image"
	"image/draw"
)

// DefaultPageSide is the atlas page size the builder uses: large enough for
// every native frame and within common GPU texture limits.
const DefaultPageSide = 2048

// framePadding keeps one transparent pixel between packed frames so GPU
// filtering never samples a neighbour.
const framePadding = 1

// FrameImage is one source frame: its cropped pixels (nil or empty for a
// frame without pixels), optionally the palette indices those pixels came
// from, and its original canvas placement.
type FrameImage struct {
	Name  string
	Image image.Image
	// Indices, when set, are the frame's palette indices (same size as
	// Image). They let the client recolour palette ranges.
	Indices      *image.Gray
	CanvasWidth  int
	CanvasHeight int
	AnchorX      int
	AnchorY      int
}

// Page is one finished atlas page; Indices is nil when no frame on the
// page has palette indices.
type Page struct {
	Index   int
	RGBA    *image.NRGBA
	Indices *image.Gray
}

// PageSink receives each finished atlas page.
type PageSink func(Page) error

// Builder packs sprites into atlas pages with shelf packing, in the order
// sprites are added so a sprite's frames stay together. Identical frames
// of the archive (pixels and indices) share their pixels.
type Builder struct {
	PageSide int
	pack     Pack
	sink     PageSink
	page     *image.NRGBA
	indices  *image.Gray
	indexed  bool // a frame on the current page has indices
	pageNo   int
	x, y     int
	shelfH   int
	seen     map[uint64][]placed
}

type placed struct {
	page    int
	rect    Rect
	indexed bool
}

// NewBuilder starts a pack for an archive; finished pages go to sink.
func NewBuilder(archive string, sink PageSink) *Builder {
	return &Builder{
		PageSide: DefaultPageSide,
		pack:     Pack{Version: Version, Archive: archive, FrameMS: DefaultFrameMS},
		sink:     sink,
		seen:     map[uint64][]placed{},
	}
}

// SetSourceBytes records the original archive's size in the pack.
func (b *Builder) SetSourceBytes(n int64) { b.pack.SourceBytes = n }

// PageName and IndexPageName are the file names of atlas page i.
func PageName(i int) string      { return fmt.Sprintf("page_%03d.png", i) }
func IndexPageName(i int) string { return fmt.Sprintf("index_%03d.png", i) }

// flush hands the current page to the sink, trimmed to height h.
func (b *Builder) flush(h int) error {
	r := image.Rect(0, 0, b.PageSide, max(h, 1))
	p := Page{Index: b.pageNo, RGBA: b.page.SubImage(r).(*image.NRGBA)}
	if b.indexed {
		p.Indices = b.indices.SubImage(r).(*image.Gray)
		b.pack.IndexPages = append(b.pack.IndexPages, IndexPageName(b.pageNo))
	} else {
		b.pack.IndexPages = append(b.pack.IndexPages, "")
	}
	return b.sink(p)
}

func (b *Builder) newPage() error {
	if b.page != nil {
		if err := b.flush(b.PageSide); err != nil {
			return err
		}
		b.pageNo++
	}
	b.page = image.NewNRGBA(image.Rect(0, 0, b.PageSide, b.PageSide))
	b.indices = image.NewGray(image.Rect(0, 0, b.PageSide, b.PageSide))
	b.indexed = false
	b.pack.Pages = append(b.pack.Pages, PageName(b.pageNo))
	b.x, b.y, b.shelfH = 0, 0, 0
	return nil
}

// place reserves a w×h rectangle, opening shelves and pages as needed.
func (b *Builder) place(w, h int) (Rect, error) {
	if w+framePadding > b.PageSide || h+framePadding > b.PageSide {
		return Rect{}, fmt.Errorf("frame %dx%d exceeds the %d page", w, h, b.PageSide)
	}
	if b.page == nil {
		if err := b.newPage(); err != nil {
			return Rect{}, err
		}
	}
	if b.x+w+framePadding > b.PageSide {
		b.x, b.y, b.shelfH = 0, b.y+b.shelfH, 0
	}
	if b.y+h+framePadding > b.PageSide {
		if err := b.newPage(); err != nil {
			return Rect{}, err
		}
	}
	r := Rect{X: b.x, Y: b.y, W: w, H: h}
	b.x += w + framePadding
	b.shelfH = max(b.shelfH, h+framePadding)
	return r, nil
}

func pixelsOf(img image.Image) *image.NRGBA {
	r := img.Bounds()
	n := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(n, n.Bounds(), img, r.Min, draw.Src)
	return n
}

func digest(n *image.NRGBA, ix *image.Gray) uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%dx%d", n.Rect.Dx(), n.Rect.Dy())
	h.Write(n.Pix)
	if ix != nil {
		h.Write([]byte{1})
		for y := 0; y < ix.Rect.Dy(); y++ {
			h.Write(ix.Pix[y*ix.Stride:][:ix.Rect.Dx()])
		}
	}
	return h.Sum64()
}

// same compares a packed rectangle on the current page with pixels and
// indices. Rectangles on pages already written are trusted on the 64-bit
// digest and matching size.
func (b *Builder) same(p placed, n *image.NRGBA, ix *image.Gray) bool {
	if p.rect.W != n.Rect.Dx() || p.rect.H != n.Rect.Dy() || p.indexed != (ix != nil) {
		return false
	}
	if p.page != b.pageNo || b.page == nil {
		return true
	}
	for row := 0; row < p.rect.H; row++ {
		a := b.page.Pix[b.page.PixOffset(p.rect.X, p.rect.Y+row):][:4*p.rect.W]
		if !bytes.Equal(a, n.Pix[row*n.Stride:][:4*p.rect.W]) {
			return false
		}
		if ix != nil {
			ai := b.indices.Pix[b.indices.PixOffset(p.rect.X, p.rect.Y+row):][:p.rect.W]
			if !bytes.Equal(ai, ix.Pix[row*ix.Stride:][:p.rect.W]) {
				return false
			}
		}
	}
	return true
}

// AddSprite packs a sprite's frames and records it. palette, when not nil,
// is the sprite's RGB palette for its indexed frames.
func (b *Builder) AddSprite(index, id int, name string, frames []FrameImage, animations []Animation, palette *[256][3]uint8) error {
	s := Sprite{Index: index, ID: id, Name: name, Animations: animations}
	if palette != nil {
		s.Palette = EncodePalette(*palette)
	}
	for _, f := range frames {
		out := Frame{
			Name: f.Name, CanvasWidth: f.CanvasWidth, CanvasHeight: f.CanvasHeight,
			AnchorX: f.AnchorX, AnchorY: f.AnchorY,
		}
		out.OffsetX, out.OffsetY = Offsets(f.CanvasWidth, f.CanvasHeight, f.AnchorX, f.AnchorY)
		if f.Image != nil && !f.Image.Bounds().Empty() {
			n := pixelsOf(f.Image)
			ix := f.Indices
			if palette == nil || ix == nil || ix.Rect.Dx() != n.Rect.Dx() || ix.Rect.Dy() != n.Rect.Dy() {
				ix = nil
			}
			key := digest(n, ix)
			found := false
			for _, p := range b.seen[key] {
				if b.same(p, n, ix) {
					out.Page, out.Rect, out.Indexed, found = p.page, p.rect, p.indexed, true
					break
				}
			}
			if !found {
				r, err := b.place(n.Rect.Dx(), n.Rect.Dy())
				if err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				dst := image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H)
				draw.Draw(b.page, dst, n, image.Point{}, draw.Src)
				if ix != nil {
					draw.Draw(b.indices, dst, ix, ix.Rect.Min, draw.Src)
					b.indexed = true
				}
				out.Page, out.Rect, out.Indexed = b.pageNo, r, ix != nil
				b.seen[key] = append(b.seen[key], placed{b.pageNo, r, ix != nil})
			}
		}
		s.Frames = append(s.Frames, out)
	}
	b.pack.Sprites = append(b.pack.Sprites, s)
	return nil
}

// Finish flushes the last page, trimmed to its used height, and returns the
// pack index.
func (b *Builder) Finish() (*Pack, error) {
	if b.page != nil {
		if err := b.flush(b.y + b.shelfH); err != nil {
			return nil, err
		}
		b.page = nil
	}
	if !b.anyIndexed() {
		b.pack.IndexPages = nil
	}
	if err := b.pack.Validate(); err != nil {
		return nil, err
	}
	return &b.pack, nil
}

func (b *Builder) anyIndexed() bool {
	for _, p := range b.pack.IndexPages {
		if p != "" {
			return true
		}
	}
	return false
}
