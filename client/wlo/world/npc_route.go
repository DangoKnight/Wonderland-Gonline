package world

import (
	"image"
	"time"

	native "wonderland-gonline/internal/assets"
)

// Event routes. Event kind 4 (FUN_00304fd0 case 4) sends a map NPC along
// one of its record's routes (native.MapNPC.Routes). The interpreter queues
// it (FUN_0030a404: the NPC, its record speed, the route and the mode, at
// most ten), and the NPC's frame (FUN_00418304) takes the route's next
// point each time it reaches one. The event's frame loop (FUN_00307300)
// holds the step's acknowledgement until every queued NPC stands on its
// route's last point; there it gets its record speed back, a mode-3 walker
// disappears (FUN_004187d8 with 0x3e7fc18), and it leaves the queue.
const (
	RouteWalk     = 1 // walk from where it stands
	RoutePlace    = 2 // stand on the first point, then walk the rest
	RouteWalkHide = 3 // walk, then disappear
	// routeSpeed is the speed byte a route walk uses (FUN_00482980(10)).
	routeSpeed = 10
	// routeQueue is FUN_0030a404's table size.
	routeQueue = 10
)

// npcRoute is an NPC's place in the route queue.
type npcRoute struct {
	queued bool
	mode   byte
	last   image.Point
}

// actorSpeed is FUN_00482980: a speed byte in eighths of the walk speed.
func actorSpeed(b byte) float64 { return float64(b) / actorSpeedDivisor * walkSpeed }

// routePoints converts a record's route.
func routePoints(r map[byte][]native.RoutePoint) map[byte][]image.Point {
	if len(r) == 0 {
		return nil
	}
	out := make(map[byte][]image.Point, len(r))
	for slot, pts := range r {
		for _, p := range pts {
			out[slot] = append(out[slot], image.Pt(int(int32(p.X)), int(int32(p.Y))))
		}
	}
	return out
}

// WalkRoute is event kind 4 for a map NPC: route slot by mode. An unknown
// NPC, route or mode does nothing, and the step is done at once.
func (w *World) WalkRoute(click uint16, slot, mode byte, now time.Time) {
	n := w.NPCs[click]
	if n == nil {
		return
	}
	pts := n.Routes[slot]
	if len(pts) == 0 {
		return
	}
	n.Conversation = false // Explicit event choreography overrides ambient pause.
	switch mode {
	case RouteWalk, RouteWalkHide:
		n.stepWalk(now)
		n.walker.pixelsPerMillisecond = actorSpeed(routeSpeed)
		w.startRoute(n, pts, now)
	case RoutePlace:
		n.stopWalk()
		n.X, n.Y = pts[0].X, pts[0].Y
		if len(pts) > 1 {
			w.startRoute(n, pts[1:], now)
		}
	default:
		return
	}
	if n.route.queued || w.routesQueued() < routeQueue {
		n.route = npcRoute{queued: true, mode: mode, last: pts[len(pts)-1]}
	}
}

func (w *World) startRoute(n *NPC, pts []image.Point, now time.Time) {
	pos := Player{X: n.X, Y: n.Y, Direction: int32(n.Action)}
	n.walker.Start(&pos, append([]image.Point(nil), pts...), now)
	n.Action = int(pos.Direction)
}

func (w *World) routesQueued() int {
	k := 0
	for _, n := range w.NPCs {
		if n.route.queued {
			k++
		}
	}
	return k
}

// RoutesDone is FUN_00307300's route check: arrivals leave the queue, and
// it reports whether the queue is empty.
func (w *World) RoutesDone() bool {
	done := true
	for _, n := range w.NPCs {
		if !n.route.queued {
			continue
		}
		if image.Pt(n.X, n.Y) != n.route.last {
			done = false
			continue
		}
		n.walker.pixelsPerMillisecond = actorSpeed(n.speed)
		if n.route.mode == RouteWalkHide {
			n.Shown = false
		}
		n.route = npcRoute{}
	}
	return done
}
