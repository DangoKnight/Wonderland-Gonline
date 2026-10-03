// Package surface stands in for the DelphiX TDirectDrawSurface the client
// draws on: a 16-bit RGB565 pixel buffer with colour-keyed blits.
//
// References are to the reference decompile
// (Wonderland-Private-Server/decompiled/aLogin_decompiled.c).
package surface

import (
	"image"
	"image/color"
)

// Surface is a width x height RGB565 buffer. Key is the transparent colour
// used when a transparent blit reads from this surface; the picture
// database creates every cached surface with key 0 (FUN_00477e18).
type Surface struct {
	W, H int
	Pix  []uint16
	Key  uint16
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

// Fill sets a rectangle to one RGB565 value.
func (s *Surface) Fill(r image.Rectangle, v uint16) {
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

// FillAlpha blends a TColor over a rectangle (DelphiX FillRectAlpha).
// The 8-bit blend of expanded pixels is an approximation of DelphiX's
// 16-bit routine, which has not been traced.
func (s *Surface) FillAlpha(r image.Rectangle, c uint32, alpha int) {
	if alpha >= 0xff {
		s.Fill(r, TColor(c))
		return
	}
	r = r.Intersect(image.Rect(0, 0, s.W, s.H))
	src := [3]int{int(uint8(c)), int(uint8(c >> 8)), int(uint8(c >> 16))}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			d := Expand(s.Pix[y*s.W+x])
			dst := [3]int{int(d.R), int(d.G), int(d.B)}
			var o [3]uint8
			for i := range o {
				o[i] = uint8((src[i]*alpha + dst[i]*(256-alpha)) >> 8)
			}
			s.Pix[y*s.W+x] = RGB565(o[0], o[1], o[2])
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
