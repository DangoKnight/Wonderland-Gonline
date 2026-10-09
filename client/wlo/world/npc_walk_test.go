package world

import (
	"testing"
	"time"
)

func TestActorWalkNativeSpeedAndInterruption(t *testing.T) {
	now := time.Unix(0, 0)
	n := &NPC{ClickID: 1, X: 100, Y: 100, Action: 8, Shown: true}
	w := &World{NPCs: map[uint16]*NPC{1: n}}
	// Independent AC22:2 payload: click 1, destination (200,100), speed 2.
	if !w.ApplyActorWalk([]byte{1, 0, 200, 0, 100, 0, 2}, now) || n.X != 100 || n.Action != FaceRight {
		t.Fatal("walk teleported or used wrong facing", n)
	}
	w.Step(now.Add(time.Second))
	if n.X != 140 || n.Y != 100 || n.SortY() != 100 {
		t.Fatal("native speed 2 must move 40 px/s", n)
	}
	// Interrupt between rendered frames; speed zero retains the preceding 2.
	w.ApplyActorWalk([]byte{1, 0, 160, 0, 200, 0, 0}, now.Add(1500*time.Millisecond))
	if n.X != 160 || n.Y != 100 || n.Action != FaceDown {
		t.Fatal("interruption jumped to old destination", n)
	}
	w.Step(now.Add(2500 * time.Millisecond))
	if n.X != 160 || n.Y != 140 {
		t.Fatal("zero speed did not preserve native speed", n)
	}
	w.Step(now.Add(10 * time.Second))
	if n.X != 160 || n.Y != 200 || n.Action != 12 || n.walker.Walking() {
		t.Fatal("arrival failed to stand at endpoint", n)
	}
	w.Step(now.Add(20 * time.Second))
	if n.X != 160 || n.Y != 200 {
		t.Fatal("arrived actor kept moving", n)
	}
}

func TestActorWalkDiagonalAndPositionReset(t *testing.T) {
	now := time.Unix(0, 0)
	n := &NPC{ClickID: 1, X: 100, Y: 100, Action: 8, Shown: true}
	w := &World{NPCs: map[uint16]*NPC{1: n}}
	w.ApplyActorWalk([]byte{1, 0, 220, 0, 4, 1, 4}, now) // (220,260), 80 px/s.
	w.Step(now.Add(time.Second))
	if n.X != 148 || n.Y != 164 {
		t.Fatal("diagonal speed was applied per axis", n)
	}
	// AC22:4 hides/reserves the actor; the old leg must not survive respawn.
	w.ApplyActorPositions([]byte{1, 0, 0, 0, 44, 1, 144, 1, 2, 0, 0, 0, 0, 0})
	w.Step(now.Add(time.Minute))
	if n.X != 300 || n.Y != 400 || n.Shown || n.walker.Walking() {
		t.Fatal("position update retained stale movement", n)
	}
	w.ApplyActorPositions([]byte{1, 0, 0, 0, 100, 0, 100, 0, 1, 0, 0, 0, 0, 0})
	w.Step(now.Add(2 * time.Minute))
	if n.X != 100 || n.Y != 100 || !n.Shown {
		t.Fatal("respawn reused old destination", n)
	}
}

func TestActorWalkMalformedAndSessionOwnership(t *testing.T) {
	now := time.Unix(0, 0)
	n := &NPC{ClickID: 1, X: 100, Y: 100, Action: 8}
	w := &World{NPCs: map[uint16]*NPC{1: n}}
	other := &World{NPCs: map[uint16]*NPC{1: {ClickID: 1, X: 100, Y: 100, Action: 8}}}
	p := []byte{1, 0, 200, 0, 100, 0, 2}
	for size := 0; size < len(p); size++ {
		if w.ApplyActorWalk(p[:size], now) || n.walker.Walking() || n.X != 100 {
			t.Fatal("truncated movement changed actor", size)
		}
	}
	if w.ApplyActorWalk(append(p, 0), now) || !w.ApplyActorWalk([]byte{255, 255, 200, 0, 100, 0, 2}, now) {
		t.Fatal("bad length or unknown actor handling")
	}
	w.ApplyActorWalk(p, now)
	w.Step(now.Add(time.Second))
	other.Step(now.Add(time.Second))
	if other.NPCs[1].X != 100 || other.NPCs[1].walker.Walking() {
		t.Fatal("movement leaked into another session")
	}
}
