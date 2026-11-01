package world

import (
	"image"
	"testing"
	"time"
)

func routeNPC() (*World, *NPC) {
	n := &NPC{ClickID: 1, X: 100, Y: 100, Action: 8, Shown: true, speed: 2,
		Routes: map[byte][]image.Point{1: {{200, 100}, {200, 300}}, 2: {{50, 50}}}}
	n.walker.pixelsPerMillisecond = actorSpeed(n.speed)
	return &World{NPCs: map[uint16]*NPC{1: n}}, n
}

// TestRouteWalk: kind 4 mode 1 walks the route's points in turn at speed
// byte 10 (0.2 px/ms) and holds the step until the last point, where the
// record speed returns.
func TestRouteWalk(t *testing.T) {
	now := time.Unix(0, 0)
	w, n := routeNPC()
	w.WalkRoute(1, 1, RouteWalk, now)
	if w.RoutesDone() || n.Action != walkingAction+FaceRight {
		t.Fatalf("not walking: %+v", n)
	}
	w.Step(now.Add(time.Second)) // 200 px: the first point, then 100 down
	if n.X != 200 || n.Y != 200 || w.RoutesDone() {
		t.Fatalf("at (%d, %d)", n.X, n.Y)
	}
	w.Step(now.Add(2 * time.Second))
	if n.X != 200 || n.Y != 300 || !w.RoutesDone() || n.route.queued {
		t.Fatalf("at (%d, %d), queued %v", n.X, n.Y, n.route.queued)
	}
	if n.walker.pixelsPerMillisecond != actorSpeed(2) || !n.Shown {
		t.Fatal("arrival did not restore the record speed")
	}
}

// TestRouteModes: mode 2 stands the NPC on the first point; mode 3 hides
// it on arrival; unknown NPCs, routes and modes do nothing.
func TestRouteModes(t *testing.T) {
	now := time.Unix(0, 0)
	w, n := routeNPC()
	w.WalkRoute(1, 2, RoutePlace, now)
	if n.X != 50 || n.Y != 50 || n.walker.Walking() || !w.RoutesDone() {
		t.Fatalf("placed at (%d, %d)", n.X, n.Y)
	}
	w.WalkRoute(1, 2, RouteWalkHide, now)
	w.Step(now.Add(time.Second))
	if w.RoutesDone(); n.Shown {
		t.Fatal("mode 3 left the NPC shown")
	}
	for _, c := range [][3]int{{9, 1, 1}, {1, 7, 1}, {1, 1, 4}} {
		w.WalkRoute(uint16(c[0]), byte(c[1]), byte(c[2]), now)
	}
	if !w.RoutesDone() || n.walker.Walking() {
		t.Fatal("an invalid route queued a walk")
	}
}
