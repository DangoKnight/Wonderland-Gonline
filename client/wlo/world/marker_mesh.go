package world

import (
	"image"
	"math"
)

const (
	sceneMarkerTilt         = 28 * math.Pi / 180
	sceneMarkerViewSlope    = .25
	sceneMarkerViewDistance = 80.0
)

type markerVertex struct{ x, y, z float64 }
type markerFace []int
type markerPalette struct{ base, light, ridge [3]float64 }

var cyanMarkerPalette = markerPalette{[3]float64{25, 125, 170}, [3]float64{125, 110, 80}, [3]float64{80, 30, 5}}
var goldMarkerPalette = markerPalette{[3]float64{180, 125, 12}, [3]float64{75, 110, 58}, [3]float64{0, 20, 50}}

// Both markers use the same pitch, perspective, directional light and ridge
// highlights. Convex meshes are culled in camera space before rasterization.
func drawMarkerMesh(dst *image.NRGBA, vertices []markerVertex, faces []markerFace, centerX, centerY float64, palette markerPalette, glow, pulse float64) {
	clear(dst.Pix)
	var camera [8]markerVertex
	for i, v := range vertices {
		y, z := v.y*math.Cos(sceneMarkerTilt)+v.z*math.Sin(sceneMarkerTilt), -v.y*math.Sin(sceneMarkerTilt)+v.z*math.Cos(sceneMarkerTilt)
		camera[i] = markerVertex{v.x, y + z*sceneMarkerViewSlope, z - y*sceneMarkerViewSlope}
	}
	project := func(v markerVertex) markerVertex {
		perspective := sceneMarkerViewDistance / (sceneMarkerViewDistance - v.z)
		return markerVertex{centerX + v.x*perspective, centerY + v.y*perspective, v.z}
	}
	cross := func(a, b markerVertex, x, y float64) float64 { return (b.x-a.x)*(y-a.y) - (b.y-a.y)*(x-a.x) }
	padding := 1 + glow*3
	for _, face := range faces {
		a, b, c := camera[face[0]], camera[face[1]], camera[face[2]]
		ux, uy, uz := b.x-a.x, b.y-a.y, b.z-a.z
		vx, vy, vz := c.x-a.x, c.y-a.y, c.z-a.z
		nx, ny, nz := uy*vz-uz*vy, uz*vx-ux*vz, ux*vy-uy*vx
		if nx*(a.x+b.x+c.x)+ny*(a.y+b.y+c.y)+nz*(a.z+b.z+c.z) < 0 {
			nx, ny, nz = -nx, -ny, -nz
		}
		if nz <= 0 {
			continue
		}
		length := math.Sqrt(nx*nx + ny*ny + nz*nz)
		light := .35 + .65*max(0.0, (-.5*nx-.4*ny+.75*nz)/length)
		// Rasterize each convex face as a polygon. A square cap is one face,
		// so its triangulation never becomes a highlighted diagonal seam.
		var polygon [8]markerVertex
		minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for i, index := range face {
			polygon[i] = project(camera[index])
			v := polygon[i]
			minX, minY, maxX, maxY = min(minX, v.x), min(minY, v.y), max(maxX, v.x), max(maxY, v.y)
		}
		orientation := 1.0
		if cross(polygon[0], polygon[1], polygon[2].x, polygon[2].y) < 0 {
			orientation = -1
		}
		var lengths [8]float64
		valid := true
		for i := range face {
			a, b := polygon[i], polygon[(i+1)%len(face)]
			lengths[i] = math.Hypot(b.x-a.x, b.y-a.y)
			valid = valid && lengths[i] >= 1e-9
		}
		if !valid {
			continue
		}
		for y := max(0, int(math.Floor(minY-padding))); y < min(dst.Bounds().Dy(), int(math.Ceil(maxY+padding))); y++ {
			for x := max(0, int(math.Floor(minX-padding))); x < min(dst.Bounds().Dx(), int(math.Ceil(maxX+padding))); x++ {
				px, py := float64(x)+.5, float64(y)+.5
				d := math.Inf(1)
				for i := range face {
					d = min(d, orientation*cross(polygon[i], polygon[(i+1)%len(face)], px, py)/lengths[i])
				}
				coverage := max(0.0, min(1.0, d+.5))
				alpha := coverage
				if glow > 0 {
					outside := max(0.0, -d)
					alpha += math.Exp(-outside*outside/(2*glow*glow)) * .35 * pulse * (1 - coverage)
				}
				i := y*dst.Stride + x*4
				if byte(255*alpha) <= dst.Pix[i+3] {
					continue
				}
				ridge := max(0.0, 1-math.Abs(d)/.8) * .55
				for channel := 0; channel < 3; channel++ {
					dst.Pix[i+channel] = byte(min(255.0, palette.base[channel]+palette.light[channel]*light+palette.ridge[channel]*ridge))
				}
				dst.Pix[i+3] = byte(255 * alpha)
			}
		}
	}
}
