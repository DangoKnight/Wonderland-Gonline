package server

import (
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/protocol"
)

// Only common combat packets are public to observers. Owner receipts, item
// consumption, skill progression and rewards never enter their round envelope.
func (s *Server) prepareSpectatorRounds(run *battleRun, steps []battle.Step) {
	run.spectatorBundled = map[*Session]bool{}
	for c := range run.spectators {
		if c.battle != run || c.combatCapabilities&protocol.CombatCapabilityRoundChain == 0 || len(steps) == 0 || len(steps) > protocol.CombatMaxRoundSteps {
			continue
		}
		var chain []protocol.CombatStep
		for _, step := range steps {
			chain = append(chain, protocol.CombatStep{Packets: step.Packets, Delay: step.Delay})
		}
		data, err := protocol.EncodeCombatRound(chain)
		if err != nil {
			continue
		} // Preserve the native transport fallback.
		id, err := c.nextCombatSequence()
		if err != nil {
			c.conn.Close()
			continue
		}
		chunks, err := protocol.CombatRoundChunks(id, data)
		if err != nil {
			continue
		}
		if err := s.sendAll(c, chunks); err != nil {
			c.conn.Close()
			continue
		}
		run.spectatorBundled[c] = true
	}
}

func (s *Server) sendBattleSpectatorStep(run *battleRun, packets ...[]byte) {
	for c := range run.spectators {
		if c.battle == run && !run.spectatorBundled[c] {
			s.sendOrClose(c, packets...)
		}
	}
}
