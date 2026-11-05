package app

import (
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"wonderland-gonline/client/wlo/surface"
)

// Native FUN_003075f0 compares SceneData's title groups and plays t<group>
// through FUN_0021d278. t10002 (Kelan) has seven frames. This presentation
// retains the expanding water reveal but uses elapsed time and English text.
const (
	areaTitleReveal            = 1500 * time.Millisecond
	areaTitleHold              = 200 * time.Millisecond
	areaTitleFade              = 500 * time.Millisecond
	areaTitleDuration          = areaTitleReveal + areaTitleHold + areaTitleFade
	areaTitleRippleEnd         = areaTitleDuration - 80*time.Millisecond
	areaTitleFontSize          = 30
	areaTitleMaxTextWidth      = 400
	areaTitleMargin            = 48
	areaTitleHeight            = 128
	areaTitleEllipseScale      = 2.2
	areaTitleWaveWidth         = 24.0
	areaTitleRippleSpacing     = 18.0
	areaTitleRippleWidth       = 2.5
	areaTitleRippleCount       = 3
	areaTitleDistortion        = 5.0
	areaTitleRippleEdgeStart   = .68
	areaTitleRippleEdgeEnd     = .98
	areaTitleTextWaveAmplitude = 1.5
	areaTitleTextWaveLength    = 110.0
	areaTitleTextWavePeriod    = 1600 * time.Millisecond
)

type areaTitle struct {
	identity       uint32
	name           string
	pending        bool
	started        time.Time
	mask           *image.Alpha
	canvas         *surface.Surface
	pixels         *image.NRGBA
	radii          []float64
	rippleEnvelope []float64
	textWave       []float64
	maxRadius      float64
}

func (c *Client) queueAreaTitle() {
	if c.World == nil {
		return
	}
	mapID := c.World.Player.Map
	var group uint16
	if c.resources != nil {
		group = c.resources.sceneTitleGroups[c.mapScenes[mapID]]
	}
	identity := uint32(mapID)
	if group != 0 {
		identity = uint32(group) << 16
	}
	previous := c.areaTitle.identity
	c.areaTitle = areaTitle{identity: identity}
	name := strings.TrimSpace(c.World.SceneName)
	if identity != previous && name != "" {
		c.areaTitle.name, c.areaTitle.pending = name, true
	}
}

func (c *Client) areaTitleTick(now time.Time) {
	t := &c.areaTitle
	if !t.started.IsZero() && (now.Sub(t.started) >= areaTitleDuration || c.battle.state.Active || c.movie != nil) {
		*t = areaTitle{identity: t.identity}
	}
	if !t.pending || !c.mapReady || c.sceneFrozen() || c.movie != nil || c.battle.state.Active || c.sceneFade.mode != sceneFadeNone {
		return
	}
	t.pending = false
	if !t.makeArtwork() {
		return
	}
	t.started = now
}

func (t *areaTitle) makeArtwork() bool {
	ttf, err := opentype.Parse(gobolditalic.TTF)
	if err != nil {
		return false
	}
	size := float64(areaTitleFontSize)
	face, err := opentype.NewFace(ttf, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return false
	}
	width := font.MeasureString(face, t.name).Ceil()
	if width > areaTitleMaxTextWidth {
		face.Close()
		size *= float64(areaTitleMaxTextWidth) / float64(width)
		face, err = opentype.NewFace(ttf, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
		if err != nil {
			return false
		}
		width = font.MeasureString(face, t.name).Ceil()
	}
	defer face.Close()
	t.mask = image.NewAlpha(image.Rect(0, 0, width+2*areaTitleMargin, areaTitleHeight))
	metrics := face.Metrics()
	baseline := (areaTitleHeight + metrics.Ascent.Ceil() - metrics.Descent.Ceil()) / 2
	d := font.Drawer{Dst: t.mask, Src: image.NewUniform(color.White), Face: face, Dot: fixed.P(areaTitleMargin, baseline)}
	d.DrawString(t.name)
	t.canvas = surface.New(t.mask.Rect.Dx(), t.mask.Rect.Dy())
	t.pixels = image.NewNRGBA(t.mask.Rect)
	t.radii = make([]float64, len(t.canvas.Pix))
	t.rippleEnvelope = make([]float64, len(t.canvas.Pix))
	t.textWave = make([]float64, t.canvas.W)
	cx, cy := float64(t.canvas.W)/2, float64(t.canvas.H)/2
	t.maxRadius = max(cx, cy*areaTitleEllipseScale) * areaTitleRippleEdgeEnd
	for y := 0; y < t.canvas.H; y++ {
		for x := 0; x < t.canvas.W; x++ {
			radius := math.Hypot(float64(x)-cx, (float64(y)-cy)*areaTitleEllipseScale)
			t.radii[y*t.canvas.W+x] = radius
			// Dissolve along an ellipse inside the canvas, so a ring never
			// reaches a visible rectangular clipping edge.
			edge := math.Hypot((float64(x)-cx)/cx, (float64(y)-cy)/cy)
			falloff := max(0.0, min(1.0, (edge-areaTitleRippleEdgeStart)/(areaTitleRippleEdgeEnd-areaTitleRippleEdgeStart)))
			t.rippleEnvelope[y*t.canvas.W+x] = 1 - falloff*falloff*(3-2*falloff)
		}
	}
	return true
}

func (t *areaTitle) render(elapsed time.Duration) *surface.Surface {
	if t.canvas == nil || elapsed < 0 || elapsed >= areaTitleDuration {
		return nil
	}
	// Ripple travel occupies almost the whole effect, instead of completing
	// early and leaving a static title. The last ring dissolves just before exit.
	p := min(1.0, float64(elapsed)/float64(areaTitleRippleEnd))
	front := p * (t.maxRadius + (areaTitleRippleCount-1)*areaTitleRippleSpacing + areaTitleRippleWidth)
	fade := 1.0
	if elapsed > areaTitleReveal+areaTitleHold {
		fade = 1 - float64(elapsed-areaTitleReveal-areaTitleHold)/float64(areaTitleFade)
	}
	w := t.canvas.W
	phase := float64(elapsed) / float64(areaTitleTextWavePeriod) * 2 * math.Pi
	// One displacement per column keeps the text wave inexpensive. Sample the
	// glyph mask between pixels to avoid a one-pixel snap at high frame rates.
	for x := range t.textWave {
		t.textWave[x] = math.Sin(float64(x)*2*math.Pi/areaTitleTextWaveLength-phase) * areaTitleTextWaveAmplitude
	}
	for y := 0; y < t.canvas.H; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			radius := t.radii[i]
			wave := radius - front
			revealWave := wave + (areaTitleRippleCount-1)*areaTitleRippleSpacing
			sx, sy := float64(x)+t.textWave[x], float64(y)
			if math.Abs(revealWave) < areaTitleWaveWidth {
				strength := 1 - math.Abs(revealWave)/areaTitleWaveWidth
				shift := math.Sin(revealWave/areaTitleWaveWidth*math.Pi) * areaTitleDistortion * strength
				sx += shift
			}
			coverage := 0.0
			if wave <= 0 {
				coverage = areaTitleMaskCoverage(t.mask, sx, sy) * areaTitleTextOpacity(wave)
			}
			ring := 0.0
			for n := 0; n < areaTitleRippleCount; n++ {
				distance := math.Abs(wave + float64(n)*areaTitleRippleSpacing)
				v := max(0.0, 1-distance/areaTitleRippleWidth)
				ring = max(ring, v*v*.65)
			}
			ring *= t.rippleEnvelope[i]
			// Text over the pale blue ring, with straight alpha for scene
			// compositing. Additive light would saturate bright terrain and make
			// partially revealed letters look opaque before the inner ring.
			ringBehind := ring * (1 - coverage)
			alpha := coverage + ringBehind
			out := color.NRGBA{}
			if alpha > 0 {
				out = color.NRGBA{
					R: byte(min(255.0, (coverage+ringBehind*.7)/alpha*255)),
					G: byte(min(255.0, (coverage+ringBehind*.85)/alpha*255)),
					B: 255,
					A: byte(min(255.0, alpha*fade*255)),
				}
			}
			t.pixels.SetNRGBA(x, y, out)
			// Keep a black-backed preview for deterministic snapshots.
			t.canvas.Pix[i] = surface.RGB565(byte(uint16(out.R)*uint16(out.A)/255), byte(uint16(out.G)*uint16(out.A)/255), byte(uint16(out.B)*uint16(out.A)/255))
		}
	}
	t.canvas.Revision++
	return t.canvas
}

// Signed distance from the outer ring: its leading edge is invisible; opacity
// increases through the intermediate rings and reaches one at the inner ring.
func areaTitleTextOpacity(wave float64) float64 {
	return max(0.0, min(1.0, -wave/((areaTitleRippleCount-1)*areaTitleRippleSpacing)))
}

type areaTitleFrame struct {
	pixels   *image.NRGBA
	revision uint64
}

func areaTitleMaskCoverage(mask *image.Alpha, x, y float64) float64 {
	if x < 0 || y < 0 || x >= float64(mask.Rect.Dx()-1) || y >= float64(mask.Rect.Dy()-1) {
		return 0
	}
	ix, iy := int(x), int(y)
	fx, fy := x-float64(ix), y-float64(iy)
	i := iy*mask.Stride + ix
	top := float64(mask.Pix[i])*(1-fx) + float64(mask.Pix[i+1])*fx
	bottom := float64(mask.Pix[i+mask.Stride])*(1-fx) + float64(mask.Pix[i+mask.Stride+1])*fx
	return (top*(1-fy) + bottom*fy) / 255
}

func (c *Client) drawAreaTitle(now time.Time) {
	t := &c.areaTitle
	if t.started.IsZero() {
		return
	}
	if canvas := t.render(now.Sub(t.started)); canvas != nil {
		_ = c.Screen.DrawSprite((ScreenWidth-canvas.W)/2, (ScreenHeight-canvas.H)/2, areaTitleFrame{t.pixels, canvas.Revision}, func() (*image.NRGBA, error) { return t.pixels, nil })
	}
}
