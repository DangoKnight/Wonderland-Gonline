package battle

import (
	"slices"
	"time"
	"wonderland-gonline/internal/protocol"
)

// The reference server's first attack followed its switch batch by 342 ms.
// Use a 350 ms compatibility floor; entry mode 5, rather than this delay, keeps
// the replacement unavailable for the remainder of the switching round.
const petSwitchDelay = 350 * time.Millisecond

func (b *Battle) HasCommand(f *Fighter) bool {
	if f == nil {
		return false
	}
	_, ok := b.Pending[f.key()]
	return ok
}

// ReplacePet is called after the server validates and saves a Team request.
// Switching consumes both commands; neither fighter gains a defense bonus.
// The outgoing pet's selection is discarded, even if it was selected first.
func (b *Battle) ReplacePet(owner, old, next *Fighter) []Step {
	team := &b.Attackers
	if owner.Side == Defender {
		team = &b.Defenders
	}
	var steps []Step
	if old != nil {
		delete(b.Pending, old.key())
		*team = slices.DeleteFunc(*team, func(f *Fighter) bool { return f == old })
		steps = append(steps, Step{Packets: [][]byte{{protocol.CommandBattleState, protocol.BattleStateExit, old.X, old.Y}}})
	}
	b.Idle(owner)
	if next != nil {
		next.reviveSupport = b.reviveSupport[next.experienceID()]
		next.Side, next.Y = owner.Side, owner.Y
		next.X = owner.X - 1
		if owner.Side == Defender {
			next.X = owner.X + 1
		}
		*team = append(*team, next)
		next.Joining = true
		b.Idle(next)
		steps = append(steps, Step{Packets: [][]byte{
			record(protocol.Builder{protocol.CommandBattleState, protocol.BattleStateParticipant}, protocol.BattleParticipantWaitNextTurn, next, byte(Pet), next.ClickID, owner.ID),
		}, Delay: petSwitchDelay})
	} else if len(steps) > 0 {
		steps[len(steps)-1].Delay = petSwitchDelay
	}
	return steps
}
