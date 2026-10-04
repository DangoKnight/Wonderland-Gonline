package world

import (
	"math/rand/v2"
	"strings"
	"sync"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/protocol"
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
	walkStatic             = 1
	walkPatrol             = 2
	walkBounds             = 3
	walkRoam               = 4
	walkPatrolAlternate    = 5
	actorWalkingSpeed      = 2
	actorCoordinateMinimum = 50
	actorCoordinateMaximum = 4000
	actorWaypointLimit     = 10000
	actorBoundsLimit       = 300
	actorRoamStep          = 40
	actorSpawnLeash        = 60
	actorReturnRadius      = 20
	actorInitialDelayMin   = time.Second
	actorInitialDelayRange = 7 * time.Second
	actorRespawnDelay      = 3 * time.Second
	actorPatrolDelayMin    = 3500 * time.Millisecond
	actorPatrolDelayRange  = 4 * time.Second
	actorBoundsDelayMin    = 4 * time.Second
	actorBoundsDelayRange  = 5 * time.Second
	actorRoamDelayMin      = 5 * time.Second
	actorRoamDelayRange    = 5 * time.Second
	actorMinimumPathDelay  = 2 * time.Second
)

type actorState struct {
	x, y uint16
	step int
	next time.Time
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
		n.X, n.Y = state.x, state.y
	}
	return n, true
}
func (w *World) NPCs(mapID uint16) []NPC {
	m, ok := w.Map(mapID)
	if !ok {
		return nil
	}
	out := append([]NPC(nil), m.NPCs...)
	w.actors.mu.Lock()
	defer w.actors.mu.Unlock()
	for i, n := range out {
		if state := w.actors.states[mapID][n.ClickID]; state != nil {
			out[i].X, out[i].Y = state.x, state.y
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

// AdvanceActors advances one occupied map at most once per actor per tick.
// paused reserves actors held by battles or scripts. random is an injectable
// half-open bounded draw for independent deterministic compatibility tests.
func (w *World) AdvanceActors(mapID uint16, now time.Time, paused map[uint16]bool, random func(int) int) []ActorMove {
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
			state = &actorState{x: n.X, y: n.Y, next: now.Add(actorInitialDelayMin + time.Duration(random(int(actorInitialDelayRange/time.Millisecond)))*time.Millisecond)}
			w.actors.states[mapID][n.ClickID] = state
		}
		if paused[n.ClickID] || w.Defeated(mapID, n.ClickID) || now.Before(state.next) {
			continue
		}
		x, y := int(n.X), int(n.Y)
		delay := actorPatrolDelayMin + time.Duration(random(int(actorPatrolDelayRange/time.Millisecond)))*time.Millisecond
		switch {
		case def.WalkBehavior == walkBounds && len(def.WalkSteps) >= 2:
			lo, hi := def.WalkSteps[0], def.WalkSteps[1]
			minX, minY := max(-actorBoundsLimit, min(0, int(int32(lo.X)))), max(-actorBoundsLimit, min(0, int(int32(lo.Y))))
			maxX, maxY := max(0, min(actorBoundsLimit, int(int32(hi.X)))), max(0, min(actorBoundsLimit, int(int32(hi.Y))))
			x += minX + random(maxX-minX+1)
			y += minY + random(maxY-minY+1)
			x, y = max(actorCoordinateMinimum, min(actorCoordinateMaximum, x)), max(actorCoordinateMinimum, min(actorCoordinateMaximum, y))
			delay = actorBoundsDelayMin + time.Duration(random(int(actorBoundsDelayRange/time.Millisecond)))*time.Millisecond
		case (def.WalkBehavior == walkPatrol || def.WalkBehavior == walkPatrolAlternate) && len(def.WalkSteps) > 0:
			path := def.WalkSteps
			step := path[state.step%len(path)]
			use := len(path) > 1 || state.step%2 == 0
			if use && step.X > 0 && step.X < actorWaypointLimit && step.Y > 0 && step.Y < actorWaypointLimit {
				x, y = int(step.X), int(step.Y)
				if step.Delay > 0 {
					delay = max(actorMinimumPathDelay, time.Duration(step.Delay)*time.Millisecond)
				}
			}
			if len(path) == 1 {
				state.step = (state.step + 1) % 2
			} else {
				state.step = (state.step + 1) % len(path)
			}
		case (w.Wild(mapID, n) || def.WalkBehavior == walkRoam) && !RoamingTown(mapID):
			x, y = int(state.x)+random(actorRoamStep*2+1)-actorRoamStep, int(state.y)+random(actorRoamStep*2+1)-actorRoamStep
			if abs(x-int(n.X)) > actorSpawnLeash || abs(y-int(n.Y)) > actorSpawnLeash {
				x, y = int(n.X)+random(actorReturnRadius*2+1)-actorReturnRadius, int(n.Y)+random(actorReturnRadius*2+1)-actorReturnRadius
			}
			x, y = max(actorCoordinateMinimum, min(actorCoordinateMaximum, x)), max(actorCoordinateMinimum, min(actorCoordinateMaximum, y))
			delay = actorRoamDelayMin + time.Duration(random(int(actorRoamDelayRange/time.Millisecond)))*time.Millisecond
		default:
			state.next = now.Add(actorStationaryDelay)
			continue
		}
		state.next = now.Add(delay)
		if !w.CanMove(mapID, state.x, state.y, uint16(x), uint16(y)) || x == int(state.x) && y == int(state.y) {
			continue
		}
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
