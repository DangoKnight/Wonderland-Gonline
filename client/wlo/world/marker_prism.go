package world

import (
	"image"
	"math"
)

// A tilted octahedron spins around its own axis, sharing the transition
// pyramid's projection and lighting.
func (m *Marker) renderPrism(centerY, scale float64) {
	if m.prism == nil {
		m.prism = image.NewNRGBA(image.Rect(0, 0, markerSize, markerSize))
	}
	const prismHeight = 13.0
	vertices := [6]markerVertex{{0, -prismHeight * scale, 0}, {0, prismHeight * scale, 0}}
	for i := 0; i < 4; i++ {
		a := m.spin + float64(i)*math.Pi/2
		vertices[i+2] = markerVertex{markerPrismHalfWidth * 1.4 * math.Cos(a) * scale, 0, markerPrismHalfWidth * 1.4 * math.Sin(a) * scale}
	}
	var faces [8]markerFace
	for side := 0; side < 4; side++ {
		faces[side*2] = markerFace{0, side + 2, (side+1)%4 + 2}
		faces[side*2+1] = markerFace{1, side + 2, (side+1)%4 + 2}
	}
	drawMarkerMesh(m.prism, vertices[:], faces[:], markerCenter, centerY, cyanMarkerPalette, 0, 0)
}
