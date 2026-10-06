package app

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/surface"
)

func TestRaftBreakAnimationAndRecoveryOrigin(t *testing.T) {
	c, now, sent := shoreClient(2)
	c.Pics = picdb.New()
	c.Screen = surface.New(100, 100)
	c.World.CameraAt = &image.Point{}
	strip := image.NewNRGBA(image.Rect(0, 0, 6, 9))
	colors := []color.NRGBA{{R: 255, A: 255}, {G: 128, A: 255}, {B: 255, A: 255}}
	for y := 0; y < 9; y++ {
		for x := 0; x < 6; x++ {
			strip.SetNRGBA(x, y, colors[y/3])
		}
	}
	c.Pics.Add("48010_B", strip)
	c.World.Player.VehicleID, c.World.Player.VehicleSlot = 48016, 1
	c.World.Player.X, c.World.Player.Y = 45, 50
	c.rememberVehiclePosition(c.World.Player.ID)
	c.World.Relocate(image.Pt(22, 55))
	c.vehiclePacket([]byte{15, 15, 101, 0, 0, 0, 0x90, 0xbb})
	if len(c.vehicleEffects.effects) != 1 || c.vehicleEffects.effects[0].effect.X != 45 || c.vehicleEffects.effects[0].effect.Y != 50 {
		t.Fatal("break should use the water origin")
	}
	if c.World.Player.X != 22 || c.World.Player.Y != 55 || c.World.Player.VehicleID != 0 {
		t.Fatal("break reverted recovery or retained the vehicle")
	}
	if len(*sent) != 1 || !bytes.Equal((*sent)[0], []byte{15, 13, 101, 0, 0, 0}) {
		t.Fatal("native owner acknowledgment missing", *sent)
	}
	for frame := 0; frame < 3; frame++ {
		c.Screen = surface.New(100, 100)
		c.drawVehicleEffects()
		// Original break strip is horizontally centered, with each row's bottom at Y.
		want := surface.RGB565(colors[frame].R, colors[frame].G, colors[frame].B)
		if got := c.Screen.Pix[47*100+42]; got != want {
			t.Fatalf("frame %d: %#x, want %#x", frame, got, want)
		}
		*now = now.Add(70 * time.Millisecond)
		// Effect.Draw advances after drawing, so draw once at the boundary.
		c.drawVehicleEffects()
	}
	c.drawVehicleEffects()
	if len(c.vehicleEffects.effects) != 0 {
		t.Fatal("break animation did not expire")
	}
}

func TestRaftBreakPictureFromInstalledAssets(t *testing.T) {
	if _, err := os.Stat("../../../data/pictures/manifest.json"); err != nil {
		t.Skip("native picture assets not installed")
	}
	c, _, _ := shoreClient(2)
	c.Assets = login.NewAssets("../../../data")
	c.Pics = picdb.New()
	if !c.loadVehicleBreakPicture("48010_B") {
		t.Fatal("native raft break artwork missing")
	}
	i := c.Pics.Find("48010_B")
	w, h := c.Pics.Size(i)
	if w != 136 || h != 257 {
		t.Fatalf("native strip changed: %dx%d", w, h)
	}
}
