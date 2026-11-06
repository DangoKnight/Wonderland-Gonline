package world

import "testing"

type conversationPainter struct {
	NPCPainter
	actions map[int]bool
}

func (p conversationPainter) FrameSize(action int) (int, int) {
	if p.actions[action] {
		return 40, 80
	}
	return 0, 0
}

func TestNPCConversationSparseFacings(t *testing.T) {
	painter := conversationPainter{actions: map[int]bool{9: true, 11: true, 13: true, 15: true}}
	for _, tc := range []struct{ x, y, want int }{{0, -10, 9}, {-10, -10, 9}, {-10, 0, 11}, {-10, 10, 11}, {0, 10, 13}, {10, 10, 13}, {10, 0, 15}, {10, -10, 15}} {
		n := NPC{Action: 11, Painter: painter}
		if !n.FaceConversation(tc.x, tc.y) || n.Action != tc.want {
			t.Fatalf("toward %d,%d: action %d, want %d", tc.x, tc.y, n.Action, tc.want)
		}
	}
	n := NPC{Action: 11, Painter: conversationPainter{}}
	if n.FaceConversation(10, 0) || n.Action != 11 {
		t.Fatal("missing standing artwork changed the original pose")
	}
	n.Painter = conversationPainter{actions: map[int]bool{11: true}}
	if n.FaceConversation(10, -10) || n.Action != 11 {
		t.Fatal("missing final facing selected another action group")
	}
}

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
