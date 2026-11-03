package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
)

type companionPoseCapture struct {
	*role.NPC
	action int
}

func (p *companionPoseCapture) Draw(dst *surface.Surface, x, y, action int) {
	p.action = action
	p.NPC.Draw(dst, x, y, action)
}

func TestCompanionEmoteAndRecoveryIntegration(t *testing.T) {
	c, now, _ := enteredClient(t)
	if bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE"); bundle != "" {
		useCompiledPetAssets(t, c, bundle)
	}
	c.npcTemplates, _ = world.NPCTemplates(c.Assets)
	sent := wire(t, c)
	c.showCompanion(c.World.Player.ID, 14162, []byte("Roca"))
	pet := c.World.Companions[c.World.Player.ID]
	if pet == nil {
		t.Fatal("companion missing")
	}
	painter := &companionPoseCapture{NPC: pet.Painter.(*role.NPC)}
	pet.Painter = painter
	painter.FrameSize(11) // Start loading this sprite's metadata.
	c.Sprites.WaitNative()
	c.World.Player.Direction = 10
	// Populate the owner trail so the two sprites are separately visible.
	for i := 0; i < 5; i++ {
		c.World.Player.X += 20
		*now = now.Add(125 * time.Millisecond)
		c.World.Step(*now)
	}
	*now = now.Add(time.Second)
	c.World.Step(*now)
	// The owner faces left; Roca has just followed toward the right and keeps
	// her own right-facing sitting/lying poses independently of owner turns.
	sittingAction := 51
	if w, h := painter.FrameSize(sittingAction); w == 0 || h == 0 {
		// Updated client archives omit Roca's sitting records. An incapable
		// pet must remain standing, facing right, while its owner sits/turns.
		sittingAction = 13
		t.Log("Roca has no right-facing sitting animation; checking standing fallback")
	}
	for _, tc := range []struct{ cell, action, petAction int }{{13, 48, sittingAction}, {4, 26, 27}} {
		if w, h := painter.FrameSize(tc.petAction); w == 0 || h == 0 {
			var available []int
			for a := 0; a <= 73; a++ {
				if w, h := painter.FrameSize(a); w > 0 && h > 0 {
					available = append(available, a)
				}
			}
			t.Fatalf("Roca sprite %d lacks action %d; available %v", painter.Sprite(), tc.petAction, available)
		}
		if !c.playPose(tc.cell) {
			t.Fatal("pose blocked")
		}
		for frame := 0; frame < 120; frame++ {
			*now = now.Add(time.Second / 60)
			c.Frame()
		}
		if int(c.World.Player.Direction) != tc.action || painter.action != tc.petAction {
			t.Fatalf("player action %d pet action %d", c.World.Player.Direction, painter.action)
		}
		if dir := os.Getenv("SNAPSHOT_DIR"); dir != "" {
			save(t, c, filepath.Join(dir, map[int]string{48: "companion_sitting.png", 26: "companion_lying.png"}[tc.action]))
		}
		if tc.action == 48 {
			c.mapReady = true
			cx, cy := c.World.Camera()
			c.GroundRightClick(c.World.Player.X-cx, c.World.Player.Y-cy-80)
			*now = now.Add(time.Second / 60)
			c.Frame()
			if c.World.Player.Direction != 46 || painter.action != sittingAction {
				t.Fatalf("owner-only turn: player action %d pet action %d", c.World.Player.Direction, painter.action)
			}
		}
	}
	packets := sent()
	if len(packets) != 3 || !bytes.Equal(packets[0], []byte{32, 2, 48}) || !bytes.Equal(packets[1], []byte{32, 2, 46}) || !bytes.Equal(packets[2], []byte{32, 2, 26}) {
		t.Fatal("poses must use the ordinary owner AC32 requests", packets)
	}
	c.InventoryState.Pets[1] = inventory.UsePet{ID: 14162, Stats: world.Stats{HP: 1, MaxHP: 181, MaxSP: 95}}
	c.dispatch([]byte{8, 2, 4, 2, 0, 25, 1, 4, 0, 0, 0, 0, 0, 0, 0})
	c.dispatch([]byte{8, 2, 4, 2, 0, 26, 1, 1, 0, 0, 0, 0, 0, 0, 0})
	if stats := c.InventoryState.Pets[1].Stats; stats.HP != 4 || stats.SP != 1 {
		t.Fatal("native pet recovery reports not applied", stats)
	}
}
