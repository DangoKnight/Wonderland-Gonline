package server

import (
	"maps"
	"math"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/protocol"
)

// Capability negotiation changes transport only. Both paths use the same rules,
// ownership checks, native result packets and durable settlement.
func (s *Server) acceptCombatProtocol(c *Session, p []byte) error {
	flags, ok := protocol.ParseCombatCapabilities(p, protocol.HandshakeCombatAccept)
	if !ok || flags & ^protocol.CombatCapabilityRoundChain != 0 {
		return protocol.ErrMalformed
	}
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if !s.combatRoundChain || !c.combatOffered || c.battle != nil {
		return protocol.ErrMalformed
	}
	if err := c.send(protocol.CombatCapabilities(protocol.HandshakeCombatEnabled, flags)); err != nil {
		return err
	}
	c.combatCapabilities = flags
	return nil
}

func (c *Session) nextCombatSequence() (uint32, error) {
	if c.combatSequence == math.MaxUint32 {
		return 0, protocol.ErrMalformed
	}
	c.combatSequence++
	return c.combatSequence, nil
}

func (s *Server) combatPrompt(m *battleMember, native [][]byte) ([][]byte, error) {
	m.roundBundled = false
	if m.c.combatCapabilities&protocol.CombatCapabilityRoundChain == 0 || len(native) == 0 {
		return native, nil
	}
	token, err := m.c.nextCombatSequence()
	if err != nil {
		return nil, err
	}
	m.choiceToken = token
	// Preserve the engine's choice of living character or pet as the prompt.
	p := native[0]
	return [][]byte{protocol.Builder{protocol.CommandGonlineCombat, protocol.CombatRoundPrompt}.U32(token).U8(p[2]).U8(p[3])}, nil
}

// A batch is all choices for this owner, applied atomically before any ack or
// round execution. Old/repeated tokens have no effect on subsequent rounds.
func (s *Server) combatChoicesCommand(c *Session, p []byte) error {
	if c.combatCapabilities&protocol.CombatCapabilityRoundChain == 0 {
		return ErrUnsupported
	}
	token, choices, err := protocol.DecodeCombatChoices(p)
	if err != nil {
		return err
	}
	run := c.battle
	m := run.member(c)
	if m == nil || token != m.choiceToken || run.b.Processing || run.b.Finished {
		return nil
	}
	want := 0
	for _, f := range []*battle.Fighter{m.self, m.pet} {
		if f != nil && !f.Dead() {
			want++
		}
	}
	if len(choices) != want {
		return protocol.ErrMalformed
	}
	original := run.b.Pending
	run.b.Pending = maps.Clone(original)
	var acknowledgments [][]byte
	rules := s.combatRules(run)
	for _, choice := range choices {
		ack := run.b.Submit(rules, c.character.ID, choice[1], choice[2:])
		if ack == nil {
			run.b.Pending = original
			return protocol.ErrMalformed
		}
		acknowledgments = append(acknowledgments, ack)
	}
	m.choiceToken = 0
	for _, other := range run.active() {
		s.sendOrClose(other.c, acknowledgments...)
	}
	s.tryRound(run)
	return nil
}

// prepareCombatRounds runs under worldMu. Skill progress is committed before
// its success packets enter the complete chain. A bounded oversized chain can
// fall back to native delivery without awarding progress a second time.
func (s *Server) prepareCombatRounds(run *battleRun, steps []battle.Step) map[*Session][][][]byte {
	prepared := map[*Session][][][]byte{}
	for _, m := range run.active() {
		m.roundBundled = false
		if m.c.combatCapabilities&protocol.CombatCapabilityRoundChain == 0 || len(steps) == 0 || len(steps) > protocol.CombatMaxRoundSteps/2 {
			continue
		}
		var chain []protocol.CombatStep
		progress := make([][][]byte, len(steps))
		var uses []battle.Action
		for _, step := range steps {
			uses = append(uses, step.SkillUses...)
			chain = append(chain, protocol.CombatStep{Packets: step.PacketsFor(m.c.character.ID), Delay: step.Delay})
			ownExit := false
			var withdrawals [][]byte
			for _, owner := range step.Escapes {
				if owner == m.self.ID {
					ownExit = true
				} else {
					withdrawals = append(withdrawals, protocol.Builder{protocol.CommandBattleState, protocol.BattleStateWireCode0}.U32(owner).U16(0))
				}
			}
			for _, f := range step.Overkills {
				if f.Kind != battle.Player {
					continue
				}
				if f == m.self {
					ownExit = true
					continue
				}
				for _, owner := range run.members {
					if owner.self != f {
						continue
					}
					if owner.pet != nil && !owner.pet.Overkilled {
						withdrawals = append(withdrawals, []byte{protocol.CommandBattleState, protocol.BattleStateExit, owner.pet.X, owner.pet.Y, 0})
					}
					withdrawals = append(withdrawals, protocol.Builder{protocol.CommandBattleState, protocol.BattleStateWireCode0}.U32(f.ID).U16(0))
				}
			}
			if ownExit {
				break
			}
			if len(withdrawals) > 0 {
				chain = append(chain, protocol.CombatStep{Packets: withdrawals})
			}
		}
		// One save per owner for all of its executed proficiency gains. Reward
		// settlement remains separate and follows the authoritative timeline.
		packets, err := s.commitSkillUses(m.c, run, uses)
		if err != nil {
			s.Log.Warn("round skill progress not committed", "character", m.c.character.ID, "error", err)
			s.abandonBattle(m.c)
			m.c.conn.Close()
			continue
		}
		progress[0] = packets
		chain[0].Packets = append(append([][]byte(nil), packets...), chain[0].Packets...)
		prepared[m.c] = progress
		data, err := protocol.EncodeCombatRound(chain)
		if err != nil {
			continue
		}
		id, err := m.c.nextCombatSequence()
		if err == nil {
			var chunks [][]byte
			chunks, err = protocol.CombatRoundChunks(id, data)
			if err == nil {
				err = s.sendAll(m.c, chunks)
			}
		}
		if err != nil {
			s.abandonBattle(m.c)
			m.c.conn.Close()
			continue
		}
		m.roundBundled = true
	}
	return prepared
}
