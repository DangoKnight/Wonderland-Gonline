package battle

import "wonderland-gonline/internal/protocol"

const (
	fleeFailureSkill          = 60042 // Native MBTM: run, trip (pose 64), then recover.
	fleeBaseChancePercent     = 75
	fleeEnemyLevelsPerPenalty = 6
	fleeRollOutOf             = 100
	fleeFailureTargetBytes    = 5 // Cell, result, guard and zero stat-result count.
)

// canEscape ports Battle.cs CanFlee/Succuss_miss, against the strongest living
// opponent. Equal/higher level succeeds without spending a random roll.
func (b *Battle) canEscape(r Rules, actor *Fighter) bool {
	_, enemies := b.Teams(actor.Side)
	level := 0
	for _, f := range living(enemies) {
		level = max(level, int(f.Level))
	}
	if int(actor.Level) >= level {
		return true
	}
	chance := fleeBaseChancePercent - level/fleeEnemyLevelsPerPenalty
	// The fractional reference roll cannot cross this integer threshold.
	return r.Next(0, fleeRollOutOf) >= fleeRollOutOf-chance
}

func fighterOwner(f *Fighter) uint32 {
	if f.Kind == Pet {
		return f.Owner
	}
	return f.ID
}

func (b *Battle) resolveEscapes(r Rules, actions []Action, steps []Step) ([]Step, bool) {
	for _, a := range actions {
		if a.Kind != "flee" || a.Actor == nil || a.Actor.Dead() || !a.Actor.CanAct() || a.Actor.Absent {
			continue
		}
		if !b.canEscape(r, a.Actor) {
			// Pure self animation: no HP/SP change, defense bonus or proficiency.
			p := protocol.Builder{protocol.CommandBattleAction, protocol.BattleActionAnimation}.U16(animationActorHeaderBytes + fleeFailureTargetBytes).
				U8(a.Actor.X).U8(a.Actor.Y).U16(fleeFailureSkill).U8(0).U8(1).
				U8(a.Actor.X).U8(a.Actor.Y).U8(protocol.BattleHitLanded).U8(0).U8(0)
			steps = append(steps, Step{Packets: [][]byte{turnPacket(a.Actor), p}, Delay: r.Delay(fleeFailureSkill, "flee_failure")})
			continue
		}
		owner := fighterOwner(a.Actor)
		allies, _ := b.Teams(a.Actor.Side)
		remaining := false
		for _, f := range allies {
			remaining = remaining || (f.Kind == Player || f.Kind == Pet) && fighterOwner(f) != owner && !f.Absent && !f.Withdrawn && !f.Overkilled
		}
		if b.EscapeLeaders[owner] || !remaining {
			return steps, true
		}
		step := Step{Delay: r.Delay(0, "flee"), Escapes: []uint32{owner}}
		// Both identities leave, including a knocked-out owner commanded by its pet.
		for _, kind := range []Kind{Pet, Player} {
			for _, f := range allies {
				if f.Kind == kind && fighterOwner(f) == owner {
					f.Withdrawn = true
					step.Packets = append(step.Packets, []byte{protocol.CommandBattleState, protocol.BattleStateExit, f.X, f.Y})
				}
			}
		}
		steps = append(steps, step)
	}
	return steps, false
}
