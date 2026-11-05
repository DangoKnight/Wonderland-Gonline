package app

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
)

func TestRewardNoticesBelowDialogueAndClickDismiss(t *testing.T) {
	c, now, _ := enteredClient(t)
	confirmed := 0
	c.confirm([]byte("Here is your reward."), func() { confirmed++ })
	c.Notices.Show([]byte("Obtain Coconut x1"), 6*time.Second, *now)
	*now = now.Add(300 * time.Millisecond)
	c.Frame()
	if c.Notices.panel.Top < c.Talk.Bounds().Max.Y+rewardNoticeGap {
		t.Fatal("reward overlaps dialogue")
	}
	p := c.Notices.panel
	c.GroundClick(p.Left+p.Width/2, p.Top+p.Height/2)
	if c.Notices.shown || !c.Talk.Shown() || confirmed != 0 || c.groundHeld {
		t.Fatal("notice click advanced dialogue or moved player")
	}
	f := newBattleResultsForm(c.Env)
	c.UI.Add(f)
	f.setLines([]string{"Player gained 20 EXP"})
	f.Show()
	c.battleResults.form = f
	c.battleResults.until = now.Add(6 * time.Second)
	c.Notices.Show([]byte("Obtain Wood x1"), 6*time.Second, *now)
	c.Frame()
	if f.Top < c.Talk.Bounds().Max.Y+rewardNoticeGap || c.Notices.panel.Top < f.Bottom()+rewardNoticeGap {
		t.Fatal("reward boards did not stack")
	}
}

func TestDialogueAbovePanels(t *testing.T) {
	for _, name := range []string{"social", "inventory", "settings"} {
		t.Run(name, func(t *testing.T) {
			c, now, _ := enteredClient(t)
			var panel seui.Control
			switch name {
			case "social":
				c.Social.Toggle()
				c.Social.SelectTab(hud.SocialSetUp)
				panel = c.Social
			case "inventory":
				c.Inventory.Show()
				panel = c.Inventory
			case "settings":
				c.Settings.Show()
				panel = c.Settings
			}
			confirmed := 0
			c.confirm([]byte("Ensure your data is correct. System will be sending bday gift on specified date. Log in game to receive it."), func() { confirmed++ })
			for range 10 {
				*now = now.Add(20 * time.Millisecond)
				c.Frame()
			}
			c.Sprites.WaitNative()
			c.Frame()
			// Render the dialogue over two different backgrounds to identify
			// its opaque text, buttons and frame. The decorative panel itself
			// is translucent, so its background may correctly show the window.
			dark, light := surface.New(ScreenWidth, ScreenHeight), surface.New(ScreenWidth, ScreenHeight)
			defer dark.Close()
			defer light.Close()
			light.Fill(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0xffff)
			c.Env.Screen = dark
			c.Talk.Draw()
			c.Env.Screen = light
			c.Talk.Draw()
			c.Env.Screen = c.Screen
			compared := 0
			for y := 0; y < ScreenHeight; y++ {
				for x := 0; x < ScreenWidth; x++ {
					at := y*ScreenWidth + x
					if !c.Talk.Contains(x, y) || dark.Pix[at] != light.Pix[at] {
						continue
					}
					if c.Screen.Pix[at] != dark.Pix[at] {
						t.Fatalf("%s covers dialogue at (%d,%d): %#x, want %#x", name, x, y, c.Screen.Pix[at], dark.Pix[at])
					}
					compared++
				}
			}
			if compared == 0 {
				t.Fatal("dialogue text and controls were not drawn")
			}
			button := image.Pt(-1, -1)
			for y := 0; y < ScreenHeight && button.X < 0; y++ {
				for x := 0; x < ScreenWidth; x++ {
					if row, _ := c.Talk.Pick(x, y); row == 0 {
						button = image.Pt(x, y)
						break
					}
				}
			}
			if button.X < 0 {
				t.Fatal("confirmation button was not drawn")
			}
			c.Input.X, c.Input.Y = button.X, button.Y
			c.Frame()
			if c.UI.MouseDown(seui.ButtonLeft, 0, button.X, button.Y) {
				t.Fatal("underlying window intercepted the dialogue click")
			}
			c.GroundClick(button.X, button.Y)
			if confirmed != 1 || c.Talk.Shown() || !panel.Base().Visible {
				t.Fatal("confirmation did not close once while retaining the underlying window")
			}
		})
	}
}

func TestSocialConfirmationSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR not set")
	}
	c, now, _ := enteredClient(t)
	c.Social.Toggle()
	c.Social.SelectTab(hud.SocialSetUp)
	c.confirm([]byte("Ensure your data is correct. System will be sending bday gift on specified date. Log in game to receive it."), func() {})
	for range 10 {
		*now = now.Add(20 * time.Millisecond)
		c.Frame()
	}
	save(t, c, filepath.Join(dir, "social_confirmation.png"))
}
