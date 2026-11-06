package server

import (
	"context"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

type battleDisconnectSave struct {
	next   game.Character
	roster *petRoster
	member *battleMember
	peers  [][]byte
}

// Both transports calculate a complete round before playback. Its resulting
// vitals are authoritative even when a client disconnects before a damage cue.
// Only active-pet consequences settle here: no victory loot or pending captures.
func (s *Server) stageBattleDisconnect(run *battleRun, m *battleMember) {
	c := m.c
	if c.battleDisconnect != nil {
		return
	}
	next := c.character.Clone()
	next.HP = min(next.MaxHP, uint32(max(0, m.self.HP)))
	next.SP = min(next.MaxSP, uint32(max(0, m.self.SP)))
	roster := newPetRoster()
	roster.synced = c.pets.synced
	for id, slot := range c.pets.slots {
		roster.slots[id] = slot
	}
	_, peers := s.activePetResults(run, m, &next, battle.Continue, roster)
	c.battleDisconnect = &battleDisconnectSave{next: next, roster: roster, member: m, peers: peers}
}

// Failed saves keep their immutable snapshot for the final logout checkpoint.
// Do not publish pet desertion or advance the baseline before SQL succeeds.
func (s *Server) saveBattleDisconnect(ctx context.Context, c *Session) error {
	result := c.battleDisconnect
	if result == nil {
		return nil
	}
	if err := s.commitState(ctx, c, result.next); err != nil {
		return err
	}
	c.pets = result.roster
	result.member.resultSaved = true
	c.battleDisconnect = nil
	for _, p := range result.peers {
		s.broadcastWorld(c, p)
	}
	return nil
}

// Both transports finish the immutable round before removing its visual actors.
// Server eligibility changes immediately; no future round can target them.
func (s *Server) publishBattleDisconnect(run *battleRun, m *battleMember) {
	var packets [][]byte
	if m.pet != nil {
		packets = append(packets, []byte{protocol.CommandBattleState, protocol.BattleStateExit, m.pet.X, m.pet.Y, 0})
	}
	packets = append(packets, []byte{protocol.CommandBattleState, protocol.BattleStateExit, m.self.X, m.self.Y, 0},
		protocol.Builder{protocol.CommandBattleState, protocol.BattleStateWireCode0}.U32(m.self.ID).U16(0), battleState(m.self.ID, false))
	if run.b.Processing && !run.b.Finished {
		run.withdrawals = append(run.withdrawals, packets...)
		return
	}
	for _, other := range run.active() {
		s.sendOrClose(other.c, packets...)
	}
}

func (s *Server) flushBattleDisconnects(run *battleRun) {
	packets := run.withdrawals
	run.withdrawals = nil
	for _, other := range run.active() {
		s.sendOrClose(other.c, packets...)
	}
}
