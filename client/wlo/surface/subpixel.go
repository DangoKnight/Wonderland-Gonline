package surface

import (
	"image"
	"math"
)

// Catmull-Rom weights retain fine sprite detail during fractional camera motion.
// Six-bit signed weights keep CPU/GPU arithmetic exact, including negative lobes.
func subpixelWeights(fraction int) [4]int {
	t := float64(fraction) / 256
	t2, t3 := t*t, t*t*t
	a := int(math.Floor((-.5*t+t2-.5*t3)*64 + .5))
	b := int(math.Floor((1-2.5*t2+1.5*t3)*64 + .5))
	d := int(math.Floor((-.5*t2+.5*t3)*64 + .5))
	return [4]int{a, b, 64 - a - b - d, d}
}

// DrawSubpixel reconstructs the complete scene in a single fractional camera
// space. Decoded channels are filtered and clamped before RGB565 quantization.
func (s *Surface) DrawSubpixel(r image.Rectangle, src *Surface, offsetX, offsetY float64) {
	sx, sy := int(math.Floor(offsetX)), int(math.Floor(offsetY))
	fx, fy := int((offsetX-float64(sx))*256), int((offsetY-float64(sy))*256)
	if b, ok := s.Backend.(interface {
		DrawSubpixel(image.Rectangle, *Surface, int, int, int, int)
	}); ok {
		b.DrawSubpixel(r, src, sx, sy, fx, fy)
		return
	}
	if s.Backend != nil || src.W == 0 || src.H == 0 {
		return
	}
	wx, wy := subpixelWeights(fx), subpixelWeights(fy)
	for y := max(0, r.Min.Y); y < min(s.H, r.Max.Y); y++ {
		for x := max(0, r.Min.X); x < min(s.W, r.Max.X); x++ {
			u, v := x-r.Min.X+sx, y-r.Min.Y+sy
			red, green, blue := 0, 0, 0
			for row := 0; row < 4; row++ {
				if wy[row] == 0 {
					continue
				}
				yy := max(0, min(src.H-1, v+row-1))
				for col := 0; col < 4; col++ {
					weight := wx[col] * wy[row]
					if weight == 0 {
						continue
					}
					xx := max(0, min(src.W-1, u+col-1))
					c := Expand(src.Pix[yy*src.W+xx])
					red += int(c.R) * weight
					green += int(c.G) * weight
					blue += int(c.B) * weight
				}
			}
			channel := func(sum int) uint8 { return uint8(max(0, min(255, sum/4096))) }
			s.Pix[y*s.W+x] = RGB565(channel(red), channel(green), channel(blue))
		}
	}
	s.Revision++
}
