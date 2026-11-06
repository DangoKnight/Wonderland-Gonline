package app

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// Exercise actual native resize/maximize/restore without connecting a session.
func TestDisplayNativeResizeAndMaximize(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("WONDERLAND_TEST_GPU") == "" {
		t.Skip("Windows graphics context required")
	}
	if isolateGPUFixture(t) {
		return
	}
	d := newDisplayControls(filepath.Join(t.TempDir(), "display.json"))
	d.settings.Width, d.settings.Height = 1280, 720
	d.draft = d.settings
	g := &Game{display: d}
	d.apply = g.applyDisplay
	ebiten.SetWindowSize(760, 448)
	ebiten.SetWindowSizeLimits(displayMinimumWindowWidth, displayMinimumWindowHeight, -1, -1)
	g.applyDisplay(d.settings)
	probe := &displayWindowProbe{game: g}
	if err := ebiten.RunGame(probe); err != nil {
		t.Fatal(err)
	}
	if probe.err != nil {
		t.Fatal(probe.err)
	}
	if probe.tick != 120 {
		t.Fatalf("only %d native ticks completed", probe.tick)
	}
}

type displayWindowProbe struct {
	game                      *Game
	tick                      int
	err                       error
	windowWidth, windowHeight int
}

func (p *displayWindowProbe) Layout(w, h int) (int, int) { return p.game.Layout(w, h) }
func (p *displayWindowProbe) Draw(dst *ebiten.Image) {
	if dst.Bounds().Dx() != p.game.outputWidth || dst.Bounds().Dy() != p.game.outputHeight {
		p.err = fmt.Errorf("framebuffer dimensions differ from selected output")
	}
	dst.Fill(color.RGBA{R: 24, G: 32, B: 48, A: 255})
}
func (p *displayWindowProbe) Update() error {
	if p.err != nil {
		return ebiten.Termination
	}
	p.tick++
	switch p.tick {
	case 1:
		ebiten.SetWindowSize(960, 640)
	case 30:
		ebiten.MaximizeWindow()
	case 60:
		if !ebiten.IsWindowMaximized() {
			p.err = fmt.Errorf("native maximize did not take effect")
			return ebiten.Termination
		}
		width, height := 1280, 720
		if !displayResolutionPickingEnabled {
			width, height = workspaceWidth, ScreenHeight
		}
		if p.game.outputWidth != width || p.game.outputHeight != height {
			p.err = fmt.Errorf("maximize changed render resolution")
			return ebiten.Termination
		}
		ebiten.RestoreWindow()
	case 90:
		p.windowWidth, p.windowHeight = ebiten.WindowSize()
		d := p.game.display
		d.draft = d.settings
		d.draft.Width, d.draft.Height = 1920, 1080
		d.applyDraft(time.Now())
	case 120:
		w, h := ebiten.WindowSize()
		if w != p.windowWidth || h != p.windowHeight {
			p.err = fmt.Errorf("render-resolution change resized OS window: %dx%d to %dx%d", p.windowWidth, p.windowHeight, w, h)
		}
		width, height := 1920, 1080
		if !displayResolutionPickingEnabled {
			width, height = workspaceWidth, ScreenHeight
		}
		if p.game.outputWidth != width || p.game.outputHeight != height {
			p.err = fmt.Errorf("requested framebuffer resolution not applied")
		}
		return ebiten.Termination
	}
	return nil
}

func TestDisplayNativeSavedFullscreenStartup(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("WONDERLAND_TEST_GPU") == "" {
		t.Skip("Windows graphics context required")
	}
	for _, mode := range []string{displayFullscreen, displayBorderless} {
		for _, size := range [][2]int{{1366, 768}, {3840, 2160}} {
			t.Run(fmt.Sprintf("%s_%dx%d", mode, size[0], size[1]), func(t *testing.T) {
				if isolateGPUFixture(t) {
					return
				}
				path := filepath.Join(t.TempDir(), "display.json")
				saved := DisplaySettings{Mode: mode, Width: size[0], Height: size[1], FPS: 120}
				if err := saved.save(path); err != nil {
					t.Fatal(err)
				}
				d := newDisplayControls(path)
				g := &Game{display: d}
				d.apply = g.applyDisplay
				ebiten.SetWindowSize(workspaceWidth, ScreenHeight)
				// This is Run's startup sequence, before a normal window exists.
				g.applyDisplay(d.settings)
				probe := &displayStartupProbe{game: g}
				if err := ebiten.RunGame(probe); err != nil {
					t.Fatal(err)
				}
				if probe.err != nil {
					t.Fatal(probe.err)
				}
				if probe.tick != 30 {
					t.Fatalf("only %d startup ticks completed", probe.tick)
				}
			})
		}
	}
}

type displayStartupProbe struct {
	game *Game
	tick int
	err  error
}

func (p *displayStartupProbe) Layout(w, h int) (int, int) { return p.game.Layout(w, h) }
func (p *displayStartupProbe) Draw(dst *ebiten.Image) {
	s := p.game.display.settings
	width, height := s.Width, s.Height
	if !displayResolutionPickingEnabled {
		width, height = workspaceWidth, ScreenHeight
	}
	if dst.Bounds().Dx() != width || dst.Bounds().Dy() != height {
		p.err = fmt.Errorf("saved framebuffer resolution not applied")
	}
	dst.Fill(color.RGBA{R: 24, G: 32, B: 48, A: 255})
}
func (p *displayStartupProbe) Update() error {
	if p.err != nil {
		return ebiten.Termination
	}
	p.tick++
	switch p.tick {
	case 10:
		d := p.game.display
		d.draft = d.settings
		d.draft.Mode = displayWindowed
		d.applyDraft(time.Now())
	case 20:
		p.game.display.revert()
	case 30:
		return ebiten.Termination
	}
	return nil
}
