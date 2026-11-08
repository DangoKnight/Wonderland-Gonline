package world

import (
	"image"
	"math"
	"time"
	"wonderland-gonline/client/wlo/surface"
)

// SmallDoorLight uses a smoothly spinning inverted pyramid. Native EVE LightAt
// remains the exit anchor; its gold glow and bob retain the original cue.
const (
	exitIndicatorWidth        = 64
	exitIndicatorHeight       = 104 // Headroom for the tilted base's glow at peak bob.
	exitIndicatorCenter       = 32.0
	exitIndicatorHalfWidth    = 13.0
	exitIndicatorTop          = -20.0
	exitIndicatorTip          = 30.0
	exitIndicatorCenterY      = 48.0
	exitIndicatorGlow         = 2.8
	exitIndicatorBob          = 5.0
	exitIndicatorPeriod       = 3600 * time.Millisecond
	exitIndicatorBobPeriod    = 1800 * time.Millisecond
	exitIndicatorOpacity      = 165 // Ordinary visibility over terrain and items.
	exitIndicatorModelOpacity = 102 // 40% opacity only over opaque actor pixels.
	exitIndicatorAnchorY      = 64
)

type exitIndicator struct {
	modelPixels   *image.NRGBA
	modelRevision uint64
	pixels        *image.NRGBA
	revision      uint64
}

func newExitIndicator() *exitIndicator {
	return &exitIndicator{pixels: image.NewNRGBA(image.Rect(0, 0, exitIndicatorWidth, exitIndicatorHeight))}
}

func (p *exitIndicator) render(elapsed time.Duration) {
	phase := 2 * math.Pi * float64(elapsed%exitIndicatorPeriod) / float64(exitIndicatorPeriod)
	bobPhase := 2 * math.Pi * float64(elapsed%exitIndicatorBobPeriod) / float64(exitIndicatorBobPeriod)
	vertices := [5]markerVertex{{0, exitIndicatorTip, 0}}
	for side := 0; side < 4; side++ {
		angle := phase + math.Pi/4 + float64(side)*math.Pi/2
		vertices[side+1] = markerVertex{exitIndicatorHalfWidth * math.Cos(angle), exitIndicatorTop, exitIndicatorHalfWidth * math.Sin(angle)}
	}
	faces := [5]markerFace{{0, 1, 2}, {0, 2, 3}, {0, 3, 4}, {0, 4, 1}, {1, 2, 3, 4}}
	drawMarkerMesh(p.pixels, vertices[:], faces[:], exitIndicatorCenter, exitIndicatorCenterY+exitIndicatorBob*math.Sin(bobPhase), goldMarkerPalette, exitIndicatorGlow, .8+.2*math.Cos(bobPhase))
	for i := 3; i < len(p.pixels.Pix); i += 4 {
		p.pixels.Pix[i] = byte(int(p.pixels.Pix[i]) * exitIndicatorOpacity / 255)
	}
	p.revision++
}

func (p *exitIndicator) draw(dst *surface.Surface, x, y int) {
	_ = dst.DrawSprite(x-exitIndicatorWidth/2, y-exitIndicatorAnchorY, surface.DynamicSprite{Identity: p.pixels, Revision: p.revision}, func() (*image.NRGBA, error) { return p.pixels, nil })
}

func (p *exitIndicator) drawOverModels(dst *surface.Surface, x, y int, overModel func(int, int) bool) {
	if p.modelPixels == nil {
		p.modelPixels = image.NewNRGBA(p.pixels.Bounds())
	}
	copy(p.modelPixels.Pix, p.pixels.Pix)
	x, y = x-exitIndicatorWidth/2, y-exitIndicatorAnchorY
	for row := 0; row < exitIndicatorHeight; row++ {
		for col := 0; col < exitIndicatorWidth; col++ {
			i := row*p.modelPixels.Stride + col*4 + 3
			if p.modelPixels.Pix[i] != 0 && overModel(x+col, y+row) {
				p.modelPixels.Pix[i] = byte(int(p.modelPixels.Pix[i]) * exitIndicatorModelOpacity / exitIndicatorOpacity)
			}
		}
	}
	p.modelRevision++
	_ = dst.DrawSprite(x, y, surface.DynamicSprite{Identity: p.modelPixels, Revision: p.modelRevision}, func() (*image.NRGBA, error) { return p.modelPixels, nil })
}
