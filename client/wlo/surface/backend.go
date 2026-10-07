package surface

import (
	"image"
	"image/color"
)

// Backend receives ordered raster operations. It owns a render target rather
// than a CPU shadow framebuffer. Image loaders are called only on cache misses.
// GPU implementations and drawing calls belong to the Ebitengine game thread.
type Backend interface {
	DrawRect(int, int, image.Rectangle, *Surface, bool)
	DrawStretch(image.Rectangle, *Surface, bool)
	DrawLight(int, int, image.Rectangle, *Surface, int)
	Fill(image.Rectangle, uint16)
	FillAlpha(image.Rectangle, uint32, int)
	DrawSprite(int, int, any, func() (*image.NRGBA, error)) error
	DrawIndexed(int, int, any, *image.Gray, [256]uint16, [256]bool)
	DrawSpriteScaled(int, int, int, any, func() (*image.NRGBA, error)) error
	DrawIndexedScaled(int, int, int, any, *image.Gray, [256]uint16, [256]bool)
	RGBA() *image.RGBA // Explicit capture/readback only.
	NewSurface(int, int) *Surface
	Close()
}

func (s *Surface) NewCompatible(w, h int) *Surface {
	if s.Backend != nil {
		return s.Backend.NewSurface(w, h)
	}
	return New(w, h)
}
func (s *Surface) Clone() *Surface {
	out := s.NewCompatible(s.W, s.H)
	out.Key = s.Key
	out.Draw(0, 0, s, false)
	return out
}
func (s *Surface) Close() {
	if s.Backend != nil {
		s.Backend.Close()
		s.Backend = nil
	}
}

// CPUCopy is an explicit interoperability/capture operation. Normal GPU
// rendering never calls it; CPU snapshots remain available without a backend.
func (s *Surface) CPUCopy() *Surface {
	out := FromImage(s.RGBA())
	out.Key = s.Key
	return out
}

// DrawSprite preserves straight-alpha PNG blending and native quantization.
// key must be a stable, comparable identity for the immutable source artwork.
func (s *Surface) DrawSprite(x, y int, key any, load func() (*image.NRGBA, error)) error {
	return s.DrawSpriteScaled(x, y, 1, key, load)
}

// DrawSpriteScaled applies native integer sprite scaling without creating an
// intermediate render target or duplicating the source GPU texture.
func (s *Surface) DrawSpriteScaled(x, y, scale int, key any, load func() (*image.NRGBA, error)) error {
	if scale < 1 {
		return nil
	}
	if s.Backend != nil {
		return s.Backend.DrawSpriteScaled(x, y, scale, key, load)
	}
	m, err := load()
	if err != nil || m == nil {
		return err
	}
	s.Revision++
	b := m.Bounds()
	for row := 0; row < b.Dy()*scale; row++ {
		dy := y + row
		if dy < 0 || dy >= s.H {
			continue
		}
		for col := 0; col < b.Dx()*scale; col++ {
			dx := x + col
			if dx < 0 || dx >= s.W {
				continue
			}
			c := m.NRGBAAt(b.Min.X+col/scale, b.Min.Y+row/scale)
			if c.A == 0 {
				continue
			}
			if c.A < 255 {
				c = BlendSpritePixel(c, Expand(s.Pix[dy*s.W+dx]))
			}
			s.Pix[dy*s.W+dx] = RGB565(c.R, c.G, c.B)
		}
	}
	return nil
}
func BlendSpritePixel(source color.NRGBA, destination color.RGBA) color.NRGBA {
	alpha := uint32(source.A)
	inverse := 255 - alpha
	mix := func(s, d uint8) uint8 { return uint8((uint32(s)*alpha + uint32(d)*inverse + 127) / 255) }
	return color.NRGBA{R: mix(source.R, destination.R), G: mix(source.G, destination.G), B: mix(source.B, destination.B), A: 255}
}
func (s *Surface) DrawIndexed(x, y int, key any, m *image.Gray, lut [256]uint16, opaque [256]bool) {
	s.DrawIndexedScaled(x, y, 1, key, m, lut, opaque)
}

func (s *Surface) DrawIndexedScaled(x, y, scale int, key any, m *image.Gray, lut [256]uint16, opaque [256]bool) {
	if scale < 1 {
		return
	}
	if s.Backend != nil {
		s.Backend.DrawIndexedScaled(x, y, scale, key, m, lut, opaque)
		return
	}
	s.Revision++
	b := m.Bounds()
	for row := 0; row < b.Dy()*scale; row++ {
		dy := y + row
		if dy < 0 || dy >= s.H {
			continue
		}
		for col := 0; col < b.Dx()*scale; col++ {
			dx := x + col
			if dx < 0 || dx >= s.W {
				continue
			}
			if i := m.Pix[m.PixOffset(b.Min.X+col/scale, b.Min.Y+row/scale)]; opaque[i] {
				s.Pix[dy*s.W+dx] = lut[i]
			}
		}
	}
}
