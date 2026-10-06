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
