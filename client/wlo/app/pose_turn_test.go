package app

import (
	"bytes"
	"image"
	"testing"

	"wonderland-gonline/client/wlo/hud"
)

func TestRightClickPoseFacing(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	// Cursor coordinates are relative to the actor's projected feet, including
	// at a map edge where the camera no longer centers the actor.
	for _, camera := range []image.Point{{0, 0}, {700, 800}} {
		c.World.CameraAt = &camera
		c.World.Player.X, c.World.Player.Y = camera.X+350, camera.Y+250
		for _, tc := range []struct {
			dx, dy int
			pose   byte
		}{
			{0, -80, 46}, {-80, -80, 47}, {-80, 0, 48}, {-80, 80, 49},
			{0, 80, 50}, {80, 80, 51}, {80, 0, 52}, {80, -80, 53},
		} {
			c.World.Player.Direction = 46
			if tc.pose == 46 {
				c.World.Player.Direction = 50
			}
			before := len(sent())
			c.GroundRightClick(350+tc.dx, 250+tc.dy)
			got := sent()
			if len(got) != before+1 || !bytes.Equal(got[before], []byte{32, 2, tc.pose}) {
				t.Fatalf("cursor (%d,%d), camera %v: packets %v", tc.dx, tc.dy, camera, got[before:])
			}
			if c.World.Player.Direction != int32(tc.pose) || c.World.Player.X != camera.X+350 || c.World.Player.Y != camera.Y+250 || c.World.Walking() {
				t.Fatal("turn changed the position, lost the sit pose, or started walking")
			}
			c.GroundRightClick(350+tc.dx, 250+tc.dy)
			if len(sent()) != before+1 {
				t.Fatal("unchanged facing sent another pose packet")
			}
		}
	}
}

func TestRightClickPoseFamiliesAndExpressions(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.World.CameraAt = &image.Point{}
	c.World.Player.X, c.World.Player.Y = 350, 250
	sent := wire(t, c)
	c.playEmote(103)
	expression := c.World.Expressions[c.World.Player.ID]
	for _, tc := range []struct {
		from, want int32
		x          int
	}{
		{26, 27, 430}, {27, 26, 270}, // Lie: two facings.
		{16, 17, 430}, {17, 16, 270}, // Two-facing stance.
		{54, 60, 430}, {60, 56, 270}, // Eight-facing alternate sit.
		{64, 65, 430}, {65, 64, 270}, // Two-facing action.
		{8, 14, 430}, {14, 10, 270}, // Standing stays in its family.
	} {
		c.World.Player.Direction = tc.from
		before := len(sent())
		c.GroundRightClick(tc.x, 250)
		got := sent()
		if c.World.Player.Direction != tc.want || len(got) != before+1 || !bytes.Equal(got[before], []byte{32, 2, byte(tc.want)}) {
			t.Fatalf("action %d to cursor X=%d: action %d, packets %v", tc.from, tc.x, c.World.Player.Direction, got[before:])
		}
	}
	if c.World.Expressions[c.World.Player.ID] != expression {
		t.Fatal("turning restarted or removed the independent expression")
	}
	before := len(sent())
	c.GroundRightClick(350, 250)
	if c.World.Player.Direction != 10 || len(sent()) != before {
		t.Fatal("clicking the actor's exact position changed its facing")
	}
}

func TestRightClickPoseBlocked(t *testing.T) {
	for _, name := range []string{"loading", "event", "held", "battle", "remote-battle", "vehicle", "movie", "minigame", "modal", "hovered", "dialogue", "frozen", "outside", "walking"} {
		t.Run(name, func(t *testing.T) {
			c, _, _ := enteredClient(t)
			c.mapReady = true
			c.World.Player.Direction = 46
			sent := wire(t, c)
			cx, cy := c.World.Camera()
			x, y := c.World.Player.X-cx+80, c.World.Player.Y-cy
			switch name {
			case "loading":
				c.mapReady = false
			case "event":
				c.event.active = true
			case "held":
				c.held = true
			case "battle":
				c.battle.state.Active = true
			case "remote-battle":
				c.remote.battle.active = true
			case "vehicle":
				c.World.Player.VehicleID = 48016
			case "movie":
				c.movie = &moviePlay{}
			case "minigame":
				c.sport = &sportPlay{}
			case "modal":
				c.UI.Modal = c.Settings
			case "hovered":
				c.Input.Hovered = c.Social
			case "dialogue":
				c.Talk.Say([]byte("Wait"), hud.Speaker{}, nil)
			case "frozen":
				c.fade.step = 1
			case "outside":
				x = ScreenWidth
			case "walking":
				c.World.Player.Direction = 8
				c.GroundClick(300, 350)
				if !c.World.Walking() {
					t.Fatal("walking fixture did not start")
				}
			}
			before, count := c.World.Player, len(sent())
			c.GroundRightClick(x, y)
			p := c.World.Player
			if p.X != before.X || p.Y != before.Y || p.Direction != before.Direction || len(sent()) != count {
				t.Fatal("blocked right click changed player state or sent a packet")
			}
		})
	}
}

func TestRightClickPoseWithoutWorld(t *testing.T) {
	c := &Client{}
	c.GroundRightClick(400, 300)
}
