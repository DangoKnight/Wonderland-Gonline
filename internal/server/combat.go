package server

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

// turnGrace is a compatibility margin over the native countdown: the client
// accepts commands until its counter passes 0 (FUN_0039800c reaches -1 one
// second later), and the command still has to cross the network.
const turnGrace = 2 * time.Second

// defaultTurnTimeout is the native countdown of the kind this server
// announces, plus the grace.
var defaultTurnTimeout = battle.TurnLimit(battle.KindStandard) + turnGrace

// Battle timing; variables so tests need not wait.
var (
	turnTimeout = defaultTurnTimeout
	battleSleep = time.Sleep
)

// battleRun is one PvE or PvP battle of characters and their nearby teammates
// (GetTeamMembers). Its quest event stays suspended until the outcome callback
// resumes or releases it. Reference: PvEBattleManager.
type battleRun struct {
	b       *battle.Battle
	members []*battleMember // members[0] started the battle and owns its event and loot.
	event   *eventSession   // The suspended event, if a quest action started the battle.
	source  uint16          // The battle action's d1: the callback branch's source.
	flee    bool            // The authored flee continuation (EveEventInterpreter OnFlee list).
	timer   *time.Timer
	turn    int // Increments per round so a stale timer cannot fire into a new round.
	// encounter is the visible overworld actor reserved for this battle.
	encounter    uint16
	encounterMap uint16
	wild         bool
	trial        *assets.CombatTrial
}

// battleMember is one participant: its character fighter and battle pet, if any.
type battleMember struct {
	c    *Session
	self *battle.Fighter
	pet  *battle.Fighter
}

func (run *battleRun) member(c *Session) *battleMember {
	for _, m := range run.members {
		if m.c == c {
			return m
		}
	}
	return nil
}

// active lists members still in this battle. Caller holds worldMu.
func (run *battleRun) active() []*battleMember {
	var out []*battleMember
	for _, m := range run.members {
		if m.c.battle == run {
			out = append(out, m)
		}
	}
	return out
}

func (s *Server) rules() battle.Rules {
	return battle.Rules{ComboDamagePerParticipant: s.comboDamagePerParticipant, Skills: s.Assets.Skills, Timing: s.Assets.AnimationTiming, Critical: s.Assets.Critical,
		Next: func(lo, hi int) int { return lo + rand.IntN(hi-lo) }, Float: rand.Float64}
}

// fleeContinues lists the story battles whose flee resumes the event (OnFlee in C#).
func fleeContinues(mapID, event uint16) bool {
	switch mapID {
	case game.MapID12050:
		return event == 39 || event == 16
	case game.MapID12002:
		return event == 5 || event == 7
	case game.MapID60001:
		return event == 44 || event == 48 || event == 83
	case game.MapID11077:
		return event == 7
	case game.MapID12523:
		return event == 2 || event == 12
	}
	return false
}

// questBattle is ExecuteOpcode 4/6: the formation (opcode 4) or a template operand names
// the enemies. It returns false when no battle can be built. Caller holds worldMu.
func (s *Server) questBattle(c *Session, es *eventSession, op world.Op) (bool, error) {
	var formation []uint32
	if op.Code == world.ActionBattle {
		formation = s.World.Formation(es.mapID, op.D2)
	}
	id := uint32(0)
	switch {
	case len(formation) > 0:
		id = formation[0]
	case op.D2 >= 10000:
		id = uint32(op.D2)
	case op.D1 >= 10000:
		id = uint32(op.D1)
	}
	if id == 0 {
		s.Log.Info("battle has no formation", "map", es.mapID, "event", es.ev.ClickID)
		return false, nil
	}
	level := max(5, int(op.D3))
	hp := level*35 + 200
	if op.DW1 > 0 {
		hp = int(op.DW1)
	}
	templates := formation
	if len(templates) == 0 {
		templates = []uint32{id}
	}
	var enemies []battle.Enemy
	for _, t := range templates {
		e := battle.Enemy{Template: t, Name: s.npcName(t), Level: level, HP: hp, ClickID: es.click}
		// A formation enemy uses its own Npc.dat level, HP and element.
		if npc, ok := s.Assets.NPCs[uint16(t)]; ok && len(formation) > 0 && t <= 0xffff {
			e.Level, e.HP, e.Element = max(1, int(npc.Level)), max(1, int(npc.HP)), npc.Element
		}
		enemies = append(enemies, e)
	}
	if e := c.send([]byte{protocol.CommandEvent, protocol.EventResume}); e != nil {
		return false, e
	}
	run := &battleRun{event: es, source: op.D1, flee: fleeContinues(es.mapID, es.ev.ClickID)}
	return true, s.startBattle(c, run, enemies)
}

func (s *Server) npcName(id uint32) string {
	if npc, ok := s.Assets.NPCs[uint16(id)]; ok && id <= 0xffff && npc.Name != "" {
		return npc.Name
	}
	return "Monster"
}

// battleState is AC11:4, the crossed-swords indicator for everyone on the map.
func battleState(id uint32, in bool) []byte {
	state := byte(0)
	if in {
		state = 1
	}
	return protocol.Builder{protocol.CommandBattleState, protocol.BattleStateParticipation, 2}.U32(id).U8(0).U8(0).U8(state)
}

// battleTeam is GetTeamMembers: the character, then its teammates on the same
// map, at most four. Teammates already fighting or in an event stay out.
// Caller holds worldMu.
func (s *Server) battleTeam(c *Session) []*Session {
	team := []*Session{c}
	if c.party == nil {
		return team
	}
	for _, m := range c.party.members {
		if m != c && len(team) < partyMax && m.ready && m.battle == nil && m.event == nil && s.samePlayerScene(m, c) {
			team = append(team, m)
		}
	}
	return team
}

// startBattle is InitializeAndStartBattle for a character and its team. Caller
// holds worldMu.
func (s *Server) startBattle(c *Session, run *battleRun, enemies []battle.Enemy) error {
	var fighters []*battle.Fighter
	for i, m := range s.battleTeam(c) {
		s.cancelTrade(m)
		member := &battleMember{c: m, self: battle.PlayerFighter(m.character, s.Assets.Items, i)}
		fighters = append(fighters, member.self)
		if pet := m.character.BattlePet(); pet != nil {
			copy := *pet
			copy.Skills = append([]game.PetSkill(nil), pet.Skills...)
			copy.Normalize(s.Assets.Items, false)
			copy.Name = s.companionName(copy.ID, copy.Name)
			member.pet = battle.PetFighter(copy, m.character.ID, s.Assets.Items, i)
			fighters = append(fighters, member.pet)
		}
		run.members = append(run.members, member)
	}
	for i := range enemies {
		if n, ok := s.Assets.NPCs[uint16(enemies[i].Template)]; ok && enemies[i].Template <= 0xffff {
			enemies[i].Skills = n.Skills
		}
	}
	run.b = battle.New(fighters, enemies)
	background := uint16(1)
	if c.character.Map < game.MapID10000 {
		background = c.character.Map
	}
	for _, m := range run.members {
		for _, p := range m.c.character.Pets {
			run.b.Roster[m.c.character.ID] = append(run.b.Roster[m.c.character.ID], p.ID)
		}
		m.c.battle = run
	}
	for _, m := range run.members {
		if err := s.battleIntro(run, m, background); err != nil {
			if m.c == c {
				run.b.Finished = true
				for _, member := range run.members {
					member.c.battle = nil
					member.c.conn.Close()
				}
				return err
			}
			m.c.conn.Close()
		}
	}
	if run.encounter == 0 && run.event != nil && s.World.RoamingBattle(run.event.mapID, run.event.click) {
		run.encounter = run.event.click
	}
	if run.encounter != 0 {
		run.encounterMap = c.character.Map
		s.holdEncounterMonster(run.encounterMap, run.encounter)
	}
	s.armTurnTimer(run)
	return nil
}

func (s *Server) battleIntro(run *battleRun, m *battleMember, background uint16) error {
	if i := battlePetIndex(*m.c.character, m); i >= 0 {
		pet := m.c.character.Pets[i]
		if err := s.sendAll(m.c, pet.ProgressionPackets(m.c.pets.slot(pet.ID), s.Assets.Items)); err != nil {
			return err
		}
	}
	if m.c.party != nil {
		s.partyUpdate(m.c.party)
	}
	intro := run.b.Intro(m.self, background)
	if e := s.sendAll(m.c, intro[:2]); e != nil {
		return e
	}
	state := battleState(m.c.character.ID, true)
	s.broadcastWorld(m.c, state)
	if e := m.c.send(state); e != nil {
		return e
	}
	return s.sendAll(m.c, intro[2:])
}

func (s *Server) armTurnTimer(run *battleRun) {
	turn := run.turn
	run.timer = time.AfterFunc(turnTimeout, func() {
		s.worldMu.Lock()
		defer s.worldMu.Unlock()
		if len(run.active()) == 0 || run.turn != turn || run.b.Processing || run.b.Finished {
			return
		}
		run.b.Timeout()
		s.tryRound(run)
	})
}

// battleCommand routes AC50 commands and the AC11:1 escape while in battle. The caller
// holds worldMu.
func (s *Server) battleCommand(c *Session, p []byte) error {
	run := c.battle
	switch p[0] {
	case protocol.CommandBattleAction:
		if len(p) < 2 {
			return protocol.ErrMalformed
		}
		ack := run.b.Submit(s.rules(), c.character.ID, p[1], p[2:])
		if ack == nil {
			return nil
		}
		for _, member := range run.active() {
			s.sendOrClose(member.c, ack)
		}
		s.tryRound(run)
	case protocol.CommandBattleState:
		if len(p) >= 3 && p[1] == protocol.BattleStateExit && p[2] == 3 {
			// Any member's escape ends the battle for the whole team.
			s.endBattle(run, battle.Fled)
		}
	}
	return nil
}

// tryRound is TryExecuteTurn: once every command is in, compute the round now and play
// its animations in the background. Caller holds worldMu.
func (s *Server) tryRound(run *battleRun) {
	if !run.b.Ready() {
		return
	}
	run.b.Processing = true
	if run.timer != nil {
		run.timer.Stop()
	}
	run.turn++
	steps, outcome := run.b.Round(s.rules())
	go s.playRound(run, steps, outcome)
}

// play sends each step to every member and waits for its animation without
// holding worldMu. It stops when no member remains.
func (s *Server) play(run *battleRun, steps []battle.Step) bool {
	for _, step := range steps {
		s.worldMu.Lock()
		if run.b.Finished {
			s.worldMu.Unlock()
			return false
		}
		members := run.active()
		for _, m := range members {
			progress, err := s.commitSkillUses(m.c, run, step.SkillUses)
			if err != nil {
				s.Log.Warn("battle skill progress not committed", "character", m.c.character.ID, "error", err)
				s.abandonBattle(m.c)
				m.c.conn.Close()
				continue
			}
			if s.sendAll(m.c, append(progress, step.Packets...)) != nil {
				m.c.conn.Close()
			}
		}
		alive := len(run.active()) > 0
		s.worldMu.Unlock()
		if !alive {
			return false
		}
		if step.Delay > 0 {
			battleSleep(step.Delay)
		}
	}
	return true
}

// commitSkillUses applies only the member's executed actions, before their success
// packets. Battle fighters keep detached pet copies; refresh their skills after adoption.
func (s *Server) commitSkillUses(c *Session, run *battleRun, uses []battle.Action) ([][]byte, error) {
	m := run.member(c)
	if len(uses) == 0 || m == nil {
		return nil, nil
	}
	next := c.character.Clone()
	var packets [][]byte
	changed := false
	for _, use := range uses {
		if use.Actor == m.self {
			before := append([]game.LearnedSkill(nil), next.Skills...)
			updates := next.AddSkillEXP(use.Skill, 1)
			changed = changed || len(updates) > 0
			packets = append(packets, updates...)
			for i, skill := range next.Skills {
				if skill.Grade > before[i].Grade {
					packets = append(packets, next.UnlockQualifiedSkills(true, s.hasSkill)...)
					break
				}
			}
		} else if m.pet != nil && use.Actor == m.pet {
			if i := battlePetIndex(next, m); i >= 0 {
				for _, skill := range next.Pets[i].Skills {
					if skill.ID == use.Skill && skill.Grade > 0 && skill.Grade < 10 {
						changed = true
					}
				}
				packets = append(packets, next.Pets[i].AddSkillEXP(use.Skill, 1, c.pets.slot(next.Pets[i].ID))...)
			}
		}
	}
	if !changed {
		return nil, nil
	}
	if err := s.commit(context.Background(), c, next); err != nil {
		return nil, err
	}
	m.self.Char = c.character
	if i := battlePetIndex(next, m); i >= 0 {
		m.pet.Pet.Skills = append([]game.PetSkill(nil), next.Pets[i].Skills...)
	}
	return packets, nil
}

func (s *Server) playRound(run *battleRun, steps []battle.Step, outcome battle.Outcome) {
	if !s.play(run, steps) {
		return
	}
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if len(run.active()) == 0 || run.b.Finished {
		return
	}
	if outcome != battle.Continue {
		s.endBattle(run, outcome)
		return
	}
	run.b.Processing = false
	for _, m := range run.active() {
		if s.sendAll(m.c, run.b.NextRound(m.c.character.ID)) != nil {
			m.c.conn.Close()
		}
	}
	s.armTurnTimer(run)
}

// memberResult is one member's committed ending, played after the departure.
type memberResult struct {
	m           *battleMember
	first, last [][]byte
}

// endBattle runs EndBattleVictory, EndBattleDefeat or EndBattleFlee for every
// member. Each winner earns the full EXP and gold; loot goes to the member who
// started the battle. Rewards and HP are committed before the departure plays.
// Caller holds worldMu.
func (s *Server) endBattle(run *battleRun, outcome battle.Outcome) {
	if run.b.Finished {
		return
	}
	run.b.Finished = true
	if run.timer != nil {
		run.timer.Stop()
	}
	delay := s.rules().Delay(0, "exit")
	var departure [][]byte
	switch outcome {
	case battle.Victory:
		departure = run.b.Departure(true)
	case battle.Defeat:
		departure = run.b.Departure(false)
	case battle.Fled:
		departure = run.b.Escape()
		delay = s.rules().Delay(0, "flee")
	}
	exp, gold := run.b.Rewards()
	exp = ScaleExperience(exp, s.expRateMultiplier)
	var results []memberResult
	for _, m := range run.active() {
		c := m.c
		previousQuests := c.character.Quests
		next := c.character.Clone()
		next.HP, next.SP = uint32(max(0, m.self.HP)), uint32(max(0, m.self.SP))
		var first, last [][]byte
		memberOutcome := outcome
		if run.b.PvP && m.self.Side == battle.Defender {
			if outcome == battle.Victory {
				memberOutcome = battle.Defeat
			} else if outcome == battle.Defeat {
				memberOutcome = battle.Victory
			}
		}
		switch memberOutcome {
		case battle.Victory:
			if run.b.PvP {
				break
			}
			first = append(first, s.discoverBattleMonsters(&next, run)...)
			first = append(first, s.customQuestKills(&next, run)...)
			if run.trial != nil {
				first = append(first, s.trialReward(&next, run.trial)...)
			}
			if next.Gold < game.MaxGold {
				next.Gold = uint32(min(uint64(next.Gold)+gold, uint64(game.MaxGold)))
			}
			var adds []game.Addition
			if m == run.members[0] {
				var dropped []string
				adds, dropped = s.rollLoot(run, &next)
				if len(dropped) > 0 {
					last = append(last, headBanner("Obtain "+strings.Join(dropped, ", ")))
				}
			}
			levels := next.AddExp(exp)
			if levels > 0 {
				next.Refill(s.Assets.Items)
			}
			if j := battlePetIndex(next, m); j >= 0 {
				// The pet receives the same once-scaled reward as its owner.
				pet := &next.Pets[j]
				pet.HP = int32(max(1, m.pet.HP))
				grown := s.gainPetExp(pet, uint32(min(exp, 1<<31-1)), petGrowth)
				pet.Normalize(s.Assets.Items, grown > 0)
			}
			if len(adds) > 0 {
				first = append(first, next.Bag.AdditionPacket(adds))
			}
			first = append(first, next.ExpPacket())
			if levels > 0 {
				first = append(first, next.StatPackets(s.Assets.Items)...)
			}
			// C# adds the gold without telling the client; send the new balance.
			first = append(first, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold))
		case battle.Defeat:
			full := next.Combat(s.Assets.Items)
			next.HP = uint32(max(10, int(full.MaxHP)/2))
			next.MaxHP = max(next.MaxHP, next.HP)
		}
		next.HP, next.SP = min(next.HP, next.MaxHP), min(next.SP, next.MaxSP)
		roster := newPetRoster()
		roster.synced = c.pets.synced
		for id, slot := range c.pets.slots {
			roster.slots[id] = slot
		}
		if memberOutcome == battle.Victory && !run.b.PvP {
			if run.trial == nil && m == run.members[0] && run.event == nil {
				first = append(first, s.legacyBattleRewards(&next, run, roster)...)
			}
		}
		petPackets, peerPackets := s.petResults(run, m, &next, memberOutcome, roster)
		first = append(append(first, petPackets...), departure...)
		if e := s.commit(context.Background(), c, next); e != nil {
			s.Log.Warn("battle result not committed", "character", next.ID, "error", e)
			s.abandonBattle(c)
			c.conn.Close()
			continue
		}
		var completed []uint32
		for id, q := range next.Quests {
			if before, exists := previousQuests[id]; !exists || before.State != q.State || before.Step != q.Step || before.Kills != q.Kills {
				completed = append(completed, id)
			}
		}
		sort.Slice(completed, func(i, j int) bool { return completed[i] < completed[j] })
		var questPackets [][]byte
		for _, id := range completed {
			questPackets = append(questPackets, s.World.QuestUpdate(c.view, id, next.Quests[id])...)
		}
		first = append(questPackets, first...)
		c.pets = roster
		last = append(last, s.World.Sync(c.character, c.view, false)...)
		for _, packet := range peerPackets {
			s.broadcastWorld(c, packet)
		}
		results = append(results, memberResult{m, first, last})
	}
	if len(results) > 0 {
		go s.finishBattle(run, outcome, results, delay)
	}
}

// rollLoot rolls every uncaptured monster's drops into next's bag.
func (s *Server) rollLoot(run *battleRun, next *game.Character) ([]game.Addition, []string) {
	var adds []game.Addition
	var dropped []string
	r := s.rules()
	r.DropRateMultiplier = s.dropRateMultiplier
	for _, m := range run.b.Defenders {
		if m.Captured || m.Kind != battle.Monster || run.b.PvP {
			continue
		}
		var native [5]uint16
		if npc, ok := s.Assets.NPCs[uint16(m.Template)]; ok && m.Template <= 0xffff {
			native = npc.Drops
		}
		for _, d := range r.RollDrops(s.Assets.Drops[m.Template], native, func(id uint16) bool { _, ok := s.Assets.Items[id]; return ok }) {
			limit, _ := s.stackLimit(d.Item)
			if a, e := next.Bag.Grant(game.Item{ID: d.Item}, int(d.Count), limit, s.Assets.Items); e == nil {
				adds = append(adds, a...)
				dropped = append(dropped, fmt.Sprintf("%s x%d", s.itemName(d.Item), d.Count))
			}
		}
	}
	return adds, dropped
}

// finishBattle plays the departure, then returns every member to the map. Only
// the member who started the battle continues its event.
func (s *Server) finishBattle(run *battleRun, outcome battle.Outcome, results []memberResult, delay time.Duration) {
	s.worldMu.Lock()
	for _, r := range results {
		if r.m.c.battle == run && s.sendAll(r.m.c, r.first) != nil {
			r.m.c.conn.Close()
		}
	}
	s.worldMu.Unlock()
	battleSleep(delay)
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	var closes [][]byte
	for _, r := range results {
		closes = append(closes, protocol.Builder{protocol.CommandBattleState, protocol.BattleStateWireCode0}.U32(r.m.c.character.ID).U16(0))
	}
	var leader *Session
	for _, r := range results {
		c := r.m.c
		if c.battle != run {
			continue
		}
		c.battle = nil
		c.encounter.battleOver()
		if outcome == battle.Victory && run.wild {
			c.encounter.armed = true
		}
		if r.m == run.members[0] {
			leader = c
		}
		id := c.character.ID
		packets := append(append([][]byte(nil), closes...), []byte{protocol.CommandMovement, protocol.MovementMovementLock, 0}, []byte{protocol.CommandEvent, protocol.EventResume})
		packets = append(packets, c.character.StatPackets(s.Assets.Items)...)
		state := battleState(id, false)
		packets = append(append(packets, state), r.last...)
		if s.sendAll(c, packets) != nil {
			c.conn.Close()
			continue
		}
		s.broadcastWorld(c, state)
	}
	if run.encounter != 0 {
		s.World.ReleaseEncounter(run.encounterMap, run.encounter, time.Now())
	}
	es := run.event
	if leader == nil || es == nil || leader.event != es {
		return
	}
	c := leader
	result := uint16(1)
	switch {
	case outcome == battle.Fled && !run.flee:
		// The client is already released; abandon an event with no flee continuation.
		s.endEvent(c)
		return
	case outcome != battle.Victory:
		result = 2
	}
	// RunOutcome: the battle callback branch (trigger 4) continues the event.
	next := s.World.OutcomeBranch(c.character, c.view, es.mapID, es.ev, run.source, result, es.branch)
	if next < 0 || c.character.Map != es.mapID {
		_ = s.cancelInteraction(c)
		return
	}
	_ = s.startEvent(context.Background(), c, es.click, es.ev, next, true)
}

// abandonBattle is OnPlayerDisconnect: the member leaves; its fighters stay and
// defend. The battle ends when no member remains. Caller holds worldMu.
func (s *Server) abandonBattle(c *Session) {
	run := c.battle
	if run == nil {
		return
	}
	c.battle = nil
	run.b.Leave(c.character.ID)
	if len(run.active()) == 0 {
		if run.encounter != 0 {
			s.World.ReleaseEncounter(run.encounterMap, run.encounter, time.Now())
		}
		run.b.Finished = true
		if run.timer != nil {
			run.timer.Stop()
		}
		return
	}
	if !run.b.Finished {
		if outcome := run.b.Forfeit(); outcome != battle.Continue {
			s.endBattle(run, outcome)
			return
		}
		s.tryRound(run)
	}
}

func battlePetIndex(char game.Character, m *battleMember) int {
	if m == nil || m.pet == nil {
		return -1
	}
	i, _ := char.Pet(m.pet.Pet.ID)
	return i
}

// petResults applies a battle's pet outcome to next: HP/SP, one amity point per
// knock-out (deserting below 20), and captured monsters joining the party. It returns
// the owner's packets and deferred peer broadcasts. Only the staged roster is
// changed here; the caller adopts it and publishes packets after a durable save.
func (s *Server) petResults(run *battleRun, m *battleMember, next *game.Character, outcome battle.Outcome, roster *petRoster) ([][]byte, [][]byte) {
	var packets, peerPackets [][]byte
	if i := battlePetIndex(*next, m); i >= 0 {
		pet := &next.Pets[i]
		if outcome != battle.Victory || run.b.PvP {
			pet.HP = int32(max(0, m.pet.HP))
		}
		pet.SP = int32(max(0, m.pet.SP))
		slot := roster.slot(pet.ID)
		deaths := m.pet.Deaths
		if run.b.PvP {
			deaths = 0
		}
		pet.Amity = byte(max(0, int(pet.Amity)-deaths))
		if deaths > 0 && slot != 0 {
			packets = append(packets, game.PetStat(slot, 64, int64(pet.Amity)))
		}
		if pet.Amity < 20 && deaths > 0 {
			// The pet deserts the party.
			id := pet.ID
			if next.ActiveMount != 0 && game.SamePet(next.ActiveMount, id) {
				next.ActiveMount = 0
				packets = append(packets, unmountPackets(next.ID)...)
				peerPackets = append(peerPackets, unmountPackets(next.ID)...)
			}
			if next.ActivePet != 0 && game.SamePet(next.ActivePet, id) {
				next.ActivePet = 0
				despawn := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(next.ID).U32(0)
				packets = append(packets, []byte{protocol.CommandBattlePet, protocol.BattlePetRest})
				peerPackets = append(peerPackets, protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetWireCode7}.U32(next.ID))
				if next.ActiveMount == 0 {
					packets = append(packets, despawn)
					peerPackets = append(peerPackets, despawn)
				}
			}
			next.Pets = append(next.Pets[:i], next.Pets[i+1:]...)
			if released := roster.release(id); released != 0 {
				removed := protocol.Builder{protocol.CommandPetControl, protocol.PetControlPetSlot}.U32(next.ID).U8(released)
				packets = append(packets, removed)
				peerPackets = append(peerPackets, removed)
			}
		} else if slot != 0 {
			pet.Normalize(s.Assets.Items, false)
			packets = append(packets, pet.ProgressionPackets(slot, s.Assets.Items)...)
		}
	}
	for _, catch := range run.b.Captures {
		if catch.Owner != next.ID {
			continue
		}
		slot := next.FreePetSlot()
		if slot == 0 {
			continue
		}
		t := s.petTemplate(catch.Template)
		pet := game.NewPet(catch.Template, catch.Name, slot, t, s.Assets.Items)
		pet.Level = byte(max(1, min(catch.Level, game.MaxLevel)))
		pet.EnsureSkills(t, s.hasSkill)
		pet.Normalize(s.Assets.Items, true)
		next.Pets = append(next.Pets, pet)
		if roster.register(pet.ID) {
			packets = append(packets, pet.RecruitPacket(next.ID, t))
		}
		packets = append(packets, pet.ProgressionPackets(roster.slot(pet.ID), s.Assets.Items)...)
		packets = append(packets, systemLine(fmt.Sprintf("Successfully captured %s into Pet Slot #%d!", catch.Name, slot)))
	}
	return packets, peerPackets
}
