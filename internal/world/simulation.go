package world

import (
	"math"
	"math/rand/v2"
	"strings"
	"sync"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
)

// QuestNpc.Update compatibility values. Timers and patrol cursors are transient;
// immutable paths and collision grids come from the SQL startup catalog.
const (
	actorStationaryDelay      = 300 * time.Second
	actorContainerTemplateMin = 12000
	actorContainerTemplateMax = 12999
	actorSceneryTemplateMin   = 19000
	actorSceneryTemplateMax   = 19999
	actorMechanismTemplateMin = 25000
	actorMechanismTemplateMax = 35000
)

const (
	walkStatic          = 1
	walkPatrol          = 2
	walkBounds          = 3
	walkRoam            = 4
	walkPatrolAlternate = 5
	actorWalkingSpeed   = 2
	// Native AC22:2: FUN_0041869c calls FUN_00482980(speed, 0.16),
	// which returns speed/8*0.16 pixels/ms. FUN_004126b8 uses elapsed ms.
	actorNativeBasePixelsPerMillisecond = 0.16
	actorNativeSpeedDivisor             = 8
	actorCoordinateMinimum              = 50
	actorCoordinateMaximum              = 4000
	actorWaypointLimit                  = 10000
	actorBoundsLimit                    = 300
	actorSpawnLeash                     = 150
	actorInitialDelayMin                = time.Second
	actorInitialDelayRange              = 7 * time.Second
	actorRespawnDelay                   = 3 * time.Second
	actorMovementDelayMin               = time.Second
	actorMovementDelayRange             = 2*time.Second + time.Millisecond
	actorMovementCandidates             = 4
	actorPursuitStep                    = 100
)

type actorState struct {
	x, y         uint16
	step         int
	next         time.Time
	arrives      time.Time
	departed     time.Time
	fromX, fromY uint16
	pursuing     bool
	pursuitRetry time.Time
}

func (state *actorState) positionAt(now time.Time) (uint16, uint16) {
	if state.arrives.IsZero() || !now.Before(state.arrives) {
		return state.x, state.y
	}
	if !now.After(state.departed) {
		return state.fromX, state.fromY
	}
	progress := float64(now.Sub(state.departed)) / float64(state.arrives.Sub(state.departed))
	return uint16(math.Round(float64(state.fromX) + float64(int(state.x)-int(state.fromX))*progress)),
		uint16(math.Round(float64(state.fromY) + float64(int(state.y)-int(state.fromY))*progress))
}

type actors struct {
	mu     sync.Mutex
	states map[uint16]map[uint16]*actorState
}
type ActorMove struct{ Click, X, Y uint16 }

func (m ActorMove) Packet() []byte {
	return protocol.Builder{protocol.CommandScene, protocol.SceneActorWalk}.U16(m.Click).U16(m.X).U16(m.Y).U8(actorWalkingSpeed)
}

// NPC reports the current simulation position while Map.NPC retains the spawn.
func (w *World) NPC(mapID, click uint16) (NPC, bool) {
	return w.NPCAt(mapID, click, time.Now())
}

// NPCAt estimates the current position from the issued walking leg.
func (w *World) NPCAt(mapID, click uint16, now time.Time) (NPC, bool) {
	m, ok := w.Map(mapID)
	if !ok {
		return NPC{}, false
	}
	n, ok := m.NPC(click)
	if !ok {
		return n, false
	}
	w.actors.mu.Lock()
	defer w.actors.mu.Unlock()
	if state := w.actors.states[mapID][click]; state != nil {
		n.X, n.Y = state.positionAt(now)
	}
	return n, true
}
func (w *World) NPCs(mapID uint16) []NPC {
	return w.NPCsAt(mapID, time.Now())
}

// NPCsAt gives the simulation tick a consistent view of moving actors.
func (w *World) NPCsAt(mapID uint16, now time.Time) []NPC {
	m, ok := w.Map(mapID)
	if !ok {
		return nil
	}
	out := append([]NPC(nil), m.NPCs...)
	w.actors.mu.Lock()
	defer w.actors.mu.Unlock()
	for i, n := range out {
		if state := w.actors.states[mapID][n.ClickID]; state != nil {
			out[i].X, out[i].Y = state.positionAt(now)
		}
	}
	return out
}

// Contains validates scene bounds independently of the land collision mask.
func (w *World) Contains(mapID, x, y uint16) bool {
	t, ok := w.catalog.Terrains[mapID]
	return !ok || t.Contains(int(x), int(y))
}

// CanMove validates a client-announced straight leg, rather than imposing a
// distance/speed cap: native clients announce the destination before walking.
// Missing terrain preserves legacy maps; populated grids never allow bounds
// escapes. A blocked spawn may leave its first cell to recover old saved state.
func (w *World) CanMove(mapID, fromX, fromY, toX, toY uint16) bool {
	t, ok := w.catalog.Terrains[mapID]
	if !ok {
		return true
	}
	if !t.Walkable(int(toX), int(toY)) {
		return false
	}
	if !t.Contains(int(fromX), int(fromY)) {
		return true
	}
	ax, ay := int(fromX)/assets.TerrainCellSize, int(fromY)/assets.TerrainCellSize
	bx, by := int(toX)/assets.TerrainCellSize, int(toY)/assets.TerrainCellSize
	span := max(abs(bx-ax), abs(by-ay))
	for i := 1; i <= span; i++ {
		cx, cy := ax+(bx-ax)*i/span, ay+(by-ay)*i/span
		if !t.Walkable(cx*assets.TerrainCellSize, cy*assets.TerrainCellSize) {
			return false
		}
	}
	return true
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (w *World) staticActor(mapID uint16, n NPC) bool {
	if n.Template == 0 {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(n.Name))
	if name == "" {
		name = strings.ToLower(w.templateName(n.Template))
	}
	if w.Wild(mapID, n) {
		return false
	}
	for _, word := range []string{"villager", "citizen", "resident", "grandma", "elder", "mayor", "guard", "soldier", "knight", "merchant", "sailor", "captain", "maid", "girl", "boy", "man", "woman", "robinson", "burke", "peter", "john", "natasha", "breillat"} {
		if strings.Contains(name, word) {
			return false
		}
	}
	if n.Template >= actorContainerTemplateMin && n.Template <= actorContainerTemplateMax || n.Template >= actorSceneryTemplateMin && n.Template <= actorSceneryTemplateMax || n.Template >= actorMechanismTemplateMin && n.Template <= actorMechanismTemplateMax {
		return true
	}
	for _, word := range []string{"chest", "box", "crate", "barrel", "pot", "machine", "wood", "stone", "clay", "mine", "herb", "tree", "door", "switch", "lever", "cabinet", "desk", "bed", "chair", "stove", "grass", "flower", "shell", "mushroom", "ore", "statue", "fountain", "sign", "well", "grave", "cart", "boat", "wreck", "tent", "fence", "portal", "warp", "prop", "object", "coconut", "driftwood", "bamboo"} {
		if strings.Contains(name, word) {
			return true
		}
	}
	return name == "" || strings.HasPrefix(name, "npc_0") || strings.HasPrefix(name, "unknown") || strings.HasPrefix(name, "·s")
}

// RoamingTown is QuestNpc.IsVillageOrTownMap, distinct from encounter SafeTown.
func RoamingTown(id uint16) bool {
	return id == 10000 || id == 60001 || id >= 10001 && id <= 10036 || id >= 12001 && id <= 12030 || id >= 14000 && id <= 14030 || id >= 16000 && id <= 16030 || id >= 18000 && id <= 18030
}

// ActorPosition is a transient destination supplied by the encounter owner.
type ActorPosition struct{ X, Y uint16 }

// ActorBounds retains the authored spawn-relative wandering rectangle.
type ActorBounds struct{ MinX, MinY, MaxX, MaxY uint16 }

func (b ActorBounds) Contains(x, y uint16) bool {
	return x >= b.MinX && x <= b.MaxX && y >= b.MinY && y <= b.MaxY
}

func boundedArea(n NPC, def assets.MapNPC) (ActorBounds, bool) {
	if def.WalkBehavior != walkBounds || len(def.WalkSteps) < 2 {
		return ActorBounds{}, false
	}
	lo, hi := def.WalkSteps[0], def.WalkSteps[1]
	coordinate := func(spawn uint16, offset int) uint16 {
		return uint16(max(actorCoordinateMinimum, min(actorCoordinateMaximum, int(spawn)+offset)))
	}
	return ActorBounds{
		coordinate(n.X, max(-actorBoundsLimit, min(0, int(int32(lo.X))))),
		coordinate(n.Y, max(-actorBoundsLimit, min(0, int(int32(lo.Y))))),
		coordinate(n.X, max(0, min(actorBoundsLimit, int(int32(hi.X))))),
		coordinate(n.Y, max(0, min(actorBoundsLimit, int(int32(hi.Y))))),
	}, true
}

// actorTargetArea shares movement and acquisition boundaries. Patrollers with
// authored paths can pursue nearby players throughout the scene; pathless patrols
// use the same fixed spawn leash as random roamers.
func actorTargetArea(mapID uint16, n NPC, def assets.MapNPC) (ActorBounds, bool) {
	if bounds, ok := boundedArea(n, def); ok {
		return bounds, true
	}
	switch def.WalkBehavior {
	case walkPatrol, walkPatrolAlternate:
		if len(def.WalkSteps) > 0 {
			return ActorBounds{MaxX: ^uint16(0), MaxY: ^uint16(0)}, true
		}
	case walkRoam:
	default:
		return ActorBounds{}, false
	}
	if RoamingTown(mapID) {
		return ActorBounds{}, false
	}
	return roamArea(n), true
}

func roamArea(n NPC) ActorBounds {
	coordinate := func(spawn uint16, offset int) uint16 {
		return uint16(max(actorCoordinateMinimum, min(actorCoordinateMaximum, int(spawn)+offset)))
	}
	return ActorBounds{coordinate(n.X, -actorSpawnLeash), coordinate(n.Y, -actorSpawnLeash), coordinate(n.X, actorSpawnLeash), coordinate(n.Y, actorSpawnLeash)}
}

// ActorTargetArea is fixed to spawn definitions, independent of moving positions.
func (w *World) ActorTargetArea(mapID, click uint16) (ActorBounds, bool) {
	m, ok := w.Map(mapID)
	if !ok {
		return ActorBounds{}, false
	}
	n, ok := m.NPC(click)
	if !ok {
		return ActorBounds{}, false
	}
	for _, def := range m.data.NPCs {
		if def.ClickID == click {
			return actorTargetArea(mapID, n, def)
		}
	}
	return ActorBounds{}, false
}

func (w *World) wanderPosition(mapID uint16, state *actorState, bounds ActorBounds, random func(int) int) (int, int) {
	x, y, best := int(state.x), int(state.y), 0
	for range actorMovementCandidates {
		cx := int(bounds.MinX) + random(int(bounds.MaxX-bounds.MinX)+1)
		cy := int(bounds.MinY) + random(int(bounds.MaxY-bounds.MinY)+1)
		dx, dy := cx-int(state.x), cy-int(state.y)
		if distance := dx*dx + dy*dy; distance > best && w.CanMove(mapID, state.x, state.y, uint16(cx), uint16(cy)) {
			x, y, best = cx, cy, distance
		}
	}
	return x, y
}

func (w *World) pursuitPosition(mapID uint16, state *actorState, target ActorPosition) (int, int) {
	dx, dy := float64(int(target.X)-int(state.x)), float64(int(target.Y)-int(state.y))
	scale := min(1.0, actorPursuitStep/max(1.0, math.Hypot(dx, dy)))
	x, y := int(state.x)+int(dx*scale), int(state.y)+int(dy*scale)
	if !w.CanMove(mapID, state.x, state.y, uint16(x), uint16(y)) {
		if x != int(state.x) && w.CanMove(mapID, state.x, state.y, uint16(x), state.y) {
			y = int(state.y)
		} else if y != int(state.y) && w.CanMove(mapID, state.x, state.y, state.x, uint16(y)) {
			x = int(state.x)
		}
	}
	return x, y
}

// ActorMoving reports a walking leg whose estimated native arrival is pending.
// The native protocol supplies no NPC arrival acknowledgement.
func (w *World) ActorMoving(mapID, click uint16, now time.Time) bool {
	w.actors.mu.Lock()
	defer w.actors.mu.Unlock()
	state := w.actors.states[mapID][click]
	return state != nil && now.Before(state.arrives)
}

func actorTravelTime(fromX, fromY, toX, toY uint16) time.Duration {
	distance := math.Hypot(float64(int(toX)-int(fromX)), float64(int(toY)-int(fromY)))
	pixelsPerMillisecond := float64(actorWalkingSpeed) / actorNativeSpeedDivisor * actorNativeBasePixelsPerMillisecond
	return time.Duration(math.Ceil(distance/pixelsPerMillisecond)) * time.Millisecond
}

// AdvanceActors advances one occupied map at most once per actor per tick.
// paused reserves actors held by battles or scripts. random is an injectable
// half-open bounded draw for independent deterministic compatibility tests.
func (w *World) AdvanceActors(mapID uint16, now time.Time, paused map[uint16]bool, random func(int) int) []ActorMove {
	return w.AdvanceActorsToward(mapID, now, paused, nil, random)
}

// AdvanceActorsToward follows eligible players selected by the server. Pursuit
// respects each movement type's target area and the wandering collision checks.
func (w *World) AdvanceActorsToward(mapID uint16, now time.Time, paused map[uint16]bool, targets map[uint16]ActorPosition, random func(int) int) []ActorMove {
	if random == nil {
		random = rand.IntN
	}
	m, ok := w.maps[mapID]
	if !ok {
		return nil
	}
	w.actors.mu.Lock()
	defer w.actors.mu.Unlock()
	if w.actors.states[mapID] == nil {
		w.actors.states[mapID] = map[uint16]*actorState{}
	}
	var moves []ActorMove
	for _, n := range m.NPCs {
		var def assets.MapNPC
		for _, d := range m.data.NPCs {
			if d.ClickID == n.ClickID {
				def = d
				break
			}
		}
		if def.WalkBehavior == walkStatic || w.staticActor(mapID, n) {
			continue
		}
		state := w.actors.states[mapID][n.ClickID]
		if state == nil {
			initialRange := actorInitialDelayRange
			if def.WalkBehavior >= walkPatrol && def.WalkBehavior <= walkPatrolAlternate {
				initialRange = actorMovementDelayRange
			}
			state = &actorState{x: n.X, y: n.Y, next: now.Add(actorInitialDelayMin + time.Duration(random(int(initialRange/time.Millisecond)))*time.Millisecond)}
			w.actors.states[mapID][n.ClickID] = state
		}
		if paused[n.ClickID] || w.Defeated(mapID, n.ClickID) {
			continue
		}
		area, mayPursue := actorTargetArea(mapID, n, def)
		target, hasTarget := targets[n.ClickID]
		chasing := hasTarget && mayPursue && area.Contains(target.X, target.Y) && w.Wild(mapID, n)
		interrupt := chasing && !state.pursuing && !now.Before(state.pursuitRetry)
		if !interrupt && (now.Before(state.next) || now.Before(state.arrives)) {
			continue
		}
		fromX, fromY := state.positionAt(now)
		origin := &actorState{x: fromX, y: fromY}
		x, y := int(n.X), int(n.Y)
		delay := actorMovementDelayMin + time.Duration(random(int(actorMovementDelayRange/time.Millisecond)))*time.Millisecond
		switch {
		case chasing:
			x, y = w.pursuitPosition(mapID, origin, target)
		case def.WalkBehavior == walkBounds && len(def.WalkSteps) >= 2:
			bounds, _ := boundedArea(n, def)
			x, y = w.wanderPosition(mapID, origin, bounds, random)
		case (def.WalkBehavior == walkPatrol || def.WalkBehavior == walkPatrolAlternate) && len(def.WalkSteps) > 0:
			path := def.WalkSteps
			step := path[state.step%len(path)]
			use := len(path) > 1 || state.step%2 == 0
			if use && step.X > 0 && step.X < actorWaypointLimit && step.Y > 0 && step.Y < actorWaypointLimit {
				x, y = int(step.X), int(step.Y)
			}
			if len(path) == 1 {
				state.step = (state.step + 1) % 2
			} else {
				state.step = (state.step + 1) % len(path)
			}
		case (w.Wild(mapID, n) || def.WalkBehavior == walkRoam) && !RoamingTown(mapID):
			x, y = w.wanderPosition(mapID, origin, roamArea(n), random)
		default:
			state.next = now.Add(actorStationaryDelay)
			continue
		}
		state.next = now.Add(delay)
		if chasing {
			state.pursuitRetry = now.Add(delay)
		}
		if !w.CanMove(mapID, fromX, fromY, uint16(x), uint16(y)) || x == int(fromX) && y == int(fromY) && !(interrupt && now.Before(state.arrives)) {
			continue
		}
		state.fromX, state.fromY, state.departed = fromX, fromY, now
		state.arrives = now.Add(actorTravelTime(fromX, fromY, uint16(x), uint16(y)))
		state.next = state.arrives.Add(delay)
		state.pursuing = chasing
		state.x, state.y = uint16(x), uint16(y)
		moves = append(moves, ActorMove{n.ClickID, state.x, state.y})
	}
	return moves
}

func (w *World) resetActor(mapID, click uint16, now time.Time) {
	w.actors.mu.Lock()
	defer w.actors.mu.Unlock()
	if state := w.actors.states[mapID][click]; state != nil {
		if m, ok := w.Map(mapID); ok {
			if n, ok := m.NPC(click); ok {
				state.x, state.y = n.X, n.Y
				state.step = 0
				state.arrives = time.Time{}
				state.departed = time.Time{}
				state.pursuing = false
				state.pursuitRetry = time.Time{}
				state.next = now.Add(actorRespawnDelay)
			}
		}
	}
}

// GatheringProp is the reference static-prop fallback, after authored EVE
// interactions and services. NPCs with linked scripts retain their own lifecycle.
func (w *World) GatheringProp(mapID uint16, n NPC) bool {
	return n.Template != 0 && !w.hasLinkedEvent(mapID, n.ClickID) && w.staticActor(mapID, n)
}
