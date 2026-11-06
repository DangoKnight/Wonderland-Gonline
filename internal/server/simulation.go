package server

import (
	"context"
	"math"
	"math/rand/v2"
	"sort"
	"time"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/world"
)

const (
	actorDetectionRadius     = 200
	actorTargetScanInterval  = 500 * time.Millisecond
	actorSimulationInterval  = 500 * time.Millisecond
	worldMaintenanceInterval = time.Second
)

// The existing process ticker owns ambient movement as well as respawns.
// worldMu serializes it with map entry, movement, events and battle transitions.
func (s *Server) simulateWorld(now time.Time) {
	s.simulateWorldTick(now, true)
}

// Scan-only ticks retain targets without issuing additional movement packets.
func (s *Server) simulateWorldTick(now time.Time, movement bool) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	maps := map[uint16][]*Session{}
	for _, c := range s.world {
		s.validatePartyScene(c.party)
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
			if !scan && pursuit.target == 0 {
				continue
			}
			nearest := int64(actorDetectionRadius*actorDetectionRadius + 1)
			for _, c := range viewers {
				if !scan && c.character.ID != pursuit.target {
					continue
				}
				dx, dy := int64(c.character.X)-int64(npc.X), int64(c.character.Y)-int64(npc.Y)
				distance := dx*dx + dy*dy
				if distance > actorDetectionRadius*actorDetectionRadius || distance >= nearest {
					continue
				}
				if !ambientEncounterAvailable(c, now) || !bounds.Contains(c.character.X, c.character.Y) ||
					c.view.Hidden[npc.ClickID] || !s.World.VisibleIn(c.character, c.view, mapID, npc.ClickID) || s.World.InParty(c.character, npc.Template) {
					continue
				}
				if distance < nearest {
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
		if !movement {
			continue
		}
		for _, move := range s.World.AdvanceActorsToward(mapID, now, paused, targets, nil) {
			npc, _ := s.World.NPC(mapID, move.Click)
			packet := move.Packet()
			for _, c := range viewers {
				if c.view.Hidden[move.Click] || !s.World.VisibleIn(c.character, c.view, mapID, move.Click) || s.World.InParty(c.character, npc.Template) {
					continue
				}
				s.sendOrClose(c, packet)
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

// npcStats resolves encounter metadata. Derived combat stats use the common formulas.
func (s *Server) npcStats(template uint32) (level int, element byte, name string) {
	level, name = 1, "Monster"
	if npc, ok := s.Assets.NPCs[uint16(template)]; ok && template <= 0xffff {
		level, element = max(1, int(npc.Level)), npc.Element
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
	level, element, name := s.npcStats(n.Template)
	template := n.Template
	if template == 0 {
		template = 17003
	}
	var enemies []battle.Enemy
	for i := range 1 + rand.IntN(4) {
		lv := max(1, level+rand.IntN(3)-1)
		enemies = append(enemies, battle.Enemy{Template: template, Name: name, Level: lv, Element: element, ClickID: uint16(2000 + i)})
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
		level, element, name := s.npcStats(n.Template)
		template := n.Template
		if template == 0 {
			template = 17003
		}
		enemies = append(enemies, battle.Enemy{Template: template, Name: name, Level: level, Element: element, ClickID: uint16(2000 + i)})
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
	level, element, name := s.npcStats(n.Template)
	enemy := battle.Enemy{Template: n.Template, Name: name, Level: level, Element: element, ClickID: n.ClickID}
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

// AC5:5 resets the actor shape. The reference server's proposed AC23:122 success effect is
// fishing cleanup in the verified WLRI client and must not be sent here.
func (s *Server) clearShapeshift(c *Session) {
	c.shapeshift = 0
	p := shapeshiftPacket(c)
	s.broadcastWorld(c, p)
	s.sendOrClose(c, p)
}

func shapeshiftPacket(c *Session) []byte {
	return protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateShapeshift}.U32(c.character.ID).U16(c.shapeshift)
}

// Resting recovery is a server rule of this port: neither aLogin nor the
// reference server defines one. A character holding a sitting or lying
// pose outside battles and events and their active pet recover
// restRecoveryPercent of their own maximum HP/SP (at least one point)
// every restRecoveryInterval.
const (
	restRecoveryInterval = 3 * time.Second
	restRecoveryPercent  = 2
)

func restingPose(pose byte) bool {
	return (pose >= protocol.PoseLieFirst && pose <= protocol.PoseLieLast) ||
		(pose >= protocol.PoseSitFirst && pose <= protocol.PoseSitLast)
}

// restCharacters runs on the one-second world ticker. Recovery is a
// buffered session field like walking: it updates memory, reports AC8
// current HP/SP, and leaves persistence to the guarded checkpoint.
func (s *Server) restCharacters(now time.Time) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	for _, c := range s.world {
		s.rest(c, now)
	}
}

// rest recovers one session. Caller holds worldMu.
func (s *Server) rest(c *Session, now time.Time) {
	if !c.ready || c.character == nil || c.battle != nil || c.event != nil || !restingPose(c.emote) {
		c.restedAt = time.Time{}
		return
	}
	if c.restedAt.IsZero() {
		c.restedAt = now
		return
	}
	if now.Sub(c.restedAt) < restRecoveryInterval {
		return
	}
	c.restedAt = now
	full := c.character.Combat(s.Assets.Items)
	maxHP, maxSP := uint32(max(full.MaxHP, 1)), uint32(max(full.MaxSP, 0))
	gain := func(current, maximum uint32) uint32 {
		if current >= maximum {
			return current
		}
		return min(maximum, current+max(1, maximum*restRecoveryPercent/100))
	}
	hp, sp := gain(c.character.HP, maxHP), gain(c.character.SP, maxSP)
	baseline := func() {
		if c.autosaveBaseline == nil {
			before := c.character.Clone()
			c.autosaveBaseline = &before
		}
	}
	var packets [][]byte
	if hp != c.character.HP {
		baseline()
		c.character.HP = hp
		packets = append(packets, serverwire.VitalPacket(game.StatCurrentHP, hp))
	}
	if sp != c.character.SP {
		baseline()
		c.character.SP = sp
		packets = append(packets, serverwire.VitalPacket(game.StatCurrentSP, sp))
	}
	if pet := c.character.BattlePet(); pet != nil {
		full := pet.Combat(s.Assets.Items, s.petTemplate(pet.ID))
		hp := int32(gain(uint32(max(pet.HP, 0)), uint32(max(full.MaxHP, 1))))
		sp := int32(gain(uint32(max(pet.SP, 0)), uint32(max(full.MaxSP, 0))))
		var slot byte
		if c.pets != nil {
			slot = c.pets.slot(pet.ID)
		}
		if hp != pet.HP {
			baseline()
			pet.HP = hp
			if slot != 0 {
				packets = append(packets, game.PetStat(slot, game.StatCurrentHP, int64(hp)))
			}
		}
		if sp != pet.SP {
			baseline()
			pet.SP = sp
			if slot != 0 {
				packets = append(packets, game.PetStat(slot, game.StatCurrentSP, int64(sp)))
			}
		}
	}
	if err := s.sendAll(c, packets); err != nil {
		s.Log.Debug("resting recovery report failed", "session", c.info.ID, "error", err)
	}
}
