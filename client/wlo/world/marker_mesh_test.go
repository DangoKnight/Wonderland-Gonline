package world

import (
	"image"
	"testing"
)

func TestMarkerMeshPlanarCapHasNoDiagonalSeam(t *testing.T) {
	pixels := image.NewNRGBA(image.Rect(0, 0, 64, 80))
	vertices := []markerVertex{{-12, -12, 0}, {12, -12, 0}, {12, 12, 0}, {-12, 12, 0}}
	drawMarkerMesh(pixels, vertices, []markerFace{{0, 1, 2, 3}}, 32, 40, goldMarkerPalette, 0, 0)
	want := pixels.NRGBAAt(32, 40)
	if want.A != 255 {
		t.Fatal("cap center has a transparent seam")
	}
	for y := 38; y <= 42; y++ {
		for x := 30; x <= 34; x++ {
			if got := pixels.NRGBAAt(x, y); got != want {
				t.Fatalf("flat cap contains artificial shading/seam at %d,%d: %v vs %v", x, y, got, want)
			}
		}
	}
}
