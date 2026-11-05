package world

import (
	"image"
	"math"
	"time"
	"wonderland-gonline/client/wlo/surface"
)

// Replace SmallDoorLight's four stepped frames with a continuous glowing
// downward pointer. Native EVE LightAt remains the exit's anchor.
const (
	exitIndicatorWidth     = 64
	exitIndicatorHeight    = 88
	exitIndicatorCenter    = 32.0
	exitIndicatorTop       = 20.0
	exitIndicatorTip       = 62.0
	exitIndicatorHalfWidth = 13.0
	exitIndicatorGlow      = 2.8
	exitIndicatorBob       = 5.0
	exitIndicatorPeriod    = 1800 * time.Millisecond
	exitIndicatorAnchorY   = 56
)

type exitIndicator struct {
	pixels    *image.NRGBA
	revision  uint64
	distances []float64
}

type exitIndicatorFrame struct {
	pixels   *image.NRGBA
	revision uint64
}

func newExitIndicator() *exitIndicator {
	p := &exitIndicator{pixels: image.NewNRGBA(image.Rect(0, 0, exitIndicatorWidth, exitIndicatorHeight))}
	// Static signed distance field, with extra rows for subpixel vertical motion.
	p.distances = make([]float64, exitIndicatorWidth*exitIndicatorHeight)
	for y := 0; y < exitIndicatorHeight; y++ {
		for x := 0; x < exitIndicatorWidth; x++ {
			p.distances[y*exitIndicatorWidth+x] = exitPointerDistance(float64(x)+.5, float64(y)+.5)
		}
	}
	return p
}

func exitPointerDistance(x, y float64) float64 {
	// Distance to the three edges; negative inside the triangle.
	ax, ay := exitIndicatorCenter-exitIndicatorHalfWidth, exitIndicatorTop
	bx, by := exitIndicatorCenter+exitIndicatorHalfWidth, exitIndicatorTop
	cx, cy := exitIndicatorCenter, exitIndicatorTip
	segment := func(ax, ay, bx, by float64) float64 {
		dx, dy := bx-ax, by-ay
		t := max(0.0, min(1.0, ((x-ax)*dx+(y-ay)*dy)/(dx*dx+dy*dy)))
		return math.Hypot(x-ax-t*dx, y-ay-t*dy)
	}
	d := min(segment(ax, ay, bx, by), segment(bx, by, cx, cy), segment(cx, cy, ax, ay))
	if y >= ay && y <= cy && math.Abs(x-cx) <= exitIndicatorHalfWidth*(cy-y)/(cy-ay) {
		return -d
	}
	return d
}

func (p *exitIndicator) render(elapsed time.Duration) {
	phase := 2 * math.Pi * float64(elapsed%exitIndicatorPeriod) / float64(exitIndicatorPeriod)
	bob := exitIndicatorBob * math.Sin(phase)
	pulse := .8 + .2*math.Cos(phase)
	for y := 0; y < exitIndicatorHeight; y++ {
		for x := 0; x < exitIndicatorWidth; x++ {
			sample := float64(y) - bob
			iy := int(math.Floor(sample))
			d := 100.0
			if iy >= 0 && iy+1 < exitIndicatorHeight {
				fraction := sample - float64(iy)
				d = p.distances[iy*exitIndicatorWidth+x]*(1-fraction) + p.distances[(iy+1)*exitIndicatorWidth+x]*fraction
			}
			coverage := max(0.0, min(1.0, .5-d))
			glow := math.Exp(-max(0.0, d)*max(0.0, d)/(2*exitIndicatorGlow*exitIndicatorGlow)) * .35
			// Keep the yellow body solid; only its soft halo pulses.
			alpha := coverage + glow*(1-coverage)*pulse
			edge := max(0.0, 1-math.Abs(d)/1.4)
			i := y*p.pixels.Stride + x*4
			p.pixels.Pix[i] = 255
			p.pixels.Pix[i+1] = byte(238 + 17*edge)
			p.pixels.Pix[i+2] = byte(68 + 60*edge)
			p.pixels.Pix[i+3] = byte(255 * alpha)
		}
	}
	p.revision++
}

func (p *exitIndicator) draw(dst *surface.Surface, x, y int) {
	_ = dst.DrawSprite(x-exitIndicatorWidth/2, y-exitIndicatorAnchorY, exitIndicatorFrame{p.pixels, p.revision}, func() (*image.NRGBA, error) { return p.pixels, nil })
}
