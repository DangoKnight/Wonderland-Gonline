package world

import (
	"image"
	"testing"
	"time"
)

// Native FUN_004245bc samples at 20 pixels per axis; FUN_00424a0c targets
// the third entry. These expected positions are authored from those rules.
func TestCompanionNativeTrail(t *testing.T) {
	for _, peerOwner := range []bool{false, true} {
		t.Run(map[bool]string{false: "self", true: "peer"}[peerOwner], func(t *testing.T) {
			now := time.Unix(100, 0)
			w := &World{Player: Player{ID: 1, X: 100, Y: 100, Direction: standingAction + FaceRight}}
			owner, walker := &w.Player, &w.walker
			if peerOwner {
				peer := &Peer{Player: Player{ID: 2, X: 100, Y: 100, Direction: standingAction + FaceRight}}
				w.Peers = map[uint32]*Peer{2: peer}
				owner, walker = &peer.Player, &peer.Walker
			}
			w.SetCompanion(owner.ID, 123, []byte("Pet"), nil)
			walker.Start(owner, []image.Point{{X: 200, Y: 100}}, now)
			for step := 1; step <= 6; step++ {
				w.Step(now.Add(time.Duration(step) * 125 * time.Millisecond))
				x, y, action, ok := w.companionPosition(owner.ID)
				expected := []int{100, 100, 100, 120, 140, 160}[step-1]
				if !ok || x != expected || y != 100 {
					t.Fatalf("step %d: pet (%d,%d), want (%d,100)", step, x, y, expected)
				}
				if step >= 3 && step <= 5 && action != FaceRight {
					t.Fatalf("walking action %d", action)
				}
				if step == 6 && action != standingAction+FaceRight {
					t.Fatalf("standing action %d", action)
				}
			}
			// Stopping does not bring the pet alongside or onto the owner.
			w.Step(now.Add(2 * time.Second))
			x, y, _, _ := w.companionPosition(owner.ID)
			if x != 160 || y != 100 {
				t.Fatalf("pet lost its trailing position: %d,%d", x, y)
			}
		})
	}
}

func TestCompanionFollowsTurn(t *testing.T) {
	now := time.Unix(100, 0)
	w := &World{Player: Player{ID: 1, X: 100, Y: 100, Direction: standingAction + FaceRight}}
	w.SetCompanion(1, 123, nil, nil)
	w.walker.Start(&w.Player, []image.Point{{X: 200, Y: 100}, {X: 200, Y: 200}}, now)
	for step := 1; step <= 12; step++ {
		w.Step(now.Add(time.Duration(step) * 125 * time.Millisecond))
	}
	x, y, action, _ := w.companionPosition(1)
	if x != 200 || y != 160 || action != standingAction+FaceDown {
		t.Fatalf("pet did not follow around turn: %d,%d action=%d", x, y, action)
	}
}

func TestCompanionRelocationAndRefresh(t *testing.T) {
	now := time.Unix(100, 0)
	peer := &Peer{Player: Player{ID: 2, X: 100, Y: 100, Direction: standingAction + FaceRight}}
	w := &World{Player: Player{ID: 1}, Peers: map[uint32]*Peer{2: peer}}
	w.SetCompanion(2, 123, []byte("Old"), nil)
	peer.Walker.Start(&peer.Player, []image.Point{{X: 200, Y: 100}}, now)
	for step := 1; step <= 4; step++ {
		w.Step(now.Add(time.Duration(step) * 125 * time.Millisecond))
	}
	c := w.Companions[2]
	w.SetCompanion(2, 123, []byte("New"), nil)
	if w.Companions[2] != c || !c.walker.Walking() {
		t.Fatal("name refresh reset movement")
	}
	w.PlacePeer(2, 300, 300)
	if x, y, _, _ := w.companionPosition(2); x != 300 || y != 300 {
		t.Fatalf("relocation left stale trail: %d,%d", x, y)
	}
	w.Step(now.Add(time.Second))
	if c.walker.Walking() {
		t.Fatal("pet walked back after relocation")
	}
	peer.X, peer.Y = 2000, 2000
	w.Step(now.Add(2 * time.Second))
	if x, y, _, _ := w.companionPosition(2); x != 2000 || y != 2000 {
		t.Fatalf("discontinuity left stale trail: %d,%d", x, y)
	}
	w.SetCompanion(2, 124, nil, nil)
	if w.Companions[2] == c {
		t.Fatal("new pet retained previous state")
	}
	w.RemovePeer(2)
	if _, _, _, ok := w.companionPosition(2); ok {
		t.Fatal("departed companion still visible")
	}
}

type companionNamePainter struct {
	paintCount
	anchor int
}

func (p companionNamePainter) FirstAnchorY() (int, bool) { return p.anchor, true }

func TestCompanionNativeNameHeight(t *testing.T) {
	for _, tc := range []struct {
		info         NPCTemplate
		anchor, want int
	}{
		{NPCTemplate{}, 40, -76},
		{NPCTemplate{}, 80, -51},
		{NPCTemplate{HeightPreset: 1}, 40, -136},
		{NPCTemplate{HeightScale: 1}, 40, -142},
	} {
		c := Companion{Info: tc.info, Painter: companionNamePainter{anchor: tc.anchor}}
		if got := c.nameTop(); got != tc.want {
			t.Fatalf("anchor %d flags %d/%d: name top %d want %d", tc.anchor, tc.info.HeightScale, tc.info.HeightPreset, got, tc.want)
		}
	}
}
