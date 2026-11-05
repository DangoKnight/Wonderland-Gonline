package app

import (
	"os"
	"testing"
)

func TestXaolanIndoorBattleBackground(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	formation := remoteRecord(250, 2, 4, 2, 10001, 0)
	formation[2], formation[3] = 140, 0 // Native selector for scene 59140.
	for _, packet := range [][]byte{formation, remoteRecord(5, 7, 2, 2, 12049, 0), {50, 6, 4, 2, 0}, {52, 1}} {
		c.dispatch(packet)
	}
	finishSceneFade(c, now)
	if !c.battleFrame(true) || c.battle.assets.Scene(c.battle.state.Background) != 59140 {
		t.Fatal("room formation fell back to outdoor scenery")
	}
	picture := c.Pics.Find("59140")
	if picture < 0 {
		t.Fatal("room artwork not loaded by battle entry")
	}
	// This point is outside units and command controls. Check the real,
	// single-layer image is drawn with the normal battle camera crop.
	const x, y = 100, 100
	want := c.Pics.Pixel(picture, x+c.battle.camera.X, y+c.battle.camera.Y)
	if c.Screen.Pix[y*c.Screen.W+x] != want {
		t.Fatal("room layer not drawn at the native camera position")
	}
	if out := os.Getenv("XAOLAN_BATTLE_SNAPSHOT"); out != "" {
		savePNG(t, out, c)
	}
}
