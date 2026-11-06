package app

import (
	"image"
	"testing"
	"time"
)

// sceneBrightness is the mean channel value of the map area above the HUD.
func sceneBrightness(c *Client) float64 {
	img := c.Screen.RGBA()
	area := image.Rect(100, 100, 700, 400)
	sum := 0
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			o := img.PixOffset(x, y)
			sum += int(img.Pix[o]) + int(img.Pix[o+1]) + int(img.Pix[o+2])
		}
	}
	return float64(sum) / float64(3*area.Dx()*area.Dy())
}

// TestBattleTransitions: entering darkens the map (alpha 12 per frame to
// 180), then reveals the battle field. Leaving shows the map at alpha 180
// and lightens it over 15 frames.
func TestBattleTransitions(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	c.Sprites.WaitNative()
	c.Frame()
	lit := sceneBrightness(c)
	near := func(got, want float64) bool { return got > want-4 && got < want+4 }
	enemy := remoteRecord(5, 7, 2, 2, 20001, 0)
	enemy[2] = 2
	c.dispatch(remoteRecord(250, 2, 4, 2, 10001, 0))
	c.dispatch(enemy)
	if !c.enteringBattle() {
		t.Fatal("battle started without the entry transition")
	}
	c.Frame()
	if got := sceneBrightness(c); !c.enteringBattle() || c.battleFrame(false) || !near(got, lit*(255-sceneFadeAlphaStep)/255) {
		t.Fatalf("first entry frame %.1f, lit %.1f", got, lit)
	}
	for i := 1; i < sceneFadeSteps; i++ {
		*now = now.Add(sceneFadeFrame)
		c.Frame()
	}
	if got := sceneBrightness(c); c.sceneFade.step != sceneFadeSteps || !near(got, lit*(255-sceneFadeSteps*sceneFadeAlphaStep)/255) {
		t.Fatalf("last entry frame step %d brightness %.1f", c.sceneFade.step, got)
	}
	*now = now.Add(sceneFadeFrame)
	c.Frame()
	if c.enteringBattle() || !c.battleFrame(false) {
		t.Fatal("battle field did not follow the entry transition")
	}
	if c.sceneFade.mode != sceneFadeRevealBattle || c.sceneFade.step != sceneFadeSteps {
		t.Fatal("battle field skipped reveal")
	}
	c.battleFrame(true)
	battleLit := sceneBrightness(c)
	c.drawSceneFade()
	if got := sceneBrightness(c); !near(got, battleLit*(255-sceneFadeSteps*sceneFadeAlphaStep)/255) {
		t.Fatalf("first battle reveal %.1f, lit %.1f", got, battleLit)
	}
	for i := 1; i < sceneFadeSteps; i++ {
		*now = now.Add(sceneFadeFrame)
		c.Frame()
	}
	if got := sceneBrightness(c); c.sceneFade.step != 1 || !near(got, battleLit*(255-sceneFadeAlphaStep)/255) {
		t.Fatalf("last battle reveal step %d brightness %.1f", c.sceneFade.step, got)
	}
	*now = now.Add(sceneFadeFrame)
	c.Frame()
	if c.sceneFade.mode != sceneFadeNone {
		t.Fatal("battle reveal never finished")
	}

	*now = now.Add(time.Second)
	c.dispatch([]byte{11, 0, 0x11, 0x27, 0, 0, 0, 0})
	c.Frame()
	if got := sceneBrightness(c); c.sceneFade.mode != sceneFadeLeaveBattle || !near(got, lit*(255-sceneFadeSteps*sceneFadeAlphaStep)/255) {
		t.Fatalf("first exit frame mode %d brightness %.1f", c.sceneFade.mode, got)
	}
	for i := 1; i < sceneFadeSteps; i++ {
		*now = now.Add(sceneFadeFrame)
		c.Frame()
	}
	if got := sceneBrightness(c); c.sceneFade.step != 1 || !near(got, lit*(255-sceneFadeAlphaStep)/255) {
		t.Fatalf("last exit frame step %d brightness %.1f lit %.1f", c.sceneFade.step, got, lit)
	}
	*now = now.Add(sceneFadeFrame)
	c.Frame()
	if got := sceneBrightness(c); c.sceneFade.mode != sceneFadeNone || !near(got, lit) {
		t.Fatalf("exit left mode %d brightness %.1f, lit %.1f", c.sceneFade.mode, got, lit)
	}
}
