package app

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/render"
)

// Run explicitly in a graphics context; ordinary UI tests use the CPU renderer.
func TestTeamPetPortraitGPU(t *testing.T) {
	if os.Getenv("WONDERLAND_TEST_GPU") == "" {
		t.Skip("set WONDERLAND_TEST_GPU=1 for GPU portrait verification")
	}
	c, _, _ := enteredClient(t)
	if bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE"); bundle != "" {
		useCompiledPetAssets(t, c, bundle)
	}
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 12032, Name: []byte("Robinson")}
	c.Screen.Fill(image.Rect(0, 0, 800, 600), 0)
	c.Team.Show()
	c.Team.Names[0].Now = func() time.Time { return time.Unix(100, 0) }
	c.Team.Names[0].OnClick()
	c.UI.Draw()
	want := c.Screen.RGBA()
	device, err := render.NewDevice()
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	device.Attach(c.Screen)
	g := &petPortraitGPUCheck{client: c, want: want}
	if err = ebiten.RunGame(g); err != nil {
		t.Fatal(err)
	}
	if g.err != nil {
		t.Fatal(g.err)
	}
}

type petPortraitGPUCheck struct {
	client *Client
	want   *image.RGBA
	drawn  bool
	err    error
	paint  func()
}

func (g *petPortraitGPUCheck) Layout(int, int) (int, int) { return 800, 600 }
func (g *petPortraitGPUCheck) Draw(*ebiten.Image) {
	if g.drawn {
		return
	}
	g.client.Screen.Fill(image.Rect(0, 0, 800, 600), 0)
	if g.paint != nil {
		g.paint()
	} else {
		g.client.UI.Draw()
	}
	g.drawn = true
}
func (g *petPortraitGPUCheck) Update() error {
	if !g.drawn {
		return nil
	}
	got := g.client.Screen.RGBA()
	if !bytes.Equal(got.Pix, g.want.Pix) {
		for i := range got.Pix {
			if got.Pix[i] != g.want.Pix[i] {
				g.err = fmt.Errorf("GPU render differs at (%d,%d) channel %d: got %d want %d", (i/4)%got.Bounds().Dx(), (i/4)/got.Bounds().Dx(), i%4, got.Pix[i], g.want.Pix[i])
				break
			}
		}
	}
	return ebiten.Termination
}

func TestMonkeyIllustrationGPU(t *testing.T) {
	if os.Getenv("WONDERLAND_TEST_GPU") == "" {
		t.Skip("set WONDERLAND_TEST_GPU=1")
	}
	c, _, _ := enteredClient(t)
	if bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE"); bundle != "" {
		useCompiledPetAssets(t, c, bundle)
	}
	c.dispatch(movieFrameFor(11011, 2))
	if c.movie == nil || c.movie.background == nil {
		t.Fatal("compiled movie illustration missing")
	}
	paint := func() { c.drawMovie(c.movie) }
	paint()
	want := c.Screen.RGBA()
	device, err := render.NewDevice()
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	device.Attach(c.Screen)
	g := &petPortraitGPUCheck{client: c, want: want, paint: paint}
	if err = ebiten.RunGame(g); err != nil {
		t.Fatal(err)
	}
	if g.err != nil {
		t.Fatal(g.err)
	}
}
