// Package ui recreates aLogin.exe's front-end screens. Screens compose an
// 800x600 frame in software, as the original's DelphiX surfaces do, so a
// frame can be compared pixel-for-pixel with captures of the original.
package ui

import (
	"image"
	"image/color"
)

const (
	ScreenWidth  = 800
	ScreenHeight = 600
)

// Every original surface is RGB565. Loading converts 24-bit art by truncation;
// presentation expands each channel by bit replication. Applying both at load
// keeps the composed frame identical to what the original shows.
func quantize(c color.RGBA) color.RGBA {
	r, g, b := c.R>>3, c.G>>2, c.B>>3
	return color.RGBA{r<<3 | r>>2, g<<2 | g>>4, b<<3 | b>>2, c.A}
}

func quantizeImage(m *image.RGBA) {
	for i := 0; i < len(m.Pix); i += 4 {
		c := quantize(color.RGBA{m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3]})
		m.Pix[i], m.Pix[i+1], m.Pix[i+2] = c.R, c.G, c.B
	}
}

// Sprite is a quantized image. Keyed sprites treat pure green (0,255,0 before
// quantization) as transparent. Multi-state art stacks equal frames vertically.
type Sprite struct {
	img    *image.RGBA
	frames int
	keyed  bool
}

func newSprite(src image.Image, frames int, keyed bool) *Sprite {
	b := src.Bounds()
	m := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := range b.Dy() {
		for x := range b.Dx() {
			r, g, bl, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			c := color.RGBA{byte(r >> 8), byte(g >> 8), byte(bl >> 8), 255}
			if keyed && c.R == 0 && c.G == 255 && c.B == 0 {
				c.A = 0
			}
			m.SetRGBA(x, y, c)
		}
	}
	quantizeImage(m)
	if keyed {
		// Captures show colour-keyed art drawing RGB565 pixels with zero red
		// and blue (black, 0x0020) one blue step higher; the loader keeps
		// them distinct from the key. Applied to keyed bitmaps only.
		for i := 0; i < len(m.Pix); i += 4 {
			if m.Pix[i+3] != 0 && m.Pix[i] < 8 && m.Pix[i+2] < 8 {
				m.Pix[i+2] = 8
			}
		}
	}
	return &Sprite{img: m, frames: max(frames, 1), keyed: keyed}
}

// Size is one frame's size.
func (s *Sprite) Size() (int, int) {
	return s.img.Rect.Dx(), s.img.Rect.Dy() / s.frames
}

// Draw copies a frame to dst at (x, y), skipping transparent pixels.
func (s *Sprite) Draw(dst *image.RGBA, x, y, frame int) {
	w, h := s.Size()
	frame = min(max(frame, 0), s.frames-1)
	s.DrawRect(dst, x, y, image.Rect(0, frame*h, w, frame*h+h))
}

// DrawRect copies the source rectangle src to dst at (x, y).
func (s *Sprite) DrawRect(dst *image.RGBA, x, y int, src image.Rectangle) {
	src = src.Intersect(s.img.Rect)
	for sy := range src.Dy() {
		dy := y + sy
		if dy < dst.Rect.Min.Y || dy >= dst.Rect.Max.Y {
			continue
		}
		for sx := range src.Dx() {
			dx := x + sx
			if dx < dst.Rect.Min.X || dx >= dst.Rect.Max.X {
				continue
			}
			o := s.img.PixOffset(src.Min.X+sx, src.Min.Y+sy)
			if s.img.Pix[o+3] == 0 {
				continue
			}
			copy(dst.Pix[dst.PixOffset(dx, dy):][:4], s.img.Pix[o:o+4])
		}
	}
}

func fillRect(dst *image.RGBA, r image.Rectangle, c color.RGBA) {
	c = quantize(c)
	r = r.Intersect(dst.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			dst.SetRGBA(x, y, c)
		}
	}
}

// NewFrame allocates an 800x600 frame.
func NewFrame() *image.RGBA {
	return image.NewRGBA(image.Rect(0, 0, ScreenWidth, ScreenHeight))
}
