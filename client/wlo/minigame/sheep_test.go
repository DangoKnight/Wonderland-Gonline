package minigame

import (
	"image"
	"image/color"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/surface"
)

func TestSheepNativeHammerFramesAndCleanDream(t *testing.T) {
	pics := picdb.New()
	strip := image.NewNRGBA(image.Rect(0, 0, 4, 12))
	colors := []color.NRGBA{{R: 255, A: 255}, {G: 200, A: 255}, {B: 255, A: 255}}
	for y := 0; y < 12; y++ {
		for x := 0; x < 4; x++ {
			strip.SetNRGBA(x, y, colors[y/4])
		}
	}
	pics.Add("Hammer_1", strip)
	now := time.Unix(100, 0)
	g := NewSheep(pics, 2)
	g.Rand = func(int) int { return 0 }
	g.Begin(now)
	dst := surface.New(800, 600)
	g.Draw(dst, 10, 10, now)
	if dst.Pix[200*dst.W+375] != surface.RGB565(255, 0, 0) {
		t.Fatal("ready hammer missing at native position")
	}
	g.Click(350, 300, now)
	for _, phase := range []struct {
		tick  int
		color uint16
	}{
		{3, surface.RGB565(255, 0, 0)}, {4, surface.RGB565(0, 200, 0)},
		{7, surface.RGB565(0, 200, 0)}, {8, surface.RGB565(0, 0, 255)},
		{10, surface.RGB565(0, 0, 255)}, {11, surface.RGB565(255, 0, 0)},
	} {
		at := now.Add(time.Duration(phase.tick) * 30 * time.Millisecond)
		g.Update(at)
		g.Draw(dst, 700, 500, at)
		if dst.Pix[200*dst.W+375] != phase.color {
			t.Fatal("wrong native hammer frame", phase.tick)
		}
		if dst.Pix[234*dst.W+307] != arcadePanel || dst.Pix[350*dst.W+365] != arcadePanel {
			t.Fatal("diagnostic hit box or cue label over dream")
		}
	}
	// Repeated strikes animate without charging a second answer for one cue.
	g.Click(350, 300, now.Add(330*time.Millisecond))
	g.Draw(dst, 0, 0, now.Add(450*time.Millisecond))
	if g.Lives != 2 || dst.Pix[200*dst.W+375] != surface.RGB565(0, 200, 0) {
		t.Fatal("repeated strike did not animate independently of scoring")
	}
}
