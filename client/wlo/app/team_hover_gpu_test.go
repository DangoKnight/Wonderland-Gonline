package app

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"os"
	"testing"
	"wonderland-gonline/client/wlo/render"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/seui"
)

func TestJoinTeamHoverGPU(t *testing.T) {
	if os.Getenv("WONDERLAND_TEST_GPU") == "" {
		t.Skip("set WONDERLAND_TEST_GPU=1")
	}
	c, _, _ := enteredClient(t)
	human := c.World.Body.(*role.Human)
	human.Now = c.Now
	human.Hold(0, false)
	human.SetLit(true)
	template := c.npcTemplates[17162]
	pet := role.NewNPC(c.lib, template.Look, template.Colors)
	pet.Now = c.Now
	pet.SetLit(true)
	paint := func() {
		human.DrawBody(c.Screen, 180, 300, 11)
		pet.Draw(c.Screen, 350, 300, 11)
		for _, control := range c.Team.Instances.Children {
			b, ok := control.(*seui.FixedButton)
			if !ok || b.Image != c.Env.Pics.Find("Btn_ArrowL_5") && b.Image != c.Env.Pics.Find("Btn_ArrowR_5") {
				continue
			}
			oldLeft := b.Left
			for state := byte(0); state < 3; state++ {
				b.State = state
				b.Left = oldLeft + int(state)*60
				b.Paint()
			}
			b.Left = oldLeft
		}
	}
	// Loose assets derive palettes in the background. Warm those sources
	// before comparing two renders, so neither changes halfway through.
	paint()
	c.lib.Sprites.WaitNative()
	c.Screen.Fill(image.Rect(0, 0, 800, 600), 0)
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
