// Package picdb ports TSe_CachePicDB (unit se_BmpCacheAndDraw in the
// reference decompile): every interface bitmap, registered by name,
// converted to RGB565 and drawn colour-keyed.
//
// Reference functions (aLogin_decompiled.c):
//
//	FUN_004785e4  LoadDir: register every *.bmp in a directory
//	FUN_0047ce58  Load: register one bitmap, loading now or later
//	FUN_0047b274  convert: 24-bit bitmap to RGB565 with the key rules
//	FUN_0047885c  Find: index of a name, -1 when absent
//	FUN_00477e18  Draw: whole image at a point
//	FUN_00477c9c  DrawRect: part of an image at a point
package picdb

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/clientimage"

	_ "golang.org/x/image/bmp"
	_ "image/png"
	"wonderland-gonline/client/wlo/surface"
)

// Entry is one 0x24-byte record of the picture list (+0x3c).
type Entry struct {
	Image      *surface.Surface // +0x00 pixel buffer; nil until loaded
	H, W       int              // +0x08, +0x0c
	Loaded     bool             // +0x11 load requested
	Registered bool             // +0x12
	Name       string           // +0x14
	Path       string           // +0x1c directory
	Tint       uint32           // +0x20 $00BBGGRR added to every pixel
}

// DB is TSe_CachePicDB. Names are kept in a case-insensitive sorted list
// (+0x40, a TStringList with Sorted and dupError) mapping to Entries.
type DB struct {
	Entries []Entry
	index   map[string]int
	// RGB555 selects the 5-bit green conversion (DAT_007d69f8); the
	// original sets it for 15-bit display modes.
	RGB555 bool
	// Missing records files that Load could not find; the original writes
	// them to its debug list.
	Missing []string
}

func New() *DB { return &DB{index: map[string]int{}} }

func key(name string) string { return strings.ToLower(name) }

// LoadDir is FUN_004785e4: every *.bmp file in dir, by file name without
// extension, in sorted order.
func (db *DB) LoadDir(dir string, loadNow bool, tint uint32) error {
	entries, err := clientfs.ReadDir(dir)
	if err != nil {
		return nil // FindFirst failing registers nothing.
	}
	var names []string
	seen := map[string]bool{}
	for _, e := range entries {
		logical := strings.TrimSuffix(e.Name(), clientimage.DescriptorSuffix)
		if e.IsDir() || (!strings.EqualFold(filepath.Ext(logical), ".bmp") && !strings.HasSuffix(strings.ToLower(logical), ".bmp.png")) {
			continue
		}
		n := logical
		if strings.HasSuffix(strings.ToLower(n), ".png") {
			n = n[:len(n)-4]
		}
		n = strings.TrimSuffix(n, filepath.Ext(n))
		if !seen[key(n)] {
			seen[key(n)] = true
			names = append(names, n)
		}
	}
	sort.Slice(names, func(i, j int) bool { return key(names[i]) < key(names[j]) })
	for _, n := range names {
		if err := db.Load(dir, n, loadNow, tint); err != nil {
			return err
		}
	}
	return nil
}

// Load is FUN_0047ce58: register dir/name.bmp. A name registered before is
// left alone, which lets an earlier directory override a later one. With
// loadNow false only the record is made; pixels load on first use.
func (db *DB) Load(dir, name string, loadNow bool, tint uint32) error {
	i, ok := db.index[key(name)]
	if ok {
		e := &db.Entries[i]
		if e.Registered {
			return nil
		}
		e.Loaded, e.Registered, e.Path, e.Tint = loadNow, true, dir, tint
		if !loadNow {
			return nil
		}
	}
	path := bitmapPath(dir, name)
	if _, err := clientfs.Stat(path); err != nil {
		if _, compiledErr := clientfs.Stat(path + clientimage.DescriptorSuffix); compiledErr != nil {
			db.Missing = append(db.Missing, path)
			return nil
		}
	}
	var img image.Image
	var width, height int
	if compiled, err := clientimage.Open(path); err == nil {
		bounds := compiled.Bounds()
		width, height = bounds.Dx(), bounds.Dy()
		if loadNow && tint == 0 && !db.RGB555 {
			pixels, err := compiled.Pixels(bounds, clientimage.Keyed)
			if err != nil {
				return err
			}
			if !ok {
				i = len(db.Entries)
				db.Entries = append(db.Entries, Entry{})
				db.index[key(name)] = i
			}
			db.Entries[i] = Entry{Image: &surface.Surface{W: width, H: height, Pix: pixels}, W: width, H: height, Loaded: true, Registered: true, Name: name, Path: dir, Tint: tint}
			return nil
		}
		if loadNow {
			decoded, err := compiled.NRGBA(bounds)
			if err != nil {
				return err
			}
			img = decoded
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if loadNow {
		decoded, err := decodeBMP(path)
		if err != nil {
			return err
		}
		img = decoded
		width, height = img.Bounds().Dx(), img.Bounds().Dy()
	} else {
		f, err := clientfs.Open(path)
		if err != nil {
			return err
		}
		cfg, _, err := image.DecodeConfig(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		width, height = cfg.Width, cfg.Height
	}
	if !ok {
		i = len(db.Entries)
		db.Entries = append(db.Entries, Entry{})
		db.index[key(name)] = i
	}
	e := &db.Entries[i]
	e.W, e.H, e.Registered, e.Loaded, e.Name, e.Path, e.Tint = width, height, true, loadNow, name, dir, tint
	if loadNow {
		e.Image = db.convert(img, tint)
	}
	return nil
}

func bitmapPath(dir, name string) string {
	entries, _ := clientfs.ReadDir(dir)
	for _, suffix := range []string{".bmp.png", ".bmp"} {
		for _, entry := range entries {
			if !entry.IsDir() && strings.EqualFold(strings.TrimSuffix(entry.Name(), clientimage.DescriptorSuffix), name+suffix) {
				return filepath.Join(dir, strings.TrimSuffix(entry.Name(), clientimage.DescriptorSuffix))
			}
		}
	}
	return filepath.Join(dir, name+".bmp.png")
}

func decodeBMP(path string) (image.Image, error) {
	if compiled, err := clientimage.Open(path); err == nil {
		return compiled.NRGBA(compiled.Bounds())
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	f, err := clientfs.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return img, nil
}

// convert is FUN_0047b274's pixel loop over the bitmap at pf24bit:
//   - all channels below 8: blue becomes 8, so black never equals the key;
//   - pure green (0,255,0): 0, the transparent key;
//   - (0,0,8): kept;
//   - otherwise the tint is added to each channel, saturating at 255;
//
// then packed as RGB565 (or RGB555 when the display is 15-bit).
func (db *DB) convert(img image.Image, tint uint32) *surface.Surface {
	b := img.Bounds()
	s := surface.New(b.Dx(), b.Dy())
	t0, t1, t2 := tint&0xff, tint>>8&0xff, tint>>16&0xff
	add := func(v uint8, t uint32) uint8 {
		if uint32(v)+t < 0x100 {
			return v + uint8(t)
		}
		return 0xff
	}
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			pixel := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			if pixel.A == 0 {
				continue
			}
			r, g, bl := pixel.R, pixel.G, pixel.B
			if r < 8 && g < 8 && bl < 8 {
				bl = 8
			}
			switch {
			case r == 0 && g == 0xff && bl == 0:
				r, g, bl = 0, 0, 0
			case r == 0 && g == 0 && bl == 8:
			default:
				// The tint's low byte is applied to the bitmap's first byte
				// (blue), the next to green and the third to red.
				bl, g, r = add(bl, t0), add(g, t1), add(r, t2)
			}
			gg := uint16(g >> 2)
			if db.RGB555 {
				gg = uint16(g >> 3)
			}
			s.Pix[y*s.W+x] = uint16(r>>3)<<11 | gg<<5 | uint16(bl>>3)
		}
	}
	return s
}

// Add registers a decoded picture under name, converted with the key
// rules and loaded now, as the original registers the pictures of its
// image archives (pic\*.BMg) by the names in 1.bls. A name registered
// before is left alone.
func (db *DB) Add(name string, img image.Image) {
	if _, ok := db.index[key(name)]; ok {
		return
	}
	s := db.convert(img, 0)
	db.index[key(name)] = len(db.Entries)
	db.Entries = append(db.Entries, Entry{Image: s, W: s.W, H: s.H, Loaded: true, Registered: true, Name: name})
}

// Find is FUN_0047885c without its on-demand archive loading: the index of
// a registered name, or -1. The original also loads numbered names
// (1000-59999) and "icon_sk*" from archives on first use; that path is not
// ported yet.
func (db *DB) Find(name string) int {
	if i, ok := db.index[key(name)]; ok {
		return i
	}
	return -1
}

// image returns an entry's pixels, loading a deferred entry on first use.
func (db *DB) image(i int) *surface.Surface {
	if i < 0 || i >= len(db.Entries) {
		return nil
	}
	e := &db.Entries[i]
	if e.Image == nil && e.Registered {
		path := bitmapPath(e.Path, e.Name)
		if compiled, err := clientimage.Open(path); err == nil && e.Tint == 0 && !db.RGB555 {
			if pix, err := compiled.Pixels(compiled.Bounds(), clientimage.Keyed); err == nil {
				e.Image = &surface.Surface{W: e.W, H: e.H, Pix: pix}
			}
			return e.Image
		}
		if img, err := decodeBMP(path); err == nil {
			e.Image = db.convert(img, e.Tint)
		}
	}
	return e.Image
}

// Size returns an entry's width and height (record +0x0c, +0x08).
func (db *DB) Size(i int) (int, int) {
	if i < 0 || i >= len(db.Entries) {
		return 0, 0
	}
	return db.Entries[i].W, db.Entries[i].H
}

// Draw is FUN_00477e18: the whole image at (x, y). Cached surfaces use
// colour key 0.
func (db *DB) Draw(dst *surface.Surface, i, x, y int, transparent bool) {
	if src := db.image(i); src != nil {
		dst.Draw(x, y, src, transparent)
	}
}

// DrawRect is FUN_00477c9c: the source rectangle r of the image at (x, y),
// with r first intersected with the image bounds.
func (db *DB) DrawRect(dst *surface.Surface, i, x, y int, r image.Rectangle, transparent bool) {
	if src := db.image(i); src != nil {
		dst.DrawRect(x, y, r.Intersect(image.Rect(0, 0, src.W, src.H)), src, transparent)
	}
}

// Pixel is FUN_0045e90c: an image's 16-bit pixel at (x, y), 0 outside it.
func (db *DB) Pixel(i, x, y int) uint16 {
	src := db.image(i)
	if src == nil || x < 0 || y < 0 || x >= src.W || y >= src.H {
		return 0
	}
	return src.Pix[y*src.W+x]
}

// DrawStretch is FUN_0046012c: the whole image scaled into r.
func (db *DB) DrawStretch(dst *surface.Surface, i int, r image.Rectangle, transparent bool) {
	if src := db.image(i); src != nil {
		dst.DrawStretch(r, src, transparent)
	}
}

// DrawLight is FUN_0045f0f0: the source rectangle r added to dst at the
// light level (surface.DrawLight).
func (db *DB) DrawLight(dst *surface.Surface, i, x, y int, r image.Rectangle, level int) {
	if src := db.image(i); src != nil {
		dst.DrawLight(x, y, r, src, level)
	}
}

// Opaque is FUN_004784ec with key 0: whether the pixel at (x, y) is not the
// transparent colour. A deferred image is loaded first (FUN_0047b1a0).
func (db *DB) Opaque(i, x, y int) bool {
	src := db.image(i)
	if src == nil || x < 0 || y < 0 || x >= src.W || y >= src.H {
		return false
	}
	return src.Pix[y*src.W+x] != 0
}

// Gauge geometry of FUN_00463d98: the fill's edge runs from a fixed point
// near the top to a point that moves with the percentage, so an orb-shaped
// gauge fills along its arc. Pictures wider than 100 pixels move the
// fixed point and the moving point's start right.
const (
	gaugeFixedX     = 0x2e
	gaugeWideShift  = 0xc
	gaugeWideLimit  = 100
	gaugeStartX     = 9
	gaugeWideStartX = 0x15
	gaugeSpanX      = 0x52
	gaugeSpanY      = 0x18
	gaugeVertical   = 999.0
)

// DrawGauge is FUN_00463d98 then FUN_0045e1c4: the picture with the part
// beyond percent (0..100) made transparent, drawn colour-keyed at (x, y).
func (db *DB) DrawGauge(dst *surface.Surface, i, x, y, percent int) {
	src := db.image(i)
	if src == nil {
		return
	}
	fixedX, startX := gaugeFixedX, gaugeStartX
	if src.W > gaugeWideLimit {
		fixedX, startX = gaugeFixedX+gaugeWideShift, gaugeWideStartX
	}
	round := func(v float64) int { return int(math.RoundToEven(v)) }
	endX := round(float64(percent*gaugeSpanX)*0.01) + startX
	endY := gaugeSpanY - round(float64(percent*gaugeSpanY)*0.01)
	slope := gaugeVertical
	if endX != fixedX {
		slope = float64(endY) / float64(endX-fixedX)
	}
	if dst.Backend != nil {
		// Submit row geometry instead of rebuilding a masked bitmap every frame.
		// Float64 boundary tests preserve the native gauge's edge pixels exactly.
		for row := 0; row < src.H; row++ {
			n := sort.Search(src.W, func(px int) bool {
				side := float64(px)*slope - float64(fixedX)*slope - float64(row)
				return !((slope < 0 && side > 0) || (slope >= 0 && side < 0))
			})
			if n > 0 {
				dst.DrawRect(x, y+row, image.Rect(0, row, n, row+1), src, true)
			}
			if src.Key != 0 && n < src.W {
				dst.Fill(image.Rect(x+n, y+row, x+src.W, y+row+1), 0)
			}
		}
		return
	}
	g := surface.New(src.W, src.H)
	g.Key = src.Key
	for py := 0; py < src.H; py++ {
		for px := 0; px < src.W; px++ {
			v := src.Pix[py*src.W+px]
			side := float64(px)*slope - float64(fixedX)*slope - float64(py)
			if v == 0 || (slope < 0 && side > 0) || (slope >= 0 && side < 0) {
				g.Pix[py*g.W+px] = v
			}
		}
	}
	dst.Draw(x, y, g, true)
}

// AddCompiled registers a picture view with native, precomputed UI key colors.
func (db *DB) AddCompiled(name string, m *clientimage.Image) error {
	if _, ok := db.index[key(name)]; ok {
		return nil
	}
	b := m.Bounds()
	if db.RGB555 {
		img, err := m.NRGBA(b)
		if err != nil {
			return err
		}
		db.Add(name, img)
		return nil
	}
	pix, err := m.Pixels(b, clientimage.Keyed)
	if err != nil {
		return err
	}
	db.index[key(name)] = len(db.Entries)
	db.Entries = append(db.Entries, Entry{Image: &surface.Surface{W: b.Dx(), H: b.Dy(), Pix: pix}, W: b.Dx(), H: b.Dy(), Loaded: true, Registered: true, Name: name})
	return nil
}
