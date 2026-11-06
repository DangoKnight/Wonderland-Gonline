package render

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"testing"
	"wonderland-gonline/client/wlo/surface"
)

func TestScalingModesAndShader(t *testing.T) {
	shader, err := ebiten.NewShader(scalingShaderSource)
	if err != nil {
		t.Fatal(err)
	}
	shader.Deallocate()
	for _, mode := range append(ScalingModes(), "") {
		if err := ValidateScaling(mode); err != nil {
			t.Fatal(err)
		}
	}
	if ValidateScaling("unknown") == nil {
		t.Fatal("unknown filter accepted")
	}
}

func exerciseScalingFilters(cpu, gpu *Presenter) error {
	output := ebiten.NewImage(4, 4)
	defer output.Deallocate()
	src := surface.New(2, 1)
	src.Pix = []uint16{0xf800, 0x001f}
	for _, mode := range []string{ScalingBilinear, ScalingBicubic} {
		for _, p := range []*Presenter{cpu, gpu} {
			before := p.Stats().Readbacks
			if err := p.DrawFitFiltered(output, src, mode); err != nil {
				return err
			}
			if p.Stats().Readbacks != before {
				return fmt.Errorf("filter read back the GPU scene")
			}
			pixels := make([]byte, 64)
			output.ReadPixels(pixels)
			// Row zero is the letterbox, row one is filtered opaque red/blue.
			for x := 0; x < 4; x++ {
				if pixels[x*4] != 0 || pixels[x*4+3] != 255 {
					return fmt.Errorf("%s altered letterbox", mode)
				}
			}
			blue := pixels[(1*4+1)*4+2]
			low, high := byte(63), byte(65)
			if mode == ScalingBicubic {
				low, high = 51, 53
			}
			if blue < low || blue > high || pixels[(1*4+1)*4+1] != 0 {
				return fmt.Errorf("%s RGB interpolation blue=%d (want %d..%d), green=%d", mode, blue, low, high, pixels[(1*4+1)*4+1])
			}
		}
	}
	corner := surface.New(3, 3)
	corner.Pix = []uint16{0, 0xffff, 0, 0xffff, 0, 0, 0, 0, 0}
	resident := gpu.device.NewSurface(3, 3)
	defer resident.Close()
	resident.Draw(0, 0, corner, false)
	large := ebiten.NewImage(6, 6)
	defer large.Deallocate()
	for _, source := range []*surface.Surface{corner, resident} {
		for _, mode := range []string{ScalingEdgeAware, ScalingEdgeAwareSharp} {
			if err := gpu.DrawFitFiltered(large, source, mode); err != nil {
				return err
			}
			pix := make([]byte, 6*6*4)
			large.ReadPixels(pix)
			// Published Scale2x top-left subpixel rule: matching upper/left white
			// neighbours replace this black centre subpixel with opaque white.
			for c := 0; c < 4; c++ {
				if pix[(2*6+2)*4+c] != 255 {
					return fmt.Errorf("%s did not extend diagonal edge: %v", mode, pix[(2*6+2)*4:(2*6+2)*4+4])
				}
			}
		}
	}
	// A constant green field must not acquire encoded-color interpolation artefacts.
	uniform := surface.New(3, 3)
	uniform.Fill(image.Rect(0, 0, 3, 3), 0x07e0)
	for _, mode := range ScalingModes() {
		if err := gpu.DrawFitFiltered(large, uniform, mode); err != nil {
			return err
		}
		pix := make([]byte, 6*6*4)
		large.ReadPixels(pix)
		for i := 0; i < len(pix); i += 4 {
			if pix[i] != 0 || pix[i+1] != 255 || pix[i+2] != 0 || pix[i+3] != 255 {
				return fmt.Errorf("%s changed uniform green", mode)
			}
		}
	}
	return nil
}
