// Package surface stands in for the DelphiX TDirectDrawSurface the client
// draws on: a 16-bit RGB565 pixel buffer with colour-keyed blits.
//
// References are to the reference decompile
// (Wonderland-Private-Server/decompiled/aLogin_decompiled.c).
package surface

import (
	"image"
	"image/color"
	"log"
	"wonderland-gonline/internal/clientimage"
)

// Surface is a width x height RGB565 buffer. Key is the transparent colour
// used when a transparent blit reads from this surface; the picture
// database creates every cached surface with key 0 (FUN_00477e18).
type Surface struct {
	W, H      int
	Pix       []uint16
	Tiles     *clientimage.Image // Immutable compiled image metadata; Pix remains nil.
	Backend   Backend            // nil for CPU buffers; GPU targets have no Pix buffer.
	Revision  uint64             // Increment after changing an uploaded CPU source.
	tileError bool
	Key       uint16
}

func New(w, h int) *Surface {
	return &Surface{W: w, H: h, Pix: make([]uint16, w*h)}
}

// Draw is TDirectDrawSurface.Draw(X, Y, Source, Transparent): the whole
// source at (x, y), clipped to the destination.
func (s *Surface) Draw(x, y int, src *Surface, transparent bool) {
	s.DrawRect(x, y, image.Rect(0, 0, src.W, src.H), src, transparent)
}

// DrawRect is TDirectDrawSurface.Draw(X, Y, SrcRect, Source, Transparent).
// The source rectangle is clipped to the source and the destination.
func (s *Surface) DrawRect(x, y int, r image.Rectangle, src *Surface, transparent bool) {
	if s.Backend != nil {
		s.Backend.DrawRect(x, y, r, src, transparent)
		return
	}
	s.Revision++
	if src.Backend != nil {
		src = src.CPUCopy()
	}
	if src.Tiles != nil {
		r = r.Intersect(image.Rect(0, 0, src.W, src.H))
		visible := r.Intersect(image.Rect(r.Min.X-x, r.Min.Y-y, r.Min.X-x+s.W, r.Min.Y-y+s.H))
		if visible.Empty() {
			return
		}
		origin := image.Pt(x+visible.Min.X-r.Min.X, y+visible.Min.Y-r.Min.Y)
		if err := src.Tiles.CopyPixels(visible, s.Pix, s.W, origin, transparent, src.Key); err != nil {
			src.reportTileError(err)
		}
		return
	}

	r = r.Intersect(image.Rect(0, 0, src.W, src.H))
	for sy := r.Min.Y; sy < r.Max.Y; sy++ {
		dy := y + sy - r.Min.Y
		if dy < 0 || dy >= s.H {
			continue
		}
		for sx := r.Min.X; sx < r.Max.X; sx++ {
			dx := x + sx - r.Min.X
			if dx < 0 || dx >= s.W {
				continue
			}
			v := src.Pix[sy*src.W+sx]
			if transparent && v == src.Key {
				continue
			}
			s.Pix[dy*s.W+dx] = v
		}
	}
}

// DrawStretch is rodraw2's ro_Reduce_ColorKey_Blt: the whole source
// scaled (nearest pixel) into dst's rectangle r, skipping the key colour
// when transparent.
func (s *Surface) DrawStretch(r image.Rectangle, src *Surface, transparent bool) {
	if s.Backend != nil {
		s.Backend.DrawStretch(r, src, transparent)
		return
	}
	s.Revision++
	if src.Backend != nil {
		src = src.CPUCopy()
	}
	if src.Tiles != nil {
		src = src.materialized()
		if src == nil {
			return
		}
	}
	if r.Dx() <= 0 || r.Dy() <= 0 || src.W == 0 || src.H == 0 {
		return
	}
	for dy := max(r.Min.Y, 0); dy < min(r.Max.Y, s.H); dy++ {
		sy := (dy - r.Min.Y) * src.H / r.Dy()
		for dx := max(r.Min.X, 0); dx < min(r.Max.X, s.W); dx++ {
			v := src.Pix[sy*src.W+(dx-r.Min.X)*src.W/r.Dx()]
			if transparent && v == src.Key {
				continue
			}
			s.Pix[dy*s.W+dx] = v
		}
	}
}

// LightLevelFull is the light level that adds the whole source.
const LightLevelFull = 32

// DrawLight is rodraw2's ro_Clipper_LightAlpha_ColorKey_Blt: the source
// rectangle is added to the destination, each channel scaled by
// level/32 and saturating, skipping the source's key colour.
func (s *Surface) DrawLight(x, y int, r image.Rectangle, src *Surface, level int) {
	if s.Backend != nil {
		s.Backend.DrawLight(x, y, r, src, level)
		return
	}
	s.Revision++
	if src.Backend != nil {
		src = src.CPUCopy()
	}
	if src.Tiles != nil {
		src = src.materialized()
		if src == nil {
			return
		}
	}
	r = r.Intersect(image.Rect(0, 0, src.W, src.H))
	for sy := r.Min.Y; sy < r.Max.Y; sy++ {
		dy := y + sy - r.Min.Y
		if dy < 0 || dy >= s.H {
			continue
		}
		for sx := r.Min.X; sx < r.Max.X; sx++ {
			dx := x + sx - r.Min.X
			if dx < 0 || dx >= s.W {
				continue
			}
			v := src.Pix[sy*src.W+sx]
			if v == src.Key {
				continue
			}
			d := &s.Pix[dy*s.W+dx]
			add := func(shift, max uint16) uint16 {
				c := (*d>>shift)&max + uint16(int((v>>shift)&max)*level/LightLevelFull)
				return min(c, max) << shift
			}
			*d = add(11, 0x1f) | add(5, 0x3f) | add(0, 0x1f)
		}
	}
}

// Fill sets a rectangle to one RGB565 value.
func (s *Surface) Fill(r image.Rectangle, v uint16) {
	if s.Backend != nil {
		s.Backend.Fill(r, v)
		return
	}
	s.Revision++
	r = r.Intersect(image.Rect(0, 0, s.W, s.H))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := s.Pix[y*s.W:]
		for x := r.Min.X; x < r.Max.X; x++ {
			row[x] = v
		}
	}
}

// RGB565 packs 8-bit channels by truncation, as the loaders do.
func RGB565(r, g, b uint8) uint16 {
	return uint16(r>>3)<<11 | uint16(g>>2)<<5 | uint16(b>>3)
}

// Expand converts an RGB565 value to 8-bit channels by bit replication,
// matching how the original's frames appear when captured.
func Expand(v uint16) color.RGBA {
	r, g, b := uint8(v>>11), uint8(v>>5&63), uint8(v&31)
	return color.RGBA{r<<3 | r>>2, g<<2 | g>>4, b<<3 | b>>2, 255}
}

// RGBA converts the surface for display or comparison.
func (s *Surface) RGBA() *image.RGBA {
	if s.Backend != nil {
		return s.Backend.RGBA()
	}
	if s.Tiles != nil {
		if decoded := s.materialized(); decoded != nil {
			return decoded.RGBA()
		}
		return image.NewRGBA(image.Rect(0, 0, s.W, s.H))
	}
	out := image.NewRGBA(image.Rect(0, 0, s.W, s.H))
	for i, v := range s.Pix {
		c := Expand(v)
		out.Pix[4*i], out.Pix[4*i+1], out.Pix[4*i+2], out.Pix[4*i+3] = c.R, c.G, c.B, 255
	}
	return out
}

// FromImage converts an image by truncation to RGB565, as GDI does when a
// TJPEGImage is drawn through a 16-bit surface's canvas.
func FromImage(m image.Image) *Surface {
	b := m.Bounds()
	s := New(b.Dx(), b.Dy())
	for y := 0; y < s.H; y++ {
		for x := 0; x < s.W; x++ {
			r, g, bl, _ := m.At(b.Min.X+x, b.Min.Y+y).RGBA()
			s.Pix[y*s.W+x] = RGB565(uint8(r>>8), uint8(g>>8), uint8(bl>>8))
		}
	}
	return s
}

// TColor converts a Delphi TColor ($00BBGGRR) to RGB565 by truncation.
func TColor(c uint32) uint16 {
	return RGB565(uint8(c), uint8(c>>8), uint8(c>>16))
}

// FillAlpha blends a TColor over a rectangle (DelphiX FillRectAlpha, as
// the hint box uses it): each 16-bit channel becomes (src·alpha +
// dst·(256 − alpha)) >> 8, computed on the 5-, 6- and 5-bit values. The
// original's tooltip over the sea (Chat/Whisper_Chat_01_(tooltip).png,
// $F98B3D at 200) matches this to the bit.
func (s *Surface) FillAlpha(r image.Rectangle, c uint32, alpha int) {
	if s.Backend != nil {
		s.Backend.FillAlpha(r, c, alpha)
		return
	}
	s.Revision++
	if alpha >= 0xff {
		s.Fill(r, TColor(c))
		return
	}
	r = r.Intersect(image.Rect(0, 0, s.W, s.H))
	src := TColor(c)
	blend := func(sv, dv uint16, shift, max uint16) uint16 {
		a, b := int(sv>>shift&max), int(dv>>shift&max)
		return uint16((a*alpha+b*(256-alpha))>>8) << shift
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := s.Pix[y*s.W:]
		for x := r.Min.X; x < r.Max.X; x++ {
			d := row[x]
			row[x] = blend(src, d, 11, 0x1f) | blend(src, d, 5, 0x3f) | blend(src, d, 0, 0x1f)
		}
	}
}

// Frame is TCanvas.Rectangle with a one-pixel pen and a clear brush.
func (s *Surface) Frame(r image.Rectangle, v uint16) {
	s.Fill(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), v)
	s.Fill(image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), v)
	s.Fill(image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), v)
	s.Fill(image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), v)
}

// FromCompiled keeps large map canvases lazy; drawing requests only visible tiles.
func FromCompiled(m *clientimage.Image) *Surface {
	b := m.Bounds()
	return &Surface{W: b.Dx(), H: b.Dy(), Tiles: m}
}
func (s *Surface) reportTileError(err error) {
	if !s.tileError {
		log.Printf("compiled image: %v", err)
		s.tileError = true
	}
}
func (s *Surface) materialized() *Surface {
	pix, err := s.Tiles.Pixels(s.Tiles.Bounds(), clientimage.Plain)
	if err != nil {
		s.reportTileError(err)
		return nil
	}
	return &Surface{W: s.W, H: s.H, Pix: pix, Key: s.Key}
}
