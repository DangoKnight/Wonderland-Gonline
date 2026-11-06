package battle

import (
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const reinforcementDelay = 400 * time.Millisecond

func (b *Battle) HasReinforcements() bool {
	for _, queue := range b.Reinforcements {
		if len(queue) > 0 {
			return true
		}
	}
	return false
}

// DefenderHistory retains replaced monsters for victory EXP/loot exactly once.
// Active targeting uses only Defenders, so a defeated wave cannot be targeted.
func (b *Battle) DefenderHistory() []*Fighter {
	return append(append([]*Fighter(nil), b.defeatedDefenders...), b.Defenders...)
}

func (b *Battle) DiscardReinforcements() { b.Reinforcements = [MaxEnemies][]Enemy{} }

// Reinforcements enter after the round's actions/status tick. Fresh monsters do
// not inherit the defeated occupant's status effects or act in the same round.
func (b *Battle) spawnReinforcements() [][]byte {
	if b.PvP {
		return nil
	}
	var packets [][]byte
	for slot, dead := range b.Defenders {
		if slot >= MaxEnemies || !dead.Dead() || len(b.Reinforcements[slot]) == 0 {
			continue
		}
		e := b.Reinforcements[slot][0]
		b.Reinforcements[slot] = b.Reinforcements[slot][1:]
		next := enemyFighter(e, slot)
		next.X, next.Y, next.ClickID = dead.X, dead.Y, dead.ClickID
		b.defeatedDefenders = append(b.defeatedDefenders, dead)
		b.Defenders[slot] = next
		packets = append(packets,
			record(protocol.Builder{protocol.CommandBattleState, protocol.BattleStateParticipant}, byte(next.Side), next, byte(next.Kind), next.ClickID, 0),
			StatSync(next.X, next.Y, game.StatCurrentHP, uint32(next.HP)),
			StatSync(next.X, next.Y, game.StatCurrentSP, uint32(next.SP)),
		)
	}
	return packets
}
