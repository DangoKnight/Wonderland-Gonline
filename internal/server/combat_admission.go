package server

import (
	"sort"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
)

const battleEntryDistance = 170 // Native per-axis distance < 0xab.

type battleAdmission struct {
	target       *battleMember
	spectator    bool
	order        uint64
	commandPhase bool
}

func (s *Server) requestBattleEntry(c *Session, p []byte) error {
	id := uint32(p[3]) | uint32(p[4])<<8 | uint32(p[5])<<16 | uint32(p[6])<<24
	target := s.friendSessions[id]
	reject := func() error { return c.send(serverwire.HeadBanner("This battle is unavailable.")) }
	if p[7] != 0 || p[8] != 0 || target == nil || target == c || target.character == nil || !battleInteractionAvailable(c, nil) || !gmIdle(c) || !s.samePlayerScene(c, target) || c.tentOwner != 0 || c.fishing != nil || c.gathering != nil || c.stall != nil {
		return reject()
	}
	if c.party != nil && c.party.leader() != c {
		return reject()
	}
	if target.party != nil {
		target = target.party.leader()
	}
	if target == nil || target.battle == nil || !s.samePlayerScene(c, target) {
		return reject()
	}
	run := target.battle
	m := run.member(target)
	if m == nil || run.b.Finished || m.self.Withdrawn || m.self.Overkilled || run.trial != nil || run.member(c) != nil {
		return reject()
	}
	dx, dy := int(c.character.X)-int(target.character.X), int(c.character.Y)-int(target.character.Y)
	if dx < -battleEntryDistance || dx > battleEntryDistance || dy < -battleEntryDistance || dy > battleEntryDistance {
		return reject()
	}
	watch := p[2] == protocol.BattleWatchPlayer
	space := len(run.b.AdmissionSlots(m.self.Side))
	for _, pending := range run.admissions {
		if !pending.spectator && pending.target.self.Side == m.self.Side {
			space--
		}
	}
	if !watch && (c.character.HP == 0 || !target.character.Preferences().JoinAllowed || space <= 0) {
		return reject()
	}
	s.cancelPartyLeadership(c.party, protocol.TeamLeadershipBusy)
	c.battle = run
	run.admissionOrder++
	a := battleAdmission{target: m, spectator: watch, order: run.admissionOrder}
	if run.b.Processing {
		if run.admissions == nil {
			run.admissions = map[*Session]battleAdmission{}
		}
		run.admissions[c] = a
		return c.send([]byte{protocol.CommandMovement, protocol.MovementMovementLock, 1})
	}
	return s.enterBattle(c, run, a)
}

func (s *Server) enterBattle(c *Session, run *battleRun, a battleAdmission) error {
	if c.battle != run {
		return nil
	}
	if !battleInteractionAvailable(c, nil) || a.target.c.battle != run || run.b.Finished || (!a.spectator && !a.target.c.character.Preferences().JoinAllowed) {
		s.leaveBattleObserver(c)
		return nil
	}
	self := battle.PlayerFighter(c.character, s.Assets.Items, 0)
	self.Side = a.target.self.Side
	if a.spectator {
		self.X, self.Y = protocol.BattleSpectatorCoordinate, protocol.BattleSpectatorCoordinate
		if run.spectators == nil {
			run.spectators = map[*Session]bool{}
		}
		run.spectators[c] = true
	} else {
		slot := run.b.AdmissionSlot(self.Side)
		if slot < 0 {
			s.leaveBattleObserver(c)
			return nil
		}
		battle.Place(self, self.Side, slot)
		m := &battleMember{c: c, self: self, joined: true}
		if pet := c.character.BattlePet(); pet != nil {
			copy := *pet
			copy.Skills = append([]game.PetSkill(nil), pet.Skills...)
			copy.Normalize(s.Assets.Items, false, s.petTemplate(copy.ID))
			m.pet = battle.PetFighter(copy, c.character.ID, s.Assets.Items, slot, s.petTemplate(copy.ID))
			battle.Place(m.pet, self.Side, slot)
		}
		run.b.Admit(m.self, m.pet)
		// Mode 5 enters the scene now, but the native client waits for the
		// next AC52:1. Reserve idle commands until that phase begins.
		if !a.commandPhase {
			m.self.Joining = true
			run.b.Idle(m.self)
			if m.pet != nil {
				m.pet.Joining = true
				run.b.Idle(m.pet)
			}
		}
		run.members = append(run.members, m)
		for _, pet := range c.character.Pets {
			run.b.Roster[c.character.ID] = append(run.b.Roster[c.character.ID], pet.ID)
		}
		for _, other := range run.active() {
			if other.c == c {
				continue
			}
			s.sendOrClose(other.c, battle.ParticipantPacket(self, protocol.BattlePresenceJoined))
			if m.pet != nil {
				s.sendOrClose(other.c, battle.ParticipantPacket(m.pet, protocol.BattleParticipantWaitNextTurn))
			}
		}
		s.sendBattleSpectators(run, battle.ParticipantPacket(self, protocol.BattlePresenceJoined))
		if m.pet != nil {
			s.sendBattleSpectators(run, battle.ParticipantPacket(m.pet, protocol.BattleParticipantWaitNextTurn))
		}
		s.captureEscapeLeaders(run)
	}
	background := s.battleBackground(run.members[0].c, run)
	if c.clientBackdrops {
		s.sendOrClose(c, run.backdropLocation.Packet())
		background = compatibilityBattleBackground
	}
	packets := run.b.EntrySnapshot(self, background, a.spectator)
	err := s.sendAll(c, packets)
	state := battlePresenceState(c)
	s.broadcastWorld(c, state)
	s.sendOrClose(c, state)
	return err
}

func (s *Server) admitWaitingBattleClients(run *battleRun) {
	pending := run.admissions
	run.admissions = nil
	var clients []*Session
	for c := range pending {
		clients = append(clients, c)
	}
	sort.Slice(clients, func(i, j int) bool { return pending[clients[i]].order < pending[clients[j]].order })
	for _, c := range clients {
		a := pending[c]
		a.commandPhase = true
		if err := s.enterBattle(c, run, a); err != nil {
			c.conn.Close()
		}
	}
}

func (s *Server) sendBattleSpectators(run *battleRun, packets ...[]byte) {
	for c := range run.spectators {
		if c.battle == run {
			s.sendOrClose(c, packets...)
		}
	}
}

func battlePresenceState(c *Session) []byte {
	if c.battle != nil && c.battle.spectators[c] {
		return protocol.Builder{protocol.CommandBattleState, protocol.BattleStateParticipation, 2}.U32(c.character.ID).U16(0).U8(protocol.BattlePresenceSpectator)
	}
	if c.battle != nil {
		if m := c.battle.member(c); m != nil && m.joined {
			return protocol.Builder{protocol.CommandBattleState, protocol.BattleStateParticipation, byte(battle.Player)}.U32(c.character.ID).U16(0).U8(protocol.BattlePresenceJoined)
		}
	}
	return battleState(c.character.ID, c.battle != nil)
}

func (s *Server) leaveBattleObserver(c *Session) {
	run := c.battle
	if run == nil || run.member(c) != nil {
		return
	}
	delete(run.spectators, c)
	delete(run.spectatorBundled, c)
	delete(run.admissions, c)
	c.battle = nil
	p := protocol.Builder{protocol.CommandBattleState, protocol.BattleStateWireCode0}.U32(c.character.ID).U16(0)
	s.sendOrClose(c, p, []byte{protocol.CommandMovement, protocol.MovementMovementLock, 0}, []byte{protocol.CommandEvent, protocol.EventResume})
	s.broadcastWorld(c, battleState(c.character.ID, false))
}

func (s *Server) closeBattleObservers(run *battleRun, departure ...[]byte) {
	s.sendBattleSpectators(run, departure...)
	for c := range run.spectators {
		s.leaveBattleObserver(c)
	}
	for c := range run.admissions {
		s.leaveBattleObserver(c)
	}
}
