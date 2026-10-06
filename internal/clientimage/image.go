package clientimage

import (
	"bytes"
	"container/list"
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"sync"
	"wonderland-gonline/internal/clientfs"
)

const CacheBytes = 64 << 20

type cacheEntry struct {
	path string
	data *chunk
}

var pages = struct {
	sync.Mutex
	entries      map[string]*list.Element
	order        *list.List
	bytes        int
	hits, misses uint64
}{entries: map[string]*list.Element{}, order: list.New()}

type CacheStats struct {
	Bytes, Pages int
	Hits, Misses uint64
}

func Stats() CacheStats {
	pages.Lock()
	defer pages.Unlock()
	return CacheStats{pages.bytes, len(pages.entries), pages.hits, pages.misses}
}
func ClearCache() {
	pages.Lock()
	defer pages.Unlock()
	pages.entries = map[string]*list.Element{}
	pages.order.Init()
	pages.bytes = 0
	pages.hits = 0
	pages.misses = 0
}
func page(path string) (*chunk, error) {
	pages.Lock()
	defer pages.Unlock()
	if e := pages.entries[path]; e != nil {
		pages.hits++
		pages.order.MoveToFront(e)
		return e.Value.(cacheEntry).data, nil
	}
	f, err := clientfs.Open(path)
	if err != nil {
		return nil, err
	}
	c, err := decodeChunk(f)
	f.Close()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	pages.misses++
	for pages.bytes+c.size() > CacheBytes && pages.order.Len() > 0 {
		e := pages.order.Back()
		entry := e.Value.(cacheEntry)
		pages.bytes -= entry.data.size()
		delete(pages.entries, entry.path)
		pages.order.Remove(e)
	}
	pages.entries[path] = pages.order.PushFront(cacheEntry{path, c})
	pages.bytes += c.size()
	return c, nil
}

// Image is immutable metadata, not a decoded canvas. Region reads retain only
// shared pages in the byte-bounded LRU, even when several sessions show a map.
type Image struct {
	root       string
	descriptor Descriptor
	paths      []string
	region     image.Rectangle
}

func Open(path string) (*Image, error) {
	f, err := clientfs.Open(path + DescriptorSuffix)
	if err != nil {
		return nil, err
	}
	d, err := decodeDescriptor(f)
	f.Close()
	if err != nil {
		return nil, err
	}
	root, err := assetRoot(path, d.Source)
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(d.Tiles))
	for i, tile := range d.Tiles {
		paths[i] = filepath.Join(root, filepath.FromSlash(tile.File))
	}
	return &Image{root: root, descriptor: d, paths: paths, region: image.Rect(0, 0, d.Width, d.Height)}, nil
}
func (m *Image) Bounds() image.Rectangle { return image.Rect(0, 0, m.region.Dx(), m.region.Dy()) }
func (m *Image) SubImage(r image.Rectangle) (*Image, error) {
	if r.Empty() || !r.In(m.Bounds()) {
		return nil, fmt.Errorf("image region outside canvas")
	}
	out := *m
	out.region = r.Add(m.region.Min)
	return &out, nil
}

// visit walks intersecting pages once, avoiding page lookups for every pixel.
func (m *Image) visit(r image.Rectangle, fn func(*chunk, image.Rectangle, image.Rectangle)) error {
	r = r.Intersect(m.Bounds())
	absolute := r.Add(m.region.Min)
	for i, t := range m.descriptor.Tiles {
		overlap := absolute.Intersect(t.Rect)
		if overlap.Empty() {
			continue
		}
		c, err := page(m.paths[i])
		if err != nil {
			return err
		}
		if !t.Page.In(image.Rect(0, 0, c.width, c.height)) {
			return fmt.Errorf("image tile outside page")
		}
		src := overlap.Sub(t.Rect.Min).Add(t.Page.Min)
		dst := overlap.Sub(m.region.Min).Sub(r.Min)
		fn(c, src, dst)
	}
	return nil
}
func (m *Image) NRGBA(r image.Rectangle) (*image.NRGBA, error) {
	r = r.Intersect(m.Bounds())
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))

	err := m.visit(r, func(c *chunk, src, dst image.Rectangle) {
		for y := 0; y < src.Dy(); y++ {
			for x := 0; x < src.Dx(); x++ {
				p := c.colorAt((src.Min.Y+y)*c.width + src.Min.X + x)
				at := (dst.Min.Y+y)*out.Stride + (dst.Min.X+x)*4
				out.Pix[at], out.Pix[at+1], out.Pix[at+2], out.Pix[at+3] = p.R, p.G, p.B, p.A
			}
		}
	})
	return out, err
}
func (m *Image) Pixels(r image.Rectangle, mode Mode) ([]uint16, error) {
	r = r.Intersect(m.Bounds())
	out := make([]uint16, r.Dx()*r.Dy())

	err := m.visit(r, func(c *chunk, src, dst image.Rectangle) {
		for y := 0; y < src.Dy(); y++ {
			start := (src.Min.Y+y)*c.width + src.Min.X
			at := (dst.Min.Y+y)*r.Dx() + dst.Min.X
			if mode == Plain {
				copy(out[at:at+src.Dx()], c.plain[start:start+src.Dx()])
			} else {
				for x := 0; x < src.Dx(); x++ {
					out[at+x] = c.keyedAt(start + x)
				}
			}
		}
	})
	return out, err
}

// ColorAt is useful for inspection; rendering should use region reads.
func (m *Image) ColorAt(x, y int) (color.NRGBA, error) {
	n, err := m.NRGBA(image.Rect(x, y, x+1, y+1))
	if err != nil {
		return color.NRGBA{}, err
	}
	if len(n.Pix) == 0 {
		return color.NRGBA{}, nil
	}
	return n.NRGBAAt(0, 0), nil
}

// Files returns the physical page dependencies of this immutable image view.
func (m *Image) Files() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, tile := range m.descriptor.Tiles {
		if !seen[tile.File] {
			seen[tile.File] = true
			out = append(out, tile.File)
		}
	}
	return out
}

// RemapDescriptor changes content addresses after lossless recompression.
func RemapDescriptor(data []byte, files map[string]string) ([]byte, error) {
	d, err := decodeDescriptor(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	for i := range d.Tiles {
		if name, ok := files[d.Tiles[i].File]; ok {
			d.Tiles[i].File = name
		}
	}
	return EncodeDescriptor(d)
}

// CopyPixels blits prepared native rows directly into the destination. It avoids
// a temporary viewport image and keeps the cached page buffers immutable.
func (m *Image) CopyPixels(r image.Rectangle, dst []uint16, stride int, origin image.Point, transparent bool, key uint16) error {
	if r.Empty() {
		return nil
	}
	if stride <= 0 || len(dst)%stride != 0 || !r.In(m.Bounds()) || origin.X < 0 || origin.Y < 0 || origin.X+r.Dx() > stride || origin.Y+r.Dy() > len(dst)/stride {
		return fmt.Errorf("invalid image blit rectangle")
	}
	return m.visit(r, func(c *chunk, src, at image.Rectangle) {
		for y := 0; y < src.Dy(); y++ {
			start := (src.Min.Y+y)*c.width + src.Min.X
			offset := (origin.Y+at.Min.Y+y)*stride + origin.X + at.Min.X
			pixels := c.plain[start : start+src.Dx()]
			target := dst[offset : offset+src.Dx()]
			if !transparent {
				copy(target, pixels)
			} else {
				for x, v := range pixels {
					if v != key {
						target[x] = v
					}
				}
			}
		}
	})
}

// TileView describes a visible rectangle in a content-addressed page. It does
// not decode the page: GPU callers can check their shared texture cache first.
// Destination is relative to the requested rectangle's origin.
type TileView struct {
	Path                string
	Source, Destination image.Rectangle
}

func (m *Image) VisitTiles(r image.Rectangle, fn func(TileView) error) error {
	r = r.Intersect(m.Bounds())
	absolute := r.Add(m.region.Min)
	for i, t := range m.descriptor.Tiles {
		overlap := absolute.Intersect(t.Rect)
		if overlap.Empty() {
			continue
		}
		v := TileView{Path: m.paths[i], Source: overlap.Sub(t.Rect.Min).Add(t.Page.Min), Destination: overlap.Sub(m.region.Min).Sub(r.Min)}
		if err := fn(v); err != nil {
			return err
		}
	}
	return nil
}

// Pixels returns an independent full-page native buffer for a texture upload.
func (v TileView) Pixels() ([]uint16, int, int, error) {
	c, err := page(v.Path)
	if err != nil {
		return nil, 0, 0, err
	}
	if !v.Source.In(image.Rect(0, 0, c.width, c.height)) {
		return nil, 0, 0, fmt.Errorf("image tile outside page")
	}
	return append([]uint16(nil), c.plain...), c.width, c.height, nil
}
