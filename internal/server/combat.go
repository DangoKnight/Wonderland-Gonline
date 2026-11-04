package server

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
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
	rewardError error
	rewarded    map[*battle.Fighter]bool
	b           *battle.Battle
	members     []*battleMember // members[0] started the battle and owns its event and loot.
	event       *eventSession   // The suspended event, if a quest action started the battle.
	source      uint16          // The battle action's d1: the callback branch's source.
	flee        bool            // The authored flee continuation (EveEventInterpreter OnFlee list).
	timer       *time.Timer
	turn        int // Increments per round so a stale timer cannot fire into a new round.
	// encounter is the visible overworld actor reserved for this battle.
	encounter    uint16
	encounterMap uint16
	wild         bool
	trial        *assets.CombatTrial
	petChanges   []battle.Step                // Native exit/join steps before the next round's actions.
	items        map[uint16]assets.BattleItem // Immutable definition snapshot for this encounter.
	withdrawals  [][]byte                     // Forced removals published after the already-calculated round's playback.
}

// battleMember is one participant: its character fighter and battle pet, if any.
type battleMember struct {
	c            *Session
	self         *battle.Fighter
	pet          *battle.Fighter
	choiceToken  uint32
	roundBundled bool // This recipient already has this round's native steps.
	resultSaved  bool // Final vitals/pet consequences committed; do not settle twice on disconnect.
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
		ExperienceFormula: s.battleEXPFormula,
		NPCs:              s.Assets.NPCs,
		Next:              func(lo, hi int) int { return lo + rand.IntN(hi-lo) }, Float: rand.Float64}
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
	var members []world.FormationMember
	if op.Code == world.ActionBattle {
		members = s.World.FormationMembers(es.mapID, op.D2)
		for _, member := range members {
			formation = append(formation, member.Template)
		}
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
	hp := 0 // A positive event HP value overrides the stored NPC HP.
	if op.DW1 > 0 {
		hp = int(op.DW1)
	}
	templates := formation
	if len(templates) == 0 {
		templates = []uint32{id}
	}
	var enemies []battle.Enemy
	for index, t := range templates {
		e := battle.Enemy{Template: t, Name: s.npcName(t), Level: level, HP: hp, ClickID: es.click}
		if index < len(members) {
			e.FormationSlot = members[index].Slot
		}
		// A formation enemy uses its own Npc.dat level and element.
		if npc, ok := s.Assets.NPCs[uint16(t)]; ok && len(formation) > 0 && t <= 0xffff {
			e.Level, e.Element = max(1, int(npc.Level)), npc.Element
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
	for i := range enemies {
		e := &enemies[i]
		if e.Template <= math.MaxUint16 {
			if npc, ok := s.Assets.NPCs[uint16(e.Template)]; ok {
				e.Base = game.Attributes{Strength: npc.Stats[0], Constitution: npc.Stats[1], Intelligence: npc.Stats[2], Wisdom: npc.Stats[3], Agility: npc.Stats[4]}
				e.Element = npc.Element
				if e.HP <= 0 {
					e.HP = int(npc.HP)
				}
				e.SP = int(npc.SP)
			}
		}
	}
	var fighters []*battle.Fighter
	for i, m := range s.battleTeam(c) {
		s.cancelTrade(m)
		member := &battleMember{c: m, self: battle.PlayerFighter(m.character, s.Assets.Items, i)}
		fighters = append(fighters, member.self)
		if pet := m.character.BattlePet(); pet != nil {
			copy := *pet
			copy.Skills = append([]game.PetSkill(nil), pet.Skills...)
			copy.Normalize(s.Assets.Items, false, s.petTemplate(copy.ID))
			copy.Name = s.companionName(copy.ID, copy.Name)
			member.pet = battle.PetFighter(copy, m.character.ID, s.Assets.Items, i, s.petTemplate(copy.ID))
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
	s.captureEscapeLeaders(run)
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
		if err := s.sendAll(m.c, pet.ProgressionPackets(m.c.pets.slot(pet.ID), s.Assets.Items, s.petTemplate(pet.ID))); err != nil {
			return err
		}
	}
	if m.c.party != nil {
		s.partyUpdate(m.c.party)
	}
	intro := run.b.Intro(m.self, background)
	if m.c.combatCapabilities&protocol.CombatCapabilityRoundChain != 0 {
		prompt, err := s.combatPrompt(m, intro[len(intro)-2:])
		if err != nil {
			return err
		}
		intro = append(intro[:len(intro)-2], prompt...)
	}
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
		ack := run.b.Submit(s.combatRules(run), c.character.ID, p[1], p[2:])
		if ack == nil {
			return nil
		}
		for _, member := range run.active() {
			s.sendOrClose(member.c, ack)
		}
		s.tryRound(run)
	case protocol.CommandBattleState:
		if len(p) == 3 && p[1] == protocol.BattleStateExit && p[2] == protocol.BattleExitRequest {
			m := run.member(c)
			if m == nil {
				return nil
			}
			actor := m.self
			if actor.Dead() {
				actor = m.pet
			}
			if actor == nil {
				return nil
			}
			return s.battleCommand(c, []byte{protocol.CommandBattleAction, protocol.BattleActionFleeRequest, actor.X, actor.Y, actor.X, actor.Y})
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
	s.captureEscapeLeaders(run)
	steps, outcome := run.b.Round(s.combatRules(run))
	if run.rewardError != nil {
		s.Log.Warn("combat acquisition not committed", "error", run.rewardError)
		for _, m := range run.active() {
			s.abandonBattle(m.c)
			m.c.conn.Close()
		}
		return
	}
	steps = append(run.petChanges, steps...)
	run.petChanges = nil
	go s.playRound(run, steps, outcome)
}

// play sends each step to every member and waits for its animation without
// holding worldMu. It stops when no member remains.
func (s *Server) play(run *battleRun, steps []battle.Step) bool {
	s.worldMu.Lock()
	if run.b.Finished {
		s.worldMu.Unlock()
		return false
	}
	escapes := s.prepareEscapingMembers(run, steps)
	prepared := s.prepareCombatRounds(run, steps)
	s.worldMu.Unlock()
	for index, step := range steps {
		s.worldMu.Lock()
		if run.b.Finished {
			s.worldMu.Unlock()
			return false
		}
		members := run.active()
		for _, m := range members {
			if m.roundBundled {
				continue
			}
			var progress [][]byte
			var err error
			if saved, ok := prepared[m.c]; ok {
				if index < len(saved) {
					progress = saved[index]
				}
			} else {
				progress, err = s.commitSkillUses(m.c, run, step.SkillUses)
			}
			if err != nil {
				s.Log.Warn("battle skill progress not committed", "character", m.c.character.ID, "error", err)
				s.abandonBattle(m.c)
				m.c.conn.Close()
				continue
			}
			if s.sendAll(m.c, append(progress, step.PacketsFor(m.c.character.ID)...)) != nil {
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
		if len(step.Escapes) > 0 {
			s.worldMu.Lock()
			for _, owner := range step.Escapes {
				if result, ok := escapes[owner]; ok {
					s.completeMemberEscape(run, result)
				}
			}
			if s.finishEmptyEscapeBattle(run) {
				s.worldMu.Unlock()
				return false
			}
			s.worldMu.Unlock()
		}
		if len(step.Overkills) > 0 {
			s.worldMu.Lock()
			if run.b.Finished {
				s.worldMu.Unlock()
				return false
			}
			for _, f := range step.Overkills {
				if f.Kind != battle.Player {
					continue
				}
				for _, m := range run.active() {
					if m.self != f {
						continue
					}
					if err := s.returnOverkilledPlayer(run, m, true); err != nil {
						s.Log.Warn("overkill return failed", "character", m.c.character.ID, "error", err)
						m.c.battle = nil
						run.b.Leave(m.c.character.ID)
						m.c.conn.Close()
					}
				}
			}
			alive := len(run.active()) > 0
			if !alive {
				run.b.Finished = true
				if run.timer != nil {
					run.timer.Stop()
				}
				if run.encounter != 0 {
					s.World.ReleaseEncounter(run.encounterMap, run.encounter, time.Now())
				}
			}
			s.worldMu.Unlock()
			if !alive {
				return false
			}
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
		gain := use.SkillEXP
		if gain == 0 {
			gain = game.CombatSkillEXPPerUse
		}
		gain = uint32(ScaleExperience(uint64(gain), s.expRateMultiplier))
		if use.Actor == m.self {
			before := append([]game.LearnedSkill(nil), next.Skills...)
			updates := next.AddSkillEXP(use.Skill, gain)
			changed = changed || len(updates) > 0
			packets = append(packets, updates...)
			for i, skill := range next.Skills {
				if skill.Grade > before[i].Grade {
					packets = append(packets, s.unlockQualifiedSkills(&next, true)...)
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
				packets = append(packets, next.Pets[i].AddSkillEXP(use.Skill, gain, c.pets.slot(next.Pets[i].ID))...)
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
	s.flushBattleDisconnects(run)
	if len(run.active()) == 0 || run.b.Finished {
		return
	}
	if outcome == battle.Continue {
		outcome = run.b.Forfeit()
	}
	if outcome != battle.Continue {
		s.endBattle(run, outcome)
		return
	}
	run.b.Processing = false
	for _, m := range run.active() {
		prompt, err := s.combatPrompt(m, run.b.NextRound(m.c.character.ID))
		if err != nil || s.sendAll(m.c, prompt) != nil {
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
	s.flushBattleDisconnects(run)
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
			if s.battleGoldEnabled && next.Gold < game.MaxGold {
				next.Gold = uint32(min(uint64(next.Gold)+gold, uint64(game.MaxGold)))
			}
			playerEXP := exp
			if s.battleEXPFormula == game.BattleEXPParticipation {
				playerEXP = ScaleExperience(run.b.ParticipantEXP(m.self), s.expRateMultiplier)
			}
			levels := next.AddExp(playerEXP)
			if levels > 0 {
				next.Refill(s.Assets.Items)
			}
			if j := battlePetIndex(next, m); j >= 0 {
				// Simple mode shares the owner's reward. Participation mode grants
				// each owned pet's accumulated kill share separately below.
				pet := &next.Pets[j]
				pet.HP = int32(max(1, m.pet.HP))
				petEXP := exp
				if s.battleEXPFormula == game.BattleEXPParticipation {
					petEXP = 0
				}
				grown := s.gainPetExp(pet, uint32(min(petEXP, 1<<31-1)), petGrowth)
				pet.Normalize(s.Assets.Items, grown > 0, s.petTemplate(pet.ID))
			}
			if s.battleEXPFormula == game.BattleEXPParticipation {
				for i := range next.Pets {
					pet := &next.Pets[i]
					share := run.b.PetParticipantEXP(next.ID, pet.ID)
					if share == 0 {
						continue
					}
					grown := s.gainPetExp(pet, uint32(ScaleExperience(share, s.expRateMultiplier)), petGrowth)
					pet.Normalize(s.Assets.Items, grown > 0, s.petTemplate(pet.ID))
					if slot := c.pets.slot(pet.ID); slot != 0 && (m.pet == nil || !game.SamePet(m.pet.ID, pet.ID)) {
						first = append(first, pet.ProgressionPackets(slot, s.Assets.Items, s.petTemplate(pet.ID))...)
					}
				}
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
		m.resultSaved = true
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
		if next < 0 && outcome == battle.Victory && c.character.Map == es.mapID && es.click > 0 {
			s.sendOrClose(c, s.World.HideActor(c.view, es.mapID, es.click))
		}
		_ = s.cancelInteraction(c)
		s.sendOrClose(c, s.World.Sync(c.character, c.view, false)...)
		return
	}
	_ = s.startEvent(context.Background(), c, es.click, es.ev, next, true)
}

// abandonBattle saves the resolved state and withdraws this owner and pet.
// The battle ends when no member remains. Caller holds worldMu.
func (s *Server) abandonBattle(c *Session) {
	run := c.battle
	if run == nil {
		return
	}
	m := run.member(c)
	if m != nil && m.self.Overkilled {
		// A disconnect during the flight must not bypass the saved return.
		// The round has already resolved; omit writes to the closing owner.
		if err := s.returnOverkilledPlayer(run, m, false); err != nil {
			s.Log.Warn("disconnected overkill return failed", "character", c.character.ID, "error", err)
		}
	} else if m != nil && !m.resultSaved {
		s.stageBattleDisconnect(run, m)
		ctx, cancel := context.WithTimeout(context.Background(), autosaveTimeout)
		if err := s.saveBattleDisconnect(ctx, c); err != nil {
			s.Log.Error("battle disconnect state not committed", "character", c.character.ID, "error", err)
		}
		cancel()
	}
	// The overkill path already publishes removal after committing its return.
	removed := c.battle != run
	c.battle = nil
	run.b.Leave(c.character.ID)
	c.encounter.battleOver()
	if m != nil && !removed {
		s.publishBattleDisconnect(run, m)
	}
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
		// A submitted round is immutable. In particular, an escaping teammate
		// must complete its departure before withdrawal-based endings are tested.
		if run.b.Processing {
			return
		}
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
	packets, peerPackets := s.activePetResults(run, m, next, outcome, roster)
	for _, catch := range run.b.Captures {
		if catch.Committed {
			continue
		}
		if catch.Owner != next.ID {
			continue
		}
		slot := next.FreePetSlot()
		if slot == 0 {
			continue
		}
		t := s.petTemplate(catch.Template)
		pet := game.NewCapturedPet(catch.Template, catch.Name, slot, t, s.Assets.Items, petGrowth, game.PetGrowthOptions{Formula: s.petGrowthFormula, Items: s.Assets.Items})
		pet.EnsureSkills(t, s.hasSkill)
		pet.Normalize(s.Assets.Items, true, s.petTemplate(pet.ID))
		next.Pets = append(next.Pets, pet)
		if roster.register(pet.ID) {
			packets = append(packets, pet.RecruitPacket(next.ID, t))
		}
		packets = append(packets, pet.ProgressionPackets(roster.slot(pet.ID), s.Assets.Items, s.petTemplate(pet.ID))...)
		if pet.Equipment != (game.Equipment{}) {
			packets = append(packets, s.petListPacket(next, roster))
		}
		packets = append(packets, serverwire.SystemLine(fmt.Sprintf("Successfully captured %s into Pet Slot #%d!", catch.Name, slot)))
	}
	return packets, peerPackets
}

// activePetResults checkpoints only the outgoing pet; captures settle once at battle end.
func (s *Server) activePetResults(run *battleRun, m *battleMember, next *game.Character, outcome battle.Outcome, roster *petRoster) ([][]byte, [][]byte) {
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
			packets = append(packets, game.PetStat(slot, game.PetStatAmity, int64(pet.Amity)))
		}
		if pet.Amity < game.PetDesertMinimumAmity && deaths > 0 {
			// The pet deserts the party.
			id := pet.ID
			if next.ActiveMount != 0 && game.SamePet(next.ActiveMount, id) {
				next.ActiveMount = 0
				packets = append(packets, serverwire.UnmountPackets(next.ID)...)
				peerPackets = append(peerPackets, serverwire.UnmountPackets(next.ID)...)
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
			pet.Normalize(s.Assets.Items, false, s.petTemplate(pet.ID))
			packets = append(packets, pet.ProgressionPackets(slot, s.Assets.Items, s.petTemplate(pet.ID))...)
		}
	}
	return packets, peerPackets
}

// battleStateCommand decodes the native AC11:2 player PK request before changing
// either party. raw character ID is authoritative; the trailing click ID is NPC-only.
func (s *Server) battleStateCommand(c *Session, p []byte) error {
	if len(p) < 2 {
		return ErrUnsupported
	}
	if c.battle != nil {
		return s.battleCommand(c, p)
	}
	if p[1] != protocol.BattleStateChallenge {
		return nil
	}
	if len(p) != 9 {
		return protocol.ErrMalformed
	}
	if p[2] != protocol.BattleChallengePlayer {
		return nil
	}
	targetID := uint32(p[3]) | uint32(p[4])<<8 | uint32(p[5])<<16 | uint32(p[6])<<24
	target := s.friendSessions[targetID]
	if target == nil || target == c || target.character == nil || !s.samePlayerScene(target, c) || c.tentOwner != 0 ||
		!c.character.Preferences().PKAllowed || !target.character.Preferences().PKAllowed ||
		!gmIdle(c) || !gmIdle(target) || (c.party != nil && c.party == target.party) {
		return c.send(serverwire.HeadBanner("The player is unavailable for PK."))
	}
	return s.startPvP(c, target)
}

// startPvP initializes two parties through the same turn/persistence pipeline as PvE.
// Caller holds worldMu, so no participant can join two battles during setup.
func (s *Server) startPvP(attacker, defender *Session) error {
	run := &battleRun{}
	teams := [][]*Session{s.battleTeam(attacker), s.battleTeam(defender)}
	seen := map[*Session]bool{}
	for _, team := range teams {
		for _, c := range team {
			if seen[c] || !gmIdle(c) || !c.character.Preferences().PKAllowed {
				return attacker.send(serverwire.HeadBanner("A party member is unavailable for PK."))
			}
			seen[c] = true
		}
	}
	var fighters [2][]*battle.Fighter
	for side, team := range teams {
		for i, c := range team {
			m := &battleMember{c: c, self: battle.PlayerFighter(c.character, s.Assets.Items, i)}
			battle.Place(m.self, battle.Side(side+1), i)
			fighters[side] = append(fighters[side], m.self)
			if pet := c.character.BattlePet(); pet != nil {
				copy := *pet
				copy.Skills = append([]game.PetSkill(nil), pet.Skills...)
				copy.Normalize(s.Assets.Items, false, s.petTemplate(copy.ID))
				copy.Name = s.companionName(copy.ID, copy.Name)
				m.pet = battle.PetFighter(copy, c.character.ID, s.Assets.Items, i, s.petTemplate(copy.ID))
				battle.Place(m.pet, battle.Side(side+1), i)
				fighters[side] = append(fighters[side], m.pet)
			}
			run.members = append(run.members, m)
		}
	}
	run.b = battle.NewPvP(fighters[0], fighters[1])
	for _, m := range run.members {
		m.c.battle = run
	}
	background := uint16(1)
	if attacker.character.Map < game.MapID10000 {
		background = attacker.character.Map
	}
	for _, m := range run.members {
		if err := s.battleIntro(run, m, background); err != nil {
			run.b.Finished = true
			for _, member := range run.members {
				member.c.battle = nil
				member.c.conn.Close()
			}
			return err
		}
	}
	s.armTurnTimer(run)
	return nil
}
