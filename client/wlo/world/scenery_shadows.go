package world

import (
	"image"
	"image/color"

	"wonderland-gonline/client/wlo/surface"
)

const (
	sceneryShadowChannelLimit = 64
	sceneryShadowOpacity      = 128 // Two opaque dots per four native pixels.
)

// Reconstruct complete dark/key checkerboard cells once at scenery load time.
// Fractional camera sampling then sees translucent shading instead of dots.
// Opaque artwork and other transparency retain their original pixels.
func smoothSceneryShadows(src *surface.Surface, frames int) []*image.NRGBA {
	frames = max(1, frames)
	frameHeight := src.H / frames
	if frameHeight == 0 || len(src.Pix) != src.W*src.H {
		return nil
	}
	var out *image.NRGBA
	for y := 0; y < src.H-1; y++ {
		if (y+1)%frameHeight == 0 {
			continue // Never infer coverage across animation frame boundaries.
		}
		for x := 0; x < src.W-1; x++ {
			a, b := src.Pix[y*src.W+x], src.Pix[y*src.W+x+1]
			c, d := src.Pix[(y+1)*src.W+x], src.Pix[(y+1)*src.W+x+1]
			if a != d || b != c || a == b || (a != src.Key && b != src.Key) {
				continue
			}
			ink := a
			if ink == src.Key {
				ink = b
			}
			v := surface.Expand(ink)
			if v.R > sceneryShadowChannelLimit || v.G > sceneryShadowChannelLimit || v.B > sceneryShadowChannelLimit {
				continue
			}
			if out == nil {
				out = image.NewNRGBA(image.Rect(0, 0, src.W, src.H))
				for i, pixel := range src.Pix {
					if pixel != src.Key {
						p := surface.Expand(pixel)
						out.Pix[4*i], out.Pix[4*i+1], out.Pix[4*i+2], out.Pix[4*i+3] = p.R, p.G, p.B, 255
					}
				}
			}
			shade := color.NRGBA{R: v.R, G: v.G, B: v.B, A: sceneryShadowOpacity}
			out.SetNRGBA(x, y, shade)
			out.SetNRGBA(x+1, y, shade)
			out.SetNRGBA(x, y+1, shade)
			out.SetNRGBA(x+1, y+1, shade)
		}
	}
	if out == nil {
		return nil
	}
	result := make([]*image.NRGBA, frames)
	for frame := range result {
		result[frame] = out.SubImage(image.Rect(0, frame*frameHeight, src.W, (frame+1)*frameHeight)).(*image.NRGBA)
	}
	return result
}
