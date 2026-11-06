package render

import "math"

// Viewport maps an unchanged logical canvas into an output framebuffer.
// Drawing and pointer input must use this same transform.
type Viewport struct {
	X, Y, Scale   float64
	Width, Height int
}

func FitViewport(sourceWidth, sourceHeight, outputWidth, outputHeight int) Viewport {
	if sourceWidth <= 0 || sourceHeight <= 0 || outputWidth <= 0 || outputHeight <= 0 {
		return Viewport{Scale: 1}
	}
	scale := min(float64(outputWidth)/float64(sourceWidth), float64(outputHeight)/float64(sourceHeight))
	return Viewport{X: (float64(outputWidth) - float64(sourceWidth)*scale) / 2, Y: (float64(outputHeight) - float64(sourceHeight)*scale) / 2, Scale: scale, Width: sourceWidth, Height: sourceHeight}
}

func (v Viewport) Point(x, y int) (int, int, bool) {
	if v.Scale <= 0 {
		return 0, 0, false
	}
	px, py := (float64(x)-v.X)/v.Scale, (float64(y)-v.Y)/v.Scale
	if px < 0 || py < 0 || px >= float64(v.Width) || py >= float64(v.Height) {
		return 0, 0, false
	}
	return int(math.Floor(px)), int(math.Floor(py)), true
}
