package app

import (
	"testing"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/world"
)

func TestBoundXaolanNativeDialogueKeepsAuthoredFacing(t *testing.T) {
	const northIslandMap, boundXaolanActor, boundXaolanTemplate = 60001, 62, 14154
	c, _, _ := enteredClient(t)
	templates, err := world.NPCTemplates(c.Assets)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := world.MapRecord(c.Assets, northIslandMap)
	if err != nil {
		t.Fatal(err)
	}
	npcs := world.MapNPCs(rec, templates, func(info world.NPCTemplate) world.NPCPainter { return role.NewNPC(c.lib, info.Look, info.Colors) })
	n := npcs[boundXaolanActor]
	if n == nil || n.Template != boundXaolanTemplate {
		t.Fatal("bound Xaolan missing")
	}
	c.World.NPCs = npcs
	painter := n.Painter.(*role.NPC)
	painter.FrameSize(n.Action)
	c.Sprites.WaitNative()
	if w, h := painter.FrameSize(n.Action); w == 0 || h == 0 {
		t.Fatal("bound artwork unavailable")
	}
	authored := n.Action
	// Her tied sprite has limited artwork; choosing a different standing frame
	// must never override the native no-turn flag, regardless of player position.
	for _, pos := range [][2]int{{2622, 755}, {2500, 837}, {2586, 920}, {2700, 837}} {
		c.World.Player.X, c.World.Player.Y = pos[0], pos[1]
		c.dispatch([]byte{20, 1, 0, 0, 0, 1, 1, 3, 62, 0, 2, 0, 0, 0, 0, 110, 117, 0})
		if n.Action != authored {
			t.Fatal("tied Xaolan turned during native no-turn dialogue", n.Action, authored)
		}
		if c.World.Player.Direction != int32(conversationStandingAction+world.Facing(pos[0], pos[1], n.X, n.Y, 0)) {
			t.Fatal("player did not face Xaolan")
		}
		c.dispatch([]byte{20, 8})
		if n.Action != authored || n.Conversation {
			t.Fatal("dialogue close changed bound facing")
		}
	}
}
