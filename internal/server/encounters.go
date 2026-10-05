package server

import "wonderland-go/internal/protocol"

import (
	"context"
	"math"
	"math/rand/v2"
	"time"
	"wonderland-go/internal/battle"
	"wonderland-go/internal/world"
)

// Field encounters. Reference: AC06.Recv1, PvEBattleManager.CheckAndTriggerRandomEncounter,
// StartProximityEncounter and QuestNpc.Interact/DefeatOverworldMonster.

const (
	proximityRadius = 72
	mapGrace        = 4 * time.Second
	firstEncounter  = 25
)

// encounterState is the per-session pacing of field battles.
type encounterState struct {
	steps, next int
	armed       bool // A proximity encounter needs the player to leave every monster's reach first.
	mapEnter    time.Time
	battleEnd   time.Time
	cooldown    time.Duration
}

func nextEncounter() int { return 18 + rand.IntN(17) }

// enterMap resets encounter pacing on every map entry (Player.CurMap).
func (e *encounterState) enterMap() {
	e.mapEnter, e.steps, e.armed = time.Now(), 0, true
}

// battleOver is SetBattleCooldown: a two-to-four-second grace before the next battle.
func (e *encounterState) battleOver() {
	e.battleEnd = time.Now()
	e.cooldown = 2*time.Second + time.Duration(rand.Float64()*float64(2*time.Second))
	e.steps, e.armed = 0, false
}

func (e *encounterState) resting() bool {
	return e.restingAt(time.Now())
}

func (e *encounterState) restingAt(now time.Time) bool {
	return (!e.battleEnd.IsZero() && now.Sub(e.battleEnd) < e.cooldown) || now.Sub(e.mapEnter) < mapGrace
}

// npcStats are the level, HP and element C# resolves for a map actor (Npc.dat here).
func (s *Server) npcStats(template uint32) (level, hp int, element byte, name string) {
	level, hp, name = 1, 100, "Monster"
	if npc, ok := s.Assets.NPCs[uint16(template)]; ok && template <= 0xffff {
		level, element = max(1, int(npc.Level)), npc.Element
		if npc.HP > 0 {
			hp = int(npc.HP)
		}
		if npc.Name != "" {
			name = npc.Name
		}
	}
	return
}

// stepEncounter runs after a move that started no region event. Caller holds worldMu.
func (s *Server) stepEncounter(c *Session, prevX, prevY uint16) error {
	char, enc := c.character, &c.encounter
	if c.battle != nil || c.event != nil || (char.X == prevX && char.Y == prevY) || enc.resting() || s.World.SafeTown(char.Map) {
		return nil
	}
	m, ok := s.World.Map(char.Map)
	if !ok {
		return nil
	}
	for _, n := range s.World.NPCs(char.Map) {
		if area, limited := s.World.ActorTargetArea(char.Map, n.ClickID); limited && !area.Contains(char.X, char.Y) {
			continue
		}
		if !s.World.Wild(char.Map, n) || s.World.Defeated(char.Map, n.ClickID) || c.view.Hidden[n.ClickID] ||
			!s.World.VisibleIn(char, c.view, char.Map, n.ClickID) ||
			math.Hypot(float64(int(char.X)-int(n.X)), float64(int(char.Y)-int(n.Y))) > proximityRadius {
			continue
		}
		if enc.armed {
			if e := s.proximityEncounter(c, n); e != nil {
				return e
			}
			if c.battle != nil {
				enc.armed, enc.steps, enc.next = false, 0, nextEncounter()
			}
		}
		return nil
	}
	enc.armed = true
	if enc.steps++; enc.steps >= enc.next {
		enc.steps, enc.next = 0, nextEncounter()
		return s.randomEncounter(c, m)
	}
	return nil
}

// proximityEncounter fights one to four copies of a nearby monster; a native roaming
// encounter runs its event instead.
func (s *Server) proximityEncounter(c *Session, n world.NPC) error {
	if s.World.RoamingBattle(c.character.Map, n.ClickID) {
		_, e := s.runNPCEvent(context.Background(), c, n.ClickID)
		return e
	}
	level, hp, element, name := s.npcStats(n.Template)
	template := n.Template
	if template == 0 {
		template = 17003
	}
	var enemies []battle.Enemy
	for i := range 1 + rand.IntN(4) {
		lv := max(1, level+rand.IntN(3)-1)
		enemies = append(enemies, battle.Enemy{Template: template, Name: name, Level: lv, Element: element,
			HP: int(float64(hp) * (0.9 + float64(rand.IntN(20))/100)), ClickID: uint16(2000 + i)})
	}
	return s.startBattle(c, &battleRun{encounter: n.ClickID, wild: true}, enemies)
}

// randomEncounter fights one to four monsters drawn from the map's wild actors.
func (s *Server) randomEncounter(c *Session, m *world.Map) error {
	var pool []world.NPC
	for _, n := range m.NPCs {
		if s.World.Wild(m.ID, n) && !s.World.Defeated(m.ID, n.ClickID) && !s.World.RoamingBattle(m.ID, n.ClickID) {
			pool = append(pool, n)
		}
	}
	if len(pool) == 0 {
		return nil
	}
	var enemies []battle.Enemy
	for i := range 1 + rand.IntN(4) {
		n := pool[rand.IntN(len(pool))]
		level, hp, element, name := s.npcStats(n.Template)
		template := n.Template
		if template == 0 {
			template = 17003
		}
		enemies = append(enemies, battle.Enemy{Template: template, Name: name, Level: level, HP: hp, Element: element, ClickID: uint16(2000 + i)})
	}
	return s.startBattle(c, &battleRun{}, enemies)
}

// wildClick is QuestNpc.Interact for a wild monster: a defeated one ignores the click,
// a plain one starts a battle. Roaming encounters run their events (handled=false).
func (s *Server) wildClick(c *Session, n world.NPC) (bool, error) {
	mapID := c.character.Map
	if !s.World.Wild(mapID, n) {
		return false, nil
	}
	if s.World.Defeated(mapID, n.ClickID) {
		return true, c.send([]byte{protocol.CommandEvent, protocol.EventResume})
	}
	if s.World.RoamingBattle(mapID, n.ClickID) {
		return false, nil
	}
	if e := c.send([]byte{protocol.CommandEvent, protocol.EventResume}); e != nil {
		return true, e
	}
	level, hp, _, name := s.npcStats(n.Template)
	enemy := battle.Enemy{Template: n.Template, Name: name, Level: level, HP: max(50, hp), ClickID: n.ClickID}
	return true, s.startBattle(c, &battleRun{encounter: n.ClickID, wild: true}, []battle.Enemy{enemy})
}

// holdEncounterMonster removes the actor for every public-scene viewer while
// the battle owns it. Completion or last-member disconnect schedules respawn.
func (s *Server) holdEncounterMonster(mapID, click uint16) {
	s.World.HoldEncounter(mapID, click)
	for _, peer := range s.world {
		if peer.ready && peer.character != nil && peer.character.Map == mapID && peer.tentOwner == 0 {
			if peer.send(s.World.HideActor(peer.view, mapID, click)) != nil {
				peer.conn.Close()
			}
		}
	}
}

// reviveMonsters shows respawned monsters to the players who may see them.
func (s *Server) reviveMonsters(now time.Time) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	for mapID, clicks := range s.World.Revive(now) {
		for _, peer := range s.world {
			if !peer.ready || peer.character == nil || peer.character.Map != mapID || peer.tentOwner != 0 {
				continue
			}
			for _, click := range clicks {
				if s.World.VisibleIn(peer.character, peer.view, mapID, click) {
					if peer.send(s.World.ShowActor(peer.view, mapID, click)) != nil {
						peer.conn.Close()
						break
					}
				}
			}
		}
	}
}
