package app

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"image"
	"os"
	"testing"
	"wonderland-gonline/client/wlo/housing"
	"wonderland-gonline/client/wlo/world"
)

func tentCapture(t *testing.T) map[string][]byte {
	t.Helper()
	b, err := os.ReadFile("../housing/testdata/legacy_tents.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]string
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for k, v := range doc {
		out[k], err = hex.DecodeString(v)
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestTentFurnitureMoveWaitsForServer(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	p := tentCapture(t)
	c.World.Player.ID = 4617671
	c.warp(p["interior_warp"][1:])
	c.tentPacket(p["tree"])
	c.mapReady = true
	c.Input.Hovered = nil
	before := c.housing.Furniture[1]
	c.tentDrag = &tentDrag{index: 1, start: housing.Pixel(42, 39, 1)}
	to := housing.Pixel(32, 38, 1)
	cx, cy := c.World.Camera()
	if !c.holdTentFurniture(false, to.X-cx, to.Y-cy) {
		t.Fatal("release was not consumed")
	}
	packets := sent()
	if len(packets) == 0 || !bytes.Equal(packets[len(packets)-1], p["move"]) {
		t.Fatal("move differs from the captured native packet", packets)
	}
	if c.housing.Furniture[1] != before {
		t.Fatal("request moved furniture before server confirmation")
	}
	c.tentPacket(p["move"])
	if c.housing.Furniture[1].X != 32 {
		t.Fatal("confirmation did not apply movement")
	}
}

func TestTentCapturedClientLifecycle(t *testing.T) {
	c, _, _ := enteredClient(t)
	wire(t, c)
	p := tentCapture(t)
	c.World.Player.ID = 4617671
	if !c.tentPacket(p["open"]) || c.openTentItem != 36002 {
		t.Fatal("own tent not recognized")
	}
	c.warp(p["interior_warp"][1:])
	if c.World.Player.Map != 63508 || c.World.Player.X != 460 || c.World.Player.Y != 700 {
		t.Fatal("interior entry failed", c.World.Player)
	}
	if c.tentKind != 1 || c.housing.Floor != 0 || c.tentRoomKinds[1] != 1 {
		t.Fatal("native floor/room pairs were not decoded")
	}
	if !c.tentPacket(p["tree"]) || len(c.housing.Furniture) != 2 {
		t.Fatal("interior furniture missing")
	}
	backdrop := c.tentBackdrop
	if !c.tentPacket(p["move"]) || c.housing.Furniture[1].X != 32 {
		t.Fatal("furniture move lost")
	}
	if c.tentBackdrop != backdrop {
		t.Fatal("furniture movement rebuilt an unchanged room background")
	}
	if c.tentBackdrop == nil {
		t.Fatal("no interior background")
	}
	if out := os.Getenv("WLO_TENT_REFERENCE"); out != "" {
		c.Screen.Fill(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0)
		c.drawWorldScene()
		savePNG(t, out, c)
	}
	c.World.Peers = map[uint32]*world.Peer{99: {}}
	c.warp(p["interior_warp"][1:])
	if len(c.World.Peers) != 0 || len(c.housing.Furniture) != 0 {
		t.Fatal("private room retained another room's occupants/furniture")
	}
	c.warp(p["outside_warp"][1:])
	if c.World.Player.Map != 12000 || c.World.Player.X != 2442 || c.World.Player.Y != 2055 || c.tentBackdrop != nil {
		t.Fatal("owner did not return to the server's exterior location")
	}
	if !c.tentPacket(p["signs"]) || !c.tentPacket(p["closed"]) || len(c.housing.Signs) != 2 || c.openTentItem != 0 {
		t.Fatal("closed tent remained on the field")
	}
}

func TestTentCapturedGuestEviction(t *testing.T) {
	c, _, _ := enteredClient(t)
	wire(t, c)
	p := tentCapture(t)
	c.World.Player.ID = 4617590
	c.warp(p["guest_warp"][1:])
	c.tentPacket(p["tree"])
	if c.housing.Owner == c.World.Player.ID {
		t.Fatal("guest acquired furniture ownership")
	}
	c.warp(p["guest_return"][1:])
	if !c.tentPacket(p["evicted"]) || c.World.Player.Map != 12000 || c.World.Player.X != 2402 || c.World.Player.Y != 2095 {
		t.Fatal("eviction must preserve the server-provided return coordinates")
	}
	if len(c.housing.Furniture) != 0 || c.tentBackdrop != nil {
		t.Fatal("guest retained private room data after eviction")
	}
}
