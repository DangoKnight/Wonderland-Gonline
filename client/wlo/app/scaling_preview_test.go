package app

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"wonderland-gonline/client/wlo/render"
)

func TestDisplayScalingVisualComparison(t *testing.T) {
	if os.Getenv("WONDERLAND_TEST_GPU") == "" {
		t.Skip("graphics context required")
	}
	if isolateGPUFixture(t) {
		return
	}
	c, _, _ := enteredClient(t)
	c.Frame()
	c.Sprites.WaitNative()
	c.Frame()
	p, err := render.New(render.GPU)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	probe := &scalingPreviewGame{presenter: p, client: c, dir: os.Getenv("SCALING_SNAPSHOT_DIR")}
	ebiten.SetWindowSize(800, 600)
	if err := ebiten.RunGame(probe); err != nil {
		t.Fatal(err)
	}
	if probe.err != nil {
		t.Fatal(probe.err)
	}
	if probe.index != len(render.ScalingModes()) {
		t.Fatal("not all filters rendered")
	}
}

type scalingPreviewGame struct {
	presenter *render.Presenter
	client    *Client
	target    *ebiten.Image
	index     int
	drawn     bool
	err       error
	dir       string
}

func (g *scalingPreviewGame) Layout(int, int) (int, int) { return 1200, 900 }
func (g *scalingPreviewGame) Update() error {
	if g.err != nil {
		return ebiten.Termination
	}
	if !g.drawn {
		return nil
	}
	if g.dir != "" {
		pixels := image.NewRGBA(image.Rect(0, 0, 1200, 900))
		g.target.ReadPixels(pixels.Pix)
		path := filepath.Join(g.dir, render.ScalingModes()[g.index]+".png")
		f, err := os.Create(path)
		if err != nil {
			g.err = err
			return ebiten.Termination
		}
		err = png.Encode(f, pixels)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			g.err = fmt.Errorf("snapshot: %v %v", err, closeErr)
			return ebiten.Termination
		}
	}
	g.index++
	g.drawn = false
	if g.index == len(render.ScalingModes()) {
		g.target.Deallocate()
		return ebiten.Termination
	}
	return nil
}
func (g *scalingPreviewGame) Draw(dst *ebiten.Image) {
	if g.drawn {
		return
	}
	if g.target == nil {
		g.target = ebiten.NewImage(1200, 900)
	}
	if err := g.presenter.DrawFitFiltered(g.target, g.client.Screen, render.ScalingModes()[g.index]); err != nil {
		g.err = err
		return
	}
	dst.DrawImage(g.target, nil)
	g.drawn = true
}
