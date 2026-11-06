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

func TestXaolanForestBattleBackground(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	formation := remoteRecord(250, 2, 4, 2, 10001, 0)
	formation[2], formation[3] = 90, 0 // Scene 59090, Jungle.
	for _, packet := range [][]byte{formation,
		remoteRecord(5, 7, 2, 1, 12049, 0),
		remoteRecord(5, 7, 2, 4, 12050, 0),
		remoteRecord(5, 7, 1, 2, 14155, 0), {50, 6, 4, 2, 0}, {52, 1}} {
		c.dispatch(packet)
	}
	finishSceneFade(c, now)
	if !c.battleFrame(true) || c.battle.assets.Scene(c.battle.state.Background) != 59090 {
		t.Fatal("forest rescue fell back to grassland")
	}
	picture := c.Pics.Find("59090")
	if picture < 0 {
		t.Fatal("Jungle artwork not loaded")
	}
	const x, y = 400, 500
	bg := c.battle.assets.Background(c.battle.state.Background)
	want := c.Pics.Pixel(picture, x+c.battle.camera.X, y+c.battle.camera.Y-int(bg.Heights[0]))
	if out := os.Getenv("XAOLAN_FOREST_BATTLE_SNAPSHOT"); out != "" {
		savePNG(t, out, c)
	}
	if c.Screen.Pix[y*c.Screen.W+x] != want {
		t.Fatal("Jungle ground not drawn at native camera crop", c.Screen.Pix[y*c.Screen.W+x], want, bg, c.battle.camera)
	}
}

func TestLakesideBattleBackdropIgnoresFormationMetadata(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	formation := remoteRecord(250, 2, 4, 2, 10001, 0)
	formation[2], formation[3] = 91, 25 // Native byte selector and independent metadata.
	for _, p := range [][]byte{formation, remoteRecord(5, 7, 2, 2, 17049, 0), {50, 6, 4, 2, 0}, {52, 1}} {
		c.dispatch(p)
	}
	finishSceneFade(c, now)
	if !c.battleFrame(true) || c.battle.assets.Scene(c.battle.state.Background) != 59091 {
		t.Fatal("Lakeside Jungle fell back to grassland", c.battle.state.Background)
	}
	bg := c.battle.assets.Background(c.battle.state.Background)
	if bg.Pictures != [3]uint16{59093, 59094, 59095} {
		t.Fatal("wrong native lake layers", bg)
	}
	if out := os.Getenv("LAKESIDE_BATTLE_SNAPSHOT"); out != "" {
		savePNG(t, out, c)
	}
}

func TestNorthIslandSeaflowerBeachBackground(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	formation := remoteRecord(250, 2, 4, 2, 10001, 0)
	formation[2], formation[3] = 50, 0
	for _, p := range [][]byte{formation, remoteRecord(5, 7, 2, 2, 17416, 0), {50, 6, 4, 2, 0}, {52, 1}} {
		c.dispatch(p)
	}
	finishSceneFade(c, now)
	if !c.battleFrame(true) || c.battle.assets.Scene(c.battle.state.Background) != 59050 {
		t.Fatal("coastal fight fell back to grassland")
	}
	if bg := c.battle.assets.Background(c.battle.state.Background); bg.Pictures != [3]uint16{59050, 59051, 59052} {
		t.Fatal("wrong Beach layers", bg)
	}
}
