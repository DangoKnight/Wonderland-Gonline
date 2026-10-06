package render

import (
	"container/list"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientimage"
)

const (
	TextureCacheBytes = 128 << 20
	textureSurface    = 1
	textureSprite     = 2
	textureIndices    = 3
	texturePalette    = 4
	texturePage       = 5
	rasterBlit        = 0
	rasterFill        = 1
	rasterAlphaFill   = 2
	rasterLight       = 3
	rasterSprite      = 4
	rasterIndexed     = 5
	rasterCutout      = 6
)

//go:embed raster.kage
var rasterSource []byte

type textureKey struct {
	Kind     int
	Source   any
	Revision uint64
}
type nativeKey struct {
	Source      any
	Rect        image.Rectangle
	Transparent bool
	Key         uint16
}

type textureEntry struct {
	key   textureKey
	image *ebiten.Image
	bytes int
}
type paletteKey struct {
	Pixels [256]uint16
	Opaque [256]bool
}

// Device owns a shared, byte-bounded texture LRU and shader for all sessions.
// Render targets are owned by their surfaces and excluded from the asset LRU.
// All methods run serially on the Ebitengine game thread.
type Device struct {
	cacheLimit                        int
	shader                            *ebiten.Shader
	cache                             map[textureKey]*list.Element
	order                             *list.List
	targets                           map[*target]bool
	bytes                             int
	translucent                       map[*ebiten.Image]bool
	Uploads, UploadedBytes, Readbacks int
}

func NewDevice() (*Device, error) {
	shader, err := ebiten.NewShader(rasterSource)
	if err != nil {
		return nil, fmt.Errorf("GPU raster shader: %w", err)
	}
	return &Device{cacheLimit: TextureCacheBytes, shader: shader, cache: map[textureKey]*list.Element{}, order: list.New(), targets: map[*target]bool{}, translucent: map[*ebiten.Image]bool{}}, nil
}

type DeviceStats struct{ TextureBytes, Textures, Targets, Uploads, UploadedBytes, Readbacks int }

func (d *Device) Stats() DeviceStats {
	return DeviceStats{d.bytes, d.order.Len(), len(d.targets), d.Uploads, d.UploadedBytes, d.Readbacks}
}

func (d *Device) CacheBytes() int { return d.bytes }
func (d *Device) Close() {
	for t := range d.targets {
		t.Close()
	}
	for e := d.order.Back(); e != nil; e = d.order.Back() {
		d.evict(e)
	}
	d.shader.Deallocate()
}
func (d *Device) evict(e *list.Element) {
	v := e.Value.(textureEntry)
	v.image.Deallocate()
	delete(d.translucent, v.image)
	d.bytes -= v.bytes
	delete(d.cache, v.key)
	d.order.Remove(e)
}
func (d *Device) cached(key textureKey, load func() (*ebiten.Image, error)) (*ebiten.Image, error) {
	if e := d.cache[key]; e != nil {
		d.order.MoveToFront(e)
		return e.Value.(textureEntry).image, nil
	}
	img, err := load()
	if err != nil || img == nil {
		return img, err
	}
	n := img.Bounds().Dx() * img.Bounds().Dy() * texturePixelBytes
	if n > d.cacheLimit {
		img.Deallocate()
		delete(d.translucent, img)
		return nil, fmt.Errorf("asset texture needs %d bytes, GPU cache limit is %d", n, d.cacheLimit)
	}
	for d.bytes+n > d.cacheLimit && d.order.Len() > 0 {
		d.evict(d.order.Back())
	}
	d.cache[key] = d.order.PushFront(textureEntry{key, img, n})
	d.bytes += n
	d.Uploads++
	d.UploadedBytes += n
	return img, nil
}
func nativeTexture(pix []uint16, w, h int) *ebiten.Image {
	data := make([]byte, w*h*texturePixelBytes)
	encode(data, pix, w, image.Rect(0, 0, w, h), false)
	img := ebiten.NewImage(w, h)
	img.WritePixels(data)
	return img
}
func (d *Device) source(s *surface.Surface) *ebiten.Image {
	if t, ok := s.Backend.(*target); ok {
		return t.image
	}
	return d.nativeSource(s, image.Rect(0, 0, s.W, s.H), false)
}
func (d *Device) Attach(s *surface.Surface) {
	if s.Backend != nil {
		return
	}
	old := &surface.Surface{W: s.W, H: s.H, Pix: s.Pix, Key: s.Key, Tiles: s.Tiles}
	t := d.newTarget(s.W, s.H)
	s.Backend = t
	s.Pix = nil
	s.Tiles = nil
	// Transfer the initial canvas once; subsequent frames never upload it.
	if len(old.Pix) > 0 {
		initial := nativeTexture(old.Pix, old.W, old.H)
		t.image.DrawImage(initial, &ebiten.DrawImageOptions{Blend: ebiten.BlendCopy})
		initial.Deallocate()
	}
}
func (d *Device) NewSurface(w, h int) *surface.Surface {
	return &surface.Surface{W: w, H: h, Backend: d.newTarget(w, h)}
}
func (d *Device) newTarget(w, h int) *target {
	t := &target{device: d, image: ebiten.NewImage(w, h), w: w, h: h}
	t.image.Fill(color.RGBA{A: 255})
	d.targets[t] = true
	return t
}

type target struct {
	device         *Device
	image, history *ebiten.Image
	w, h           int
}

func (t *target) NewSurface(w, h int) *surface.Surface { return t.device.NewSurface(w, h) }
func (t *target) Close() {
	delete(t.device.targets, t)
	if t.image != nil {
		t.image.Deallocate()
		t.image = nil
	}
	if t.history != nil {
		t.history.Deallocate()
		t.history = nil
	}
}
func (t *target) RGBA() *image.RGBA {
	t.device.Readbacks++
	out := image.NewRGBA(image.Rect(0, 0, t.w, t.h))
	t.image.ReadPixels(out.Pix)
	return out
}
func nativeColor(v uint16) []float32 {
	return []float32{float32(v >> 11), float32(v >> 5 & 63), float32(v & 31)}
}
func dimensions(r image.Rectangle) []float32 { return []float32{float32(r.Dx()), float32(r.Dy())} }
func origin(r image.Rectangle) []float32     { return []float32{float32(r.Min.X), float32(r.Min.Y)} }

// paint samples assets with nearest integer coordinates. Destination-reading
// effects copy only their affected region into a GPU scratch image, preserving
// legacy integer arithmetic without CPU readback or simultaneous read/write.
func (t *target) paint(op int, rect, mapping, source image.Rectangle, src, palette *ebiten.Image, key uint16, transparent bool, ink uint16, amount int) {
	rect = rect.Intersect(image.Rect(0, 0, t.w, t.h))
	if rect.Empty() {
		return
	}
	var old *ebiten.Image
	if op == rasterAlphaFill || op == rasterLight || op == rasterSprite || src == t.image {
		if t.history == nil {
			t.history = ebiten.NewImage(t.w, t.h)
		}
		copyRect := rect
		if src == t.image {
			copyRect = image.Rect(0, 0, t.w, t.h)
		}
		o := &ebiten.DrawImageOptions{Blend: ebiten.BlendCopy}
		o.GeoM.Translate(float64(copyRect.Min.X), float64(copyRect.Min.Y))
		t.history.DrawImage(t.image.SubImage(copyRect).(*ebiten.Image), o)
		old = t.history
		if src == t.image {
			src = t.history
		}
	}
	trans := float32(0)
	if transparent {
		trans = 1
	}
	uniforms := map[string]any{"Operation": float32(op), "DestinationOrigin": origin(mapping), "DestinationSize": dimensions(mapping), "SourceOrigin": origin(source), "SourceSize": dimensions(source), "Key": nativeColor(key), "Transparent": trans, "Ink": nativeColor(ink), "Amount": float32(amount)}
	vertices := []ebiten.Vertex{
		{DstX: float32(rect.Min.X), DstY: float32(rect.Min.Y), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: float32(rect.Max.X), DstY: float32(rect.Min.Y), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: float32(rect.Min.X), DstY: float32(rect.Max.Y), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: float32(rect.Max.X), DstY: float32(rect.Max.Y), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	t.image.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, t.device.shader, &ebiten.DrawTrianglesShaderOptions{Images: [4]*ebiten.Image{src, old, palette}, Uniforms: uniforms})
}
func (t *target) Fill(r image.Rectangle, v uint16) {
	t.paint(rasterFill, r, r, image.Rect(0, 0, 1, 1), nil, nil, 0, false, v, 0)
}
func (t *target) FillAlpha(r image.Rectangle, c uint32, alpha int) {
	if alpha >= 255 {
		t.Fill(r, surface.TColor(c))
		return
	}
	t.paint(rasterAlphaFill, r, r, image.Rect(0, 0, 1, 1), nil, nil, 0, false, surface.TColor(c), alpha)
}
func (t *target) DrawRect(x, y int, r image.Rectangle, s *surface.Surface, transparent bool) {
	r = r.Intersect(image.Rect(0, 0, s.W, s.H))
	dst := image.Rect(x, y, x+r.Dx(), y+r.Dy())
	if r.Empty() || dst.Intersect(image.Rect(0, 0, t.w, t.h)).Empty() {
		return
	}
	if s.Tiles != nil {
		t.drawTiles(dst, r, s, transparent, rasterBlit, 0)
		return
	}
	if s.Backend == nil && (s.W > clientimage.TileSide || s.H > clientimage.TileSide) {
		t.drawSurfaceTiles(dst, r, s, transparent, rasterBlit, 0)
		return
	}
	if s.Backend == nil {
		img := t.device.nativeSource(s, image.Rect(0, 0, s.W, s.H), transparent)
		t.blit(dst, dst, r, img, transparent)
		return
	}
	img := t.device.source(s)
	if !transparent && img != t.image {
		t.blit(dst, dst, r, img, false)
		return
	}
	t.paint(rasterBlit, dst, dst, r, img, nil, s.Key, transparent, 0, 0)
}
func (t *target) DrawStretch(r image.Rectangle, s *surface.Surface, transparent bool) {
	if r.Empty() || s.W == 0 || s.H == 0 {
		return
	}
	src := image.Rect(0, 0, s.W, s.H)
	if s.Tiles != nil {
		if !transparent {
			t.Fill(r, 0)
		} else if s.Key != 0 {
			t.fillTileHoles(r, src, s.Tiles)
		}
		t.drawTiles(r, src, s, transparent, rasterBlit, 0)
		return
	}
	if s.Backend == nil && (s.W > clientimage.TileSide || s.H > clientimage.TileSide) {
		t.drawSurfaceTiles(r, src, s, transparent, rasterBlit, 0)
		return
	}
	t.paint(rasterBlit, r, r, src, t.device.source(s), nil, s.Key, transparent, 0, 0)
}
func (t *target) DrawLight(x, y int, r image.Rectangle, s *surface.Surface, level int) {
	r = r.Intersect(image.Rect(0, 0, s.W, s.H))
	dst := image.Rect(x, y, x+r.Dx(), y+r.Dy())
	if r.Empty() || dst.Intersect(image.Rect(0, 0, t.w, t.h)).Empty() {
		return
	}
	if s.Tiles != nil {
		t.drawTiles(dst, r, s, true, rasterLight, level)
		return
	}
	if s.Backend == nil && (s.W > clientimage.TileSide || s.H > clientimage.TileSide) {
		t.drawSurfaceTiles(dst, r, s, true, rasterLight, level)
		return
	}
	t.paint(rasterLight, dst, dst, r, t.device.source(s), nil, s.Key, true, 0, level)
}
func (t *target) drawTiles(dst, r image.Rectangle, s *surface.Surface, transparent bool, op, amount int) {
	visible := dst.Intersect(image.Rect(0, 0, t.w, t.h))
	if visible.Empty() {
		return
	}
	// Include only source texels sampled by the clipped destination.
	request := image.Rect(r.Min.X+(visible.Min.X-dst.Min.X)*r.Dx()/dst.Dx(), r.Min.Y+(visible.Min.Y-dst.Min.Y)*r.Dy()/dst.Dy(), r.Min.X+(visible.Max.X-1-dst.Min.X)*r.Dx()/dst.Dx()+1, r.Min.Y+(visible.Max.Y-1-dst.Min.Y)*r.Dy()/dst.Dy()+1)
	err := s.Tiles.VisitTiles(request, func(v clientimage.TileView) error {
		img, err := t.device.cached(textureKey{Kind: texturePage, Source: nativeKey{Source: v.Path, Transparent: transparent && op == rasterBlit, Key: s.Key}}, func() (*ebiten.Image, error) {
			pix, w, h, err := v.Pixels()
			if err != nil {
				return nil, err
			}
			return keyedTexture(pix, w, h, transparent && op == rasterBlit, s.Key), nil
		})
		if err != nil {
			return err
		}
		part := v.Destination.Add(request.Min)
		// ceil maps source tile boundaries onto nearest-neighbor destination pixels.
		ceil := func(a, b int) int { return (a + b - 1) / b }
		at := image.Rect(dst.Min.X+ceil((part.Min.X-r.Min.X)*dst.Dx(), r.Dx()), dst.Min.Y+ceil((part.Min.Y-r.Min.Y)*dst.Dy(), r.Dy()), dst.Min.X+ceil((part.Max.X-r.Min.X)*dst.Dx(), r.Dx()), dst.Min.Y+ceil((part.Max.Y-r.Min.Y)*dst.Dy(), r.Dy()))
		pageOrigin := v.Source.Min.Sub(part.Min).Add(r.Min)
		source := image.Rectangle{Min: pageOrigin, Max: pageOrigin.Add(r.Size())}
		if !v.Source.In(img.Bounds()) {
			return fmt.Errorf("GPU tile outside page")
		}
		if op == rasterBlit && dst.Size() == r.Size() {
			t.blit(at, dst, source, img, transparent)
		} else {
			t.paint(op, at, dst, source, img, nil, s.Key, transparent, 0, amount)
		}
		return nil
	})
	if err != nil {
		log.Printf("GPU image tiles: %v", err)
	}
}
func (t *target) DrawSprite(x, y int, key any, load func() (*image.NRGBA, error)) error {
	img, err := t.device.cached(textureKey{Kind: textureSprite, Source: key}, func() (*ebiten.Image, error) {
		m, err := load()
		if err != nil || m == nil {
			return nil, err
		}
		b := m.Bounds()
		data := make([]byte, b.Dx()*b.Dy()*texturePixelBytes)
		for y := 0; y < b.Dy(); y++ {
			copy(data[y*b.Dx()*texturePixelBytes:(y+1)*b.Dx()*texturePixelBytes], m.Pix[m.PixOffset(b.Min.X, b.Min.Y+y):m.PixOffset(b.Min.X, b.Min.Y+y)+b.Dx()*texturePixelBytes])
		}
		img := ebiten.NewImage(b.Dx(), b.Dy())
		img.WritePixels(data)
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				a := data[(y*b.Dx()+x)*texturePixelBytes+3]
				if a != 0 && a != 255 {
					t.device.translucent[img] = true
					break
				}
			}
		}
		return img, nil
	})
	if err != nil || img == nil {
		return err
	}
	src := img.Bounds()
	dst := src.Add(image.Pt(x, y))
	op := rasterSprite
	if !t.device.translucent[img] {
		op = rasterCutout
	}
	t.paint(op, dst, dst, src, img, nil, 0, false, 0, 0)
	return nil
}
func (t *target) DrawIndexed(x, y int, key any, m *image.Gray, lut [256]uint16, opaque [256]bool) {
	img, err := t.device.cached(textureKey{Kind: textureIndices, Source: key}, func() (*ebiten.Image, error) {
		b := m.Bounds()
		data := make([]byte, b.Dx()*b.Dy()*texturePixelBytes)
		for row := 0; row < b.Dy(); row++ {
			for col := 0; col < b.Dx(); col++ {
				i := (row*b.Dx() + col) * texturePixelBytes
				data[i], data[i+3] = m.GrayAt(b.Min.X+col, b.Min.Y+row).Y, 255
			}
		}
		img := ebiten.NewImage(b.Dx(), b.Dy())
		img.WritePixels(data)
		return img, nil
	})
	if err != nil {
		panic(err)
	}
	pal, err := t.device.cached(textureKey{Kind: texturePalette, Source: paletteKey{lut, opaque}}, func() (*ebiten.Image, error) {
		data := make([]byte, len(lut)*texturePixelBytes)
		for i, v := range lut {
			c := surface.Expand(v)
			data[i*texturePixelBytes], data[i*texturePixelBytes+1], data[i*texturePixelBytes+2] = c.R, c.G, c.B
			if opaque[i] {
				data[i*texturePixelBytes+3] = 255
			}
		}
		img := ebiten.NewImage(len(lut), 1)
		img.WritePixels(data)
		return img, nil
	})
	if err != nil {
		panic(err)
	}
	src := img.Bounds()
	dst := src.Add(image.Pt(x, y))
	t.paint(rasterIndexed, dst, dst, src, img, pal, 0, false, 0, 0)
}

// nativeSource uploads fixed grid-aligned tiles so scrolling a loose editable
// map cannot allocate a whole-map GPU texture or create camera-specific entries.
func (d *Device) nativeSource(s *surface.Surface, r image.Rectangle, transparent bool) *ebiten.Image {
	key := nativeKey{Source: s, Rect: r, Transparent: transparent, Key: s.Key}
	img, err := d.cached(textureKey{Kind: textureSurface, Source: key, Revision: s.Revision}, func() (*ebiten.Image, error) {
		pix := make([]uint16, r.Dx()*r.Dy())
		for y := 0; y < r.Dy(); y++ {
			copy(pix[y*r.Dx():(y+1)*r.Dx()], s.Pix[(r.Min.Y+y)*s.W+r.Min.X:(r.Min.Y+y)*s.W+r.Max.X])
		}
		return keyedTexture(pix, r.Dx(), r.Dy(), transparent, s.Key), nil
	})
	if err != nil {
		panic(err)
	}
	return img
}
func keyedTexture(pix []uint16, w, h int, transparent bool, key uint16) *ebiten.Image {
	data := make([]byte, w*h*texturePixelBytes)
	encode(data, pix, w, image.Rect(0, 0, w, h), false)
	if transparent {
		for i, v := range pix {
			if v == key {
				clear(data[i*texturePixelBytes : (i+1)*texturePixelBytes])
			}
		}
	}
	img := ebiten.NewImage(w, h)
	img.WritePixels(data)
	return img
}

// Ordinary 1:1 native blits use Ebitengine's built-in sprite path. Compatible
// glyph/UI calls can batch on its internal texture atlas without unique shader
// uniforms. Scaling/effects retain the exact integer compatibility shader.
func (t *target) blit(rect, mapping, source image.Rectangle, img *ebiten.Image, transparent bool) {
	visible := rect.Intersect(image.Rect(0, 0, t.w, t.h))
	if visible.Empty() {
		return
	}
	at := source.Min.Add(visible.Min.Sub(mapping.Min))
	src := image.Rectangle{Min: at, Max: at.Add(visible.Size())}
	o := &ebiten.DrawImageOptions{}
	if !transparent {
		o.Blend = ebiten.BlendCopy
	}
	o.GeoM.Translate(float64(visible.Min.X), float64(visible.Min.Y))
	t.image.DrawImage(img.SubImage(src).(*ebiten.Image), o)
}
func (t *target) drawSurfaceTiles(dst, r image.Rectangle, s *surface.Surface, transparent bool, op, amount int) {
	visible := dst.Intersect(image.Rect(0, 0, t.w, t.h))
	if visible.Empty() {
		return
	}
	request := image.Rect(r.Min.X+(visible.Min.X-dst.Min.X)*r.Dx()/dst.Dx(), r.Min.Y+(visible.Min.Y-dst.Min.Y)*r.Dy()/dst.Dy(), r.Min.X+(visible.Max.X-1-dst.Min.X)*r.Dx()/dst.Dx()+1, r.Min.Y+(visible.Max.Y-1-dst.Min.Y)*r.Dy()/dst.Dy()+1)
	for y := request.Min.Y / clientimage.TileSide * clientimage.TileSide; y < request.Max.Y; y += clientimage.TileSide {
		for x := request.Min.X / clientimage.TileSide * clientimage.TileSide; x < request.Max.X; x += clientimage.TileSide {
			tile := image.Rect(x, y, min(x+clientimage.TileSide, s.W), min(y+clientimage.TileSide, s.H))
			part := tile.Intersect(request)
			ceil := func(a, b int) int { return (a + b - 1) / b }
			at := image.Rect(dst.Min.X+ceil((part.Min.X-r.Min.X)*dst.Dx(), r.Dx()), dst.Min.Y+ceil((part.Min.Y-r.Min.Y)*dst.Dy(), r.Dy()), dst.Min.X+ceil((part.Max.X-r.Min.X)*dst.Dx(), r.Dx()), dst.Min.Y+ceil((part.Max.Y-r.Min.Y)*dst.Dy(), r.Dy()))
			source := r.Sub(tile.Min)
			img := t.device.nativeSource(s, tile, transparent && op == rasterBlit)
			if op == rasterBlit && dst.Size() == r.Size() {
				t.blit(at, dst, source, img, transparent)
			} else {
				t.paint(op, at, dst, source, img, nil, s.Key, transparent, 0, amount)
			}
		}
	}
}

// CPU DrawStretch materializes omitted compiled pixels as native zero. With a
// nonzero color key those holes are opaque, while keyed pixels within existing
// pages must still preserve the old destination. Fill only the uncovered areas.
func (t *target) fillTileHoles(dst, r image.Rectangle, m *clientimage.Image) {
	holes := []image.Rectangle{dst.Intersect(image.Rect(0, 0, t.w, t.h))}
	_ = m.VisitTiles(r, func(v clientimage.TileView) error {
		part := v.Destination.Add(r.Min)
		ceil := func(a, b int) int { return (a + b - 1) / b }
		cover := image.Rect(dst.Min.X+ceil((part.Min.X-r.Min.X)*dst.Dx(), r.Dx()), dst.Min.Y+ceil((part.Min.Y-r.Min.Y)*dst.Dy(), r.Dy()), dst.Min.X+ceil((part.Max.X-r.Min.X)*dst.Dx(), r.Dx()), dst.Min.Y+ceil((part.Max.Y-r.Min.Y)*dst.Dy(), r.Dy()))
		next := make([]image.Rectangle, 0, len(holes))
		for _, h := range holes {
			i := h.Intersect(cover)
			if i.Empty() {
				next = append(next, h)
				continue
			}
			for _, piece := range []image.Rectangle{image.Rect(h.Min.X, h.Min.Y, h.Max.X, i.Min.Y), image.Rect(h.Min.X, i.Max.Y, h.Max.X, h.Max.Y), image.Rect(h.Min.X, i.Min.Y, i.Min.X, i.Max.Y), image.Rect(i.Max.X, i.Min.Y, h.Max.X, i.Max.Y)} {
				if !piece.Empty() {
					next = append(next, piece)
				}
			}
		}
		holes = next
		return nil
	})
	for _, h := range holes {
		t.Fill(h, 0)
	}
}
