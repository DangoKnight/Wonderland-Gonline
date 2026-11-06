package render

import "wonderland-gonline/internal/localization"

import (
	_ "embed"
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"wonderland-gonline/client/wlo/surface"
)

// Scaling identifiers are durable client preference values. Shader indices
// are private compatibility operands for scaling.kage.
const (
	ScalingNearest              = "nearest"
	ScalingBilinear             = "bilinear"
	ScalingBicubic              = "bicubic"
	ScalingEdgeAware            = "edge-aware"
	ScalingEdgeAwareSharp       = "edge-aware-sharp"
	scalingShaderNearest        = 0
	scalingShaderBilinear       = 1
	scalingShaderBicubic        = 2
	scalingShaderEdgeAware      = 3
	scalingShaderEdgeAwareSharp = 4
	scalingShaderBuildEdges     = 5
)

func ScalingModes() []string {
	return []string{ScalingNearest, ScalingBilinear, ScalingBicubic, ScalingEdgeAware, ScalingEdgeAwareSharp}
}
func ScalingLabel(mode string) string {
	switch mode {
	case ScalingBilinear:
		return localization.UI("ui.render.bilinear_d3bae110")
	case ScalingBicubic:
		return localization.UI("ui.render.bicubic_d64f696c")
	case ScalingEdgeAware:
		return "Edge-aware"
	case ScalingEdgeAwareSharp:
		return localization.UI("ui.render.edge_aware_sharpen_d4367278")
	default:
		return localization.UI("ui.render.native_nearest_7f21baff")
	}
}
func ValidateScaling(mode string) error {
	if mode == "" {
		return nil
	} // Preferences predating selectable filters.
	for _, known := range ScalingModes() {
		if mode == known {
			return nil
		}
	}
	return fmt.Errorf(localization.UI("ui.render.unknown_scaling_filter_q_66ed1d57"), mode)
}
func scalingIndex(mode string) int {
	switch mode {
	case ScalingBilinear:
		return scalingShaderBilinear
	case ScalingBicubic:
		return scalingShaderBicubic
	case ScalingEdgeAware:
		return scalingShaderEdgeAware
	case ScalingEdgeAwareSharp:
		return scalingShaderEdgeAwareSharp
	default:
		return scalingShaderNearest
	}
}

//go:embed scaling.kage
var scalingShaderSource []byte

// DrawFitFiltered filters decoded RGBA colors, never packed RGB565 words.
// Targets and intermediate images stay on the GPU even with CPU composition.
func (p *Presenter) DrawFitFiltered(dst *ebiten.Image, src *surface.Surface, mode string) error {
	if err := ValidateScaling(mode); err != nil {
		p.DrawFit(dst, src)
		return err
	}
	if scalingIndex(mode) == scalingShaderNearest {
		p.DrawFit(dst, src)
		return nil
	}
	if p.scalingSource == nil || p.scalingSource.Bounds().Dx() != src.W || p.scalingSource.Bounds().Dy() != src.H {
		if p.scalingSource != nil {
			p.scalingSource.Deallocate()
		}
		p.scalingSource = ebiten.NewImage(src.W, src.H)
	}
	p.draw(p.scalingSource, src, ebiten.GeoM{})
	v := FitViewport(src.W, src.H, dst.Bounds().Dx(), dst.Bounds().Dy())
	var transform ebiten.GeoM
	transform.Scale(v.Scale, v.Scale)
	transform.Translate(v.X, v.Y)
	return p.DrawFinalFiltered(dst, p.scalingSource, transform, mode)
}

// DrawFinalFiltered applies the selected filter when the native framebuffer is
// enlarged into the OS window. This keeps filters effective at fixed resolution.
func (p *Presenter) DrawFinalFiltered(dst ebiten.FinalScreen, src *ebiten.Image, transform ebiten.GeoM, mode string) error {
	fallback := func() {
		dst.Fill(color.Black)
		dst.DrawImage(src, &ebiten.DrawImageOptions{GeoM: transform, Blend: ebiten.BlendCopy})
	}
	if err := ValidateScaling(mode); err != nil {
		fallback()
		return err
	}
	index := scalingIndex(mode)
	if index == scalingShaderNearest {
		fallback()
		return nil
	}
	if p.scalingShader == nil && p.scalingError == nil {
		p.scalingShader, p.scalingError = ebiten.NewShader(scalingShaderSource)
	}
	if p.scalingError != nil {
		fallback()
		return fmt.Errorf("scaling shader: %w", p.scalingError)
	}
	source := src
	drawWidth, drawHeight := src.Bounds().Dx(), src.Bounds().Dy()
	if index == scalingShaderEdgeAware || index == scalingShaderEdgeAwareSharp {
		// Compute neighbourhood rules at 2x native resolution, independently
		// of the possibly much larger OS window.
		drawWidth, drawHeight = drawWidth*2, drawHeight*2
		if p.scalingEdges == nil || p.scalingEdges.Bounds().Dx() != drawWidth || p.scalingEdges.Bounds().Dy() != drawHeight {
			if p.scalingEdges != nil {
				p.scalingEdges.Deallocate()
			}
			p.scalingEdges = ebiten.NewImage(drawWidth, drawHeight)
		}
		var edgeTransform ebiten.GeoM
		edgeTransform.Scale(2, 2)
		p.scalingEdges.DrawRectShader(src.Bounds().Dx(), src.Bounds().Dy(), p.scalingShader, &ebiten.DrawRectShaderOptions{GeoM: edgeTransform, Images: [4]*ebiten.Image{source}, Uniforms: map[string]any{"Filter": float32(scalingShaderBuildEdges)}, Blend: ebiten.BlendCopy})
		source = p.scalingEdges
		var half ebiten.GeoM
		half.Scale(0.5, 0.5)
		half.Concat(transform)
		transform = half
	}
	dst.Fill(color.Black)
	dst.DrawRectShader(drawWidth, drawHeight, p.scalingShader, &ebiten.DrawRectShaderOptions{GeoM: transform, Images: [4]*ebiten.Image{source}, Uniforms: map[string]any{"Filter": float32(index)}, Blend: ebiten.BlendCopy})
	return nil
}
