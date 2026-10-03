package battle

const (
	// Adjacent participants, rather than the fastest and slowest, must satisfy
	// this limit. See docs/COMBOS.md for the saved community references.
	comboMaxSpeedGap        = 99
	comboGuaranteedLevelGap = 25
	comboRebirthLevelBonus  = 99
	comboEqualLevelChance   = 0.5
	// Shared damage bonus for every participant in an eligible combo.
	comboDamageMultiplier = 1.3
)

func (a Action) scheduledSpeed() int {
	if a.speedRecorded {
		return a.comboSpeed
	}
	return a.Actor.speed()
}

func (r Rules) comboEligible(a Action) bool {
	if a.Kind != "attack" || a.Actor == nil || a.Actor.Absent || !r.CanUse(a.Actor, a.Skill) {
		return false
	}
	if Basic(a.Skill) {
		return true
	}
	grade := 1
	if a.Actor.Pet != nil {
		if skill, ok := petLearned(a.Actor.Pet, a.Skill); ok {
			grade = int(skill.Grade)
		}
	} else if a.Actor.Char != nil {
		if skill, ok := learned(a.Actor.Char, a.Skill); ok {
			grade = int(skill.Grade)
		}
	}
	return !r.Skills[a.Skill].IsAreaAttack(grade)
}

// comboGroups also runs immediately before damage, after unavailable casters
// and redirected targets have been removed. A missing bridge cannot grant a
// combo to the remaining attackers. The caller supplies descending speed order.
func (r Rules) comboGroups(actions []Action) [][]Action {
	var groups [][]Action
	for _, a := range actions {
		if len(groups) > 0 {
			last := groups[len(groups)-1]
			previous := last[len(last)-1]
			gap := previous.scheduledSpeed() - a.scheduledSpeed()
			if r.comboEligible(previous) && r.comboEligible(a) &&
				previous.Actor.Side == a.Actor.Side && previous.Redirected == a.Redirected &&
				previous.TX == a.TX && previous.TY == a.TY &&
				gap >= 0 && gap <= comboMaxSpeedGap {
				groups[len(groups)-1] = append(last, a)
				continue
			}
		}
		groups = append(groups, []Action{a})
	}
	return groups
}

// comboChance uses the whole present side's player/pet roster, including support
// casters and knocked-out fighters. Absent fighters have left the party battle.
// Reborn players and pets add 99 to their level before averaging.
func (b *Battle) comboChance(side Side, target *Fighter) float64 {
	roster := b.Attackers
	if side == Defender {
		roster = b.Defenders
	}
	levels, count := 0, 0
	for _, f := range roster {
		if f.Absent || (f.Kind != Player && f.Kind != Pet) {
			continue
		}
		levels += f.comboLevel()
		count++
	}
	if count == 0 || target == nil {
		return 0
	}
	gap := float64(levels)/float64(count) - float64(target.comboLevel())
	return max(0, min(1, comboEqualLevelChance+gap/(2*comboGuaranteedLevelGap)))
}

func (r Rules) comboSucceeds(b *Battle, side Side, target *Fighter) bool {
	chance := b.comboChance(side, target)
	if chance <= 0 {
		return false
	}
	if chance >= 1 {
		return true
	}
	return r.Float() < chance
}

// comboLevel keeps the offset out of stored levels and uses int arithmetic so
// high reborn levels cannot wrap the native byte-sized visible level.
func (f *Fighter) comboLevel() int {
	level := int(f.Level)
	if f.Char != nil && f.Char.Reborn || f.Pet != nil && f.Pet.Reborn {
		level += comboRebirthLevelBonus
	}
	return level
}
