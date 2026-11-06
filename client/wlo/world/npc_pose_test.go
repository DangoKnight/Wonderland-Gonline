package world

import "testing"

func TestKelanHouseFallenNPCPackets(t *testing.T) {
	// Map 12004 click 2: template 14021, (381,404), authored rotation 6.
	// Pre-event operation: 2,2,0,7,0,26,0,0,2,0,0,0,0,1,...
	n := &NPC{ClickID: 2, X: 381, Y: 404, Action: 11}
	w := &World{NPCs: map[uint16]*NPC{2: n}}
	w.ApplyActorPositions([]byte{2, 0, 255, 1, 125, 1, 148, 1, 1, 0, 0, 0, 0, 26})
	if n.Action != 26 || n.Fixed || !n.Shown {
		t.Fatalf("position packet did not apply fallen pose: %+v", n)
	}
	if !w.ApplyActorPose([]byte{2, 0, 26, 1}) || n.Action != 26 || !n.Fixed || n.Frame != 1 || n.Wrap {
		t.Fatalf("fallen frame not held: %+v", n)
	}
	// A visibility refresh and starting a conversation must not stand him up.
	w.ApplyActorPositions([]byte{2, 0, 255, 255, 125, 1, 148, 1, 1, 0, 0, 0, 0, 0})
	if n.FaceConversation(462, 435) || n.Action != 26 || !n.Fixed || n.Frame != 1 {
		t.Fatal("refresh or conversation reset fallen pose")
	}
}

func TestNPCPoseDirectionAndAnimationRelease(t *testing.T) {
	for _, tc := range []struct{ standing, fallen int }{{11, 26}, {13, 27}} {
		n := &NPC{Action: tc.standing}
		w := &World{NPCs: map[uint16]*NPC{1: n}}
		w.ApplyActorPose([]byte{1, 0, 26, 1})
		if n.Action != tc.fallen || !n.Fixed || n.Frame != 1 {
			t.Fatal(tc, n)
		}
		w.ApplyActorPose([]byte{1, 0, 26, 255})
		if n.Action != tc.fallen || n.Fixed {
			t.Fatal("free frame failed to resume animation", n)
		}
		if w.ApplyActorPose([]byte{1, 0, 26}) || !w.ApplyActorPose([]byte{99, 0, 26, 1}) {
			t.Fatal("malformed or unknown actor handling")
		}
	}
}

func TestKelanHousePoseAuthoredReference(t *testing.T) {
	needAssets(t)
	rec, err := MapRecord(assets, 12004)
	if err != nil {
		t.Fatal(err)
	}
	n := MapNPCs(rec, nil, nil)[2]
	if n == nil || n.Template != 14021 || n.X != 381 || n.Y != 404 || n.Action != 11 {
		t.Fatalf("unexpected house actor: %+v", n)
	}
}
