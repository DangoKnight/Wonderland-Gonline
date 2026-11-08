package server

import (
	"context"
	"time"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/protocol"
)

func (s *Server) finishEmptyEscapeBattle(run *battleRun) bool {
	if len(run.active()) != 0 {
		return false
	}
	run.b.Finished = true
	s.closeBattleObservers(run)
	if run.timer != nil {
		run.timer.Stop()
	}
	if run.encounter != 0 {
		s.World.ReleaseEncounter(run.encounterMap, run.encounter, time.Now())
	}
	return true
}

func (s *Server) captureEscapeLeaders(run *battleRun) {
	run.b.EscapeLeaders = map[uint32]bool{}
	for _, m := range run.active() {
		if !m.joined && (m.c.party == nil || m.c.party.leader() == m.c) {
			run.b.EscapeLeaders[m.self.ID] = true
		}
	}
}

// Commit escape vitals/pet results before native success packets or a complete
// round envelope are published. Only the escaping owner receives these receipts.
// Caller holds worldMu; steps is the private round playback slice.
func (s *Server) prepareEscapingMembers(run *battleRun, steps []battle.Step) map[uint32]memberResult {
	results := map[uint32]memberResult{}
	for i := range steps {
		for _, owner := range steps[i].Escapes {
			for _, m := range run.active() {
				if m.self.ID != owner {
					continue
				}
				c := m.c
				next := c.character.Clone()
				next.HP, next.SP = uint32(max(0, m.self.HP)), uint32(max(0, m.self.SP))
				next.HP, next.SP = min(next.HP, next.MaxHP), min(next.SP, next.MaxSP)
				roster := newPetRoster()
				roster.synced = c.pets.synced
				for id, slot := range c.pets.slots {
					roster.slots[id] = slot
				}
				packets, peers := s.petResults(run, m, &next, battle.Fled, roster)
				if err := s.commit(context.Background(), c, next); err != nil {
					s.Log.Warn("escape result not committed", "character", owner, "error", err)
					steps[i].Packets, steps[i].Escapes = nil, nil
					s.abandonBattle(c)
					c.conn.Close()
					// Observers still discard the withdrawn identities. This is
					// disconnect cleanup, not an owner success receipt or leader escape.
					var cleanup [][]byte
					if m.pet != nil {
						cleanup = append(cleanup, []byte{protocol.CommandBattleState, protocol.BattleStateExit, m.pet.X, m.pet.Y})
					}
					cleanup = append(cleanup, []byte{protocol.CommandBattleState, protocol.BattleStateExit, m.self.X, m.self.Y}, protocol.Builder{protocol.CommandBattleState, protocol.BattleStateWireCode0}.U32(owner).U16(0))
					for _, other := range run.active() {
						s.sendOrClose(other.c, cleanup...)
					}
					continue
				}
				c.pets, m.self.Char = roster, c.character
				m.resultSaved = true
				if steps[i].OwnerPackets == nil {
					steps[i].OwnerPackets = map[uint32][][]byte{}
				}
				steps[i].OwnerPackets[owner] = append(steps[i].OwnerPackets[owner], packets...)
				for _, p := range peers {
					s.broadcastWorld(c, p)
				}
				results[owner] = memberResult{m: m}
			}
		}
	}
	return results
}

// Complete only this explicit member escape after its departure animation.
// Leader escapes use endBattle instead; overkill/disconnect never call this.
func (s *Server) completeMemberEscape(run *battleRun, result memberResult) {
	m, c := result.m, result.m.c
	if c.battle != run {
		return
	}
	c.battle = nil
	run.b.Leave(m.self.ID)
	c.encounter.battleOver()
	closePacket := protocol.Builder{protocol.CommandBattleState, protocol.BattleStateWireCode0}.U32(m.self.ID).U16(0)
	s.sendBattleSpectators(run, closePacket)
	for _, other := range run.active() {
		if !other.roundBundled {
			s.sendOrClose(other.c, closePacket)
		}
	}
	state := battleState(m.self.ID, false)
	packets := [][]byte{closePacket, {protocol.CommandMovement, protocol.MovementMovementLock, 0}, {protocol.CommandEvent, protocol.EventResume}, state}
	packets = append(packets, c.character.StatPackets(s.Assets.Items)...)
	packets = append(packets, s.World.Sync(c.character, c.view, false)...)
	s.sendOrClose(c, packets...)
	s.broadcastWorld(c, state)
	if c.event != nil {
		s.endEvent(c)
	}
}
