package server

import (
	"math"
	"sort"
	"time"
	"wonderland-gonline/internal/world"
)

const (
	actorDetectionRadius    = 200
	actorTargetScanInterval = 3 * time.Second
)

// The existing process ticker owns ambient movement as well as respawns.
// worldMu serializes it with map entry, movement, events and battle transitions.
func (s *Server) simulateWorld(now time.Time) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	maps := map[uint16][]*Session{}
	for _, c := range s.world {
		if c.ready && c.character != nil && c.tentOwner == 0 {
			maps[c.character.Map] = append(maps[c.character.Map], c)
		}
	}
	if s.actorPursuits == nil {
		s.actorPursuits = map[uint16]map[uint16]*actorPursuit{}
	}
	for id := range s.actorPursuits {
		if len(maps[id]) == 0 {
			delete(s.actorPursuits, id)
		}
	}
	ids := make([]int, 0, len(maps))
	for id := range maps {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		mapID := uint16(id)
		viewers := maps[mapID]
		sort.Slice(viewers, func(i, j int) bool { return viewers[i].character.ID < viewers[j].character.ID })
		paused := map[uint16]bool{}
		for _, c := range viewers {
			if c.battle != nil {
				paused[c.battle.encounter] = true
				if c.battle.event != nil {
					paused[c.battle.event.click] = true
				}
			}
			if es := c.event; es != nil {
				paused[es.click] = true
				if es.ev != nil && es.branch >= 0 && es.branch < len(es.ev.Branches) {
					for _, o := range es.ev.Branches[es.branch].Operations {
						op := world.DecodeOp(o)
						if op.Code == world.ActionActor {
							paused[op.D1] = true
						}
					}
				}
			}
		}
		if s.actorPursuits[mapID] == nil {
			s.actorPursuits[mapID] = map[uint16]*actorPursuit{}
		}
		targets := map[uint16]world.ActorPosition{}
		pursued := map[uint16]*Session{}
		npcs := s.World.NPCsAt(mapID, now)
		for _, npc := range npcs {
			bounds, mayPursue := s.World.ActorTargetArea(mapID, npc.ClickID)
			if !mayPursue || paused[npc.ClickID] || !s.World.Wild(mapID, npc) || s.World.Defeated(mapID, npc.ClickID) {
				delete(s.actorPursuits[mapID], npc.ClickID)
				continue
			}
			pursuit := s.actorPursuits[mapID][npc.ClickID]
			if pursuit == nil {
				pursuit = &actorPursuit{}
				s.actorPursuits[mapID][npc.ClickID] = pursuit
			}
			scan := pursuit.target == 0 && !now.Before(pursuit.nextScan)
			if scan {
				pursuit.target = 0
				pursuit.nextScan = now.Add(actorTargetScanInterval)
			}
			nearest := math.Inf(1)
			for _, c := range viewers {
				if !scan && c.character.ID != pursuit.target {
					continue
				}
				if !ambientEncounterAvailable(c, now) || !bounds.Contains(c.character.X, c.character.Y) ||
					c.view.Hidden[npc.ClickID] || !s.World.VisibleIn(c.character, c.view, mapID, npc.ClickID) || s.World.InParty(c.character, npc.Template) {
					continue
				}
				distance := math.Hypot(float64(int(c.character.X)-int(npc.X)), float64(int(c.character.Y)-int(npc.Y)))
				if distance <= actorDetectionRadius && distance < nearest {
					nearest = distance
					pursuit.target = c.character.ID
					pursued[npc.ClickID] = c
					targets[npc.ClickID] = world.ActorPosition{X: c.character.X, Y: c.character.Y}
				}
			}
			if pursued[npc.ClickID] == nil && pursuit.target != 0 {
				pursuit.nextScan = now.Add(actorTargetScanInterval)
				pursuit.target = 0
			}
		}
		// Check the position from the start of this tick: a destination packet
		// announces an animation, rather than instant arrival on the client.
		for _, npc := range npcs {
			c := pursued[npc.ClickID]
			if c == nil || !ambientEncounterAvailable(c, now) || s.World.ActorMoving(mapID, npc.ClickID, now) {
				continue
			}
			if math.Hypot(float64(int(c.character.X)-int(npc.X)), float64(int(c.character.Y)-int(npc.Y))) > proximityRadius ||
				!s.World.CanMove(mapID, npc.X, npc.Y, c.character.X, c.character.Y) {
				continue
			}
			if err := s.proximityEncounter(c, npc); err != nil {
				s.Log.Warn("ambient actor encounter failed", "actor", npc.ClickID, "error", err)
			} else if c.battle != nil {
				c.encounter.armed, c.encounter.steps = false, 0
				c.encounter.next = nextEncounter()
				paused[npc.ClickID] = true
			}
			if c.event != nil {
				paused[npc.ClickID] = true
			}
		}
		for _, move := range s.World.AdvanceActorsToward(mapID, now, paused, targets, nil) {
			npc, _ := s.World.NPC(mapID, move.Click)
			for _, c := range viewers {
				if c.view.Hidden[move.Click] || !s.World.VisibleIn(c.character, c.view, mapID, move.Click) || s.World.InParty(c.character, npc.Template) {
					continue
				}
				s.sendOrClose(c, move.Packet())
			}
		}
	}
}

// Ambient aggression must not interrupt an owned interaction or target a ghost.
func ambientEncounterAvailable(c *Session, now time.Time) bool {
	return commandTravelAvailable(c) && !c.invisible && c.tentOwner == 0 &&
		c.trade == nil && c.stall == nil && c.fishing == nil && c.gathering == nil &&
		c.manufacturing == nil && c.arcade == nil && !c.encounter.restingAt(now)
}
