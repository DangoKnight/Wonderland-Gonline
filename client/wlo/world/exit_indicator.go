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
	exitIndicatorWidth     = 64
	exitIndicatorHeight    = 104 // Headroom for the tilted base's glow at peak bob.
	exitIndicatorCenter    = 32.0
	exitIndicatorHalfWidth = 13.0
	exitIndicatorTop       = -20.0
	exitIndicatorTip       = 30.0
	exitIndicatorCenterY   = 48.0
	exitIndicatorGlow      = 2.8
	exitIndicatorBob       = 5.0
	exitIndicatorPeriod    = 3600 * time.Millisecond
	exitIndicatorBobPeriod = 1800 * time.Millisecond
	exitIndicatorOpacity   = 165 // About 65% opacity, applied to body and glow.
	exitIndicatorAnchorY   = 64
)

type exitIndicator struct {
	pixels   *image.NRGBA
	revision uint64
}
type exitIndicatorFrame struct {
	pixels   *image.NRGBA
	revision uint64
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
	_ = dst.DrawSprite(x-exitIndicatorWidth/2, y-exitIndicatorAnchorY, exitIndicatorFrame{p.pixels, p.revision}, func() (*image.NRGBA, error) { return p.pixels, nil })
}
