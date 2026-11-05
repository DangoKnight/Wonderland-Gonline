package app

import (
	"testing"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

func TestConversationFrameTurningAndRestoration(t *testing.T) {
	c, _, _ := enteredClient(t)
	n := &world.NPC{ClickID: 2, X: 100, Y: 100, Action: 9, Info: world.NPCTemplate{Kind: 2}}
	c.World.NPCs[2] = n
	c.World.Player.X, c.World.Player.Y = 120, 100
	p := talkFrame(2, 10002)
	p[10] = 2 // Native talk mode 2 preserves the NPC direction.
	c.dispatch(p)
	if n.Action != 9 || c.World.Player.Direction != 10 {
		t.Fatal("non-turning talk changed NPC or did not face the speaker")
	}
	p[10] = 1
	c.dispatch(p)
	if n.Action != 14 {
		t.Fatal("talk did not face NPC toward the player")
	}
	c.World.Player.X = 80
	c.dispatch(p)
	if n.Action != 10 {
		t.Fatal("second talk did not reorient the NPC")
	}
	c.dispatch([]byte{22, 7, 2, 0, 2}) // An expression must retain dialogue restoration.
	c.dispatch([]byte{20, 8})
	if n.Action != 9 {
		t.Fatal("event end did not restore the original facing")
	}
	c.World.Questions[1] = world.Question{}
	c.dispatch(questionFrame(2, 1))
	if n.Action != 10 {
		t.Fatal("question prompt did not turn the NPC")
	}
	c.dispatch([]byte{protocol.CommandEvent, protocol.EventResume})
	if n.Action != 9 {
		t.Fatal("question end did not restore facing")
	}
	n.Info.Kind = 6
	c.dispatch(p)
	if n.Action != 9 {
		t.Fatal("type-6 prop turned")
	}
	c.dispatch([]byte{20, 8})
	n.Info.Kind = 2
	n.Action = 16
	c.dispatch(p)
	if n.Action != 16 {
		t.Fatal("special NPC pose changed")
	}
	c.dispatch([]byte{20, 8})
}

func TestConversationScriptedActorChangesSupersedeRestoration(t *testing.T) {
	for _, laterTalk := range []bool{false, true} {
		c, _, _ := enteredClient(t)
		// Captured honey hand-in: the patient starts held in lying pose 26,
		// speaks in mode 2, then receives AC22:8 animation 8 (standing).
		n := &world.NPC{ClickID: 2, X: 100, Y: 100, Action: 26, Fixed: true, Frame: 2, Info: world.NPCTemplate{Kind: 2}}
		c.World.NPCs[2] = n
		c.World.Player.X, c.World.Player.Y = 120, 100
		line := talkFrame(2, 11083)
		line[10] = 2
		c.dispatch(line)
		c.dispatch([]byte{22, 8, 2, 0})
		if n.Action != 26 {
			t.Fatal("malformed animation changed the patient")
		}
		if laterTalk {
			c.dispatch([]byte{22, 8, 2, 0, 8}) // Captured Legacy layout.
		} else {
			c.dispatch([]byte{22, 8, 2, 0, 8, 0}) // Gonline compatibility byte.
		}
		if n.Action != 8 || n.Fixed || n.Conversation {
			t.Fatal("standing animation did not release the old conversation pose", n)
		}
		if laterTalk {
			line[10] = 1
			c.dispatch(line)
			if n.Action != 14 {
				t.Fatal("later dialogue did not turn the standing patient", n.Action)
			}
		}
		c.dispatch([]byte{20, 8})
		if n.Action != 8 || n.Fixed || n.Conversation {
			t.Fatal("event end restored the old lying pose", laterTalk, n)
		}
	}
}

func TestConversationPreservesScriptedPoseAndTurn(t *testing.T) {
	for _, packet := range [][]byte{
		{22, 9, 2, 0, 26, 1},
		eventFrameOf(eventKindNPCTurn, 2, 10, npcTurnDirection, 5),
	} {
		c, _, _ := enteredClient(t)
		n := &world.NPC{ClickID: 2, X: 100, Y: 100, Action: 9, Info: world.NPCTemplate{Kind: 2}}
		c.World.NPCs[2] = n
		c.World.Player.X, c.World.Player.Y = 120, 100
		line := talkFrame(2, 11083)
		line[10] = 1
		c.dispatch(line)
		c.dispatch(packet)
		want := n.Action
		c.dispatch([]byte{20, 8})
		if n.Action != want || n.Conversation {
			t.Fatal("event end replaced the scripted actor state", packet, n)
		}
	}
}
