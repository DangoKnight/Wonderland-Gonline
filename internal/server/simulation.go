package server

import (
	"sort"
	"time"
	"wonderland-go/internal/world"
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
	ids := make([]int, 0, len(maps))
	for id := range maps {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		mapID := uint16(id)
		viewers := maps[mapID]
		paused := map[uint16]bool{}
		for _, c := range viewers {
			if c.battle != nil {
				paused[c.battle.encounter] = true
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
		for _, move := range s.World.AdvanceActors(mapID, now, paused, nil) {
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
