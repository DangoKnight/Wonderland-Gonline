package world

import "testing"

func TestGuidepostConversationUsesTemplateKind(t *testing.T) {
	for _, template := range []uint32{19020, 19021, 19022, 19023, 19024} {
		// Npc.dat marks every guidepost as kind 8.
		n := NPC{Action: 11, Template: template, Info: NPCTemplate{Kind: 8}}
		if n.FaceConversation(10, 0) || n.Action != 11 {
			t.Fatal("fixed template changed artwork", template)
		}
	}
	n := NPC{Action: 11, Info: NPCTemplate{Kind: 8}}
	if n.FaceConversation(10, 0) || n.Action != 11 {
		t.Fatal("guidepost turned")
	}
	for _, template := range []uint32{19019, 19020, 19024, 19025} {
		n := NPC{Action: 11, Template: template, Info: NPCTemplate{Kind: 2}}
		if !n.FaceConversation(10, 0) || n.Action != 14 {
			t.Fatal("template ID overrode directional actor metadata", template)
		}
	}
}
