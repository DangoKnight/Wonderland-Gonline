package battle

import "wonderland-gonline/internal/game"

// The acquisition step follows the strike's delay, before overkill removal or
// the next actor. Empty/no-drop results do not add work to either transport.
func (r Rules) appendKillReward(b *Battle, steps []Step, target *Fighter, actors []Action) []Step {
	if !target.Dead() {
		return steps
	}
	if r.ExperienceFormula == game.BattleEXPParticipation {
		b.recordKillExperience(target, actors)
	}
	if r.RewardKill == nil {
		return steps
	}
	step := r.RewardKill(target, actors)
	if len(step.Packets) == 0 && len(step.OwnerPackets) == 0 {
		return steps
	}
	return append(steps, step)
}
