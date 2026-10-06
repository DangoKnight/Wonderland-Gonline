package world

import "testing"

func TestNPCConversationFacing(t *testing.T) {
	for _, tc := range []struct{ x, y, want int }{{100, 90, 8}, {90, 90, 9}, {90, 100, 10}, {90, 110, 11}, {100, 110, 12}, {110, 110, 13}, {110, 100, 14}, {110, 90, 15}, {100, 100, 13}} {
		n := NPC{X: 100, Y: 100, Action: 13, Info: NPCTemplate{Kind: 2}}
		if !n.FaceConversation(tc.x, tc.y) || n.Action != tc.want {
			t.Fatalf("toward %d,%d: action %d, want %d", tc.x, tc.y, n.Action, tc.want)
		}
	}
	for _, tc := range []struct {
		kind   byte
		action int
	}{{6, 11}, {2, 16}, {2, 26}, {2, 46}} {
		n := NPC{X: 100, Y: 100, Action: tc.action, Info: NPCTemplate{Kind: tc.kind}}
		if n.FaceConversation(110, 100) || n.Action != tc.action {
			t.Fatalf("changed native exception kind %d action %d", tc.kind, tc.action)
		}
	}
	// Native code excludes exactly template kind 6, not every Prop() kind.
	n := NPC{Action: 8, Info: NPCTemplate{Kind: 9}}
	if !n.FaceConversation(10, 0) || n.Action != 14 {
		t.Fatal("blanket prop filter exceeded the native exception")
	}
}
