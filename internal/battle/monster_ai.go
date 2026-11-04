package battle

import "wonderland-gonline/internal/assets"

const (
	monsterTargetAdvantageWeight    = 4
	monsterTargetNeutralWeight      = 2
	monsterTargetDisadvantageWeight = 1
)

func monsterTargetWeight(actor, target *Fighter) int {
	switch relation := Elemental(actor.Element, target.Element); {
	case relation > 1:
		return monsterTargetAdvantageWeight
	case relation < 1:
		return monsterTargetDisadvantageWeight
	default:
		return monsterTargetNeutralWeight
	}
}

// monsterTarget samples already validated hostile targets without a player/pet
// preference. Even an elemental disadvantage retains a chance of being chosen.
func (r Rules) monsterTarget(actor *Fighter, targets []*Fighter) *Fighter {
	total := 0
	for _, target := range targets {
		total += monsterTargetWeight(actor, target)
	}
	roll := r.Next(0, total)
	for _, target := range targets {
		roll -= monsterTargetWeight(actor, target)
		if roll < 0 {
			return target
		}
	}
	panic("monster target roll outside weight range")
}

// monsterSkill selects uniformly among distinct usable native slots. No skill ID
// or name decides a buff/debuff: cast targets come from the effect definitions.
// Healing retains the same compatibility classifier used for player abilities.
func (r Rules) monsterSkill(b *Battle, actor *Fighter) (Action, bool) {
	if actor.Dead() || !actor.CanAct() {
		return Action{}, false
	}
	allies, enemies := b.Teams(actor.Side)
	type choice struct {
		action         Action
		hostileTargets []*Fighter
	}
	var choices []choice
	seen := map[uint16]bool{}
	for _, id := range actor.Skills {
		if id == 0 || seen[id] || Basic(id) || !r.CanUse(actor, id) {
			continue
		}
		seen[id] = true
		s := r.Skills[id]
		a := Action{Actor: actor, Kind: "attack", Skill: id}
		var targets []*Fighter
		switch {
		case s.RestoresSP() || s.CleansesStatus():
			a.Kind = "heal"
			for _, f := range living(allies) {
				if s.RestoresSP() && f.SP < f.MaxSP || s.CleansesStatus() && r.needsCleanse(f, s) {
					targets = append(targets, f)
				}
			}
		case hasCastEffects(s.Effects):
			a.Kind = "effect"
			// Validate all effect targets together; mixed self/enemy casts are valid.
			for _, f := range b.all() {
				valid := true
				needed := false
				for _, e := range s.Effects {
					if e.OnHit || e.Target == assets.EffectSelf {
						continue
					}
					needed = true
					if f.Dead() || f.Joining || f.HasEffect(id) || (e.Target == assets.EffectAlly && f.Side != actor.Side) || (e.Target == assets.EffectEnemy && f.Side == actor.Side) {
						valid = false
					}
				}
				if valid && needed {
					targets = append(targets, f)
				}
			}
			selfOnly := true
			for _, e := range s.Effects {
				if !e.OnHit && e.Target != assets.EffectSelf {
					selfOnly = false
				}
			}
			if selfOnly && !actor.HasEffect(id) {
				targets = []*Fighter{actor}
			}
		case isHeal(s) || isRevive(s):
			a.Kind = "heal"
			for _, f := range allies {
				if (isRevive(s) && f.Dead()) || (isHeal(s) && !f.Dead() && f.HP < f.MaxHP) {
					targets = append(targets, f)
				}
			}
		default:
			// Only native damage layers are offensive AI candidates.
			if s.EffectLayer != assets.SkillLayerPhysical && s.EffectLayer != assets.SkillLayerMagical {
				continue
			}
			targets = living(enemies)
		}
		if len(targets) == 0 {
			continue
		}
		// Support retains its resource-ratio priority. Hostile targeting is sampled
		// only after choosing a skill, so its target count cannot bias skill choice.
		target := targets[0]
		if a.Kind == "heal" {
			for _, f := range targets[1:] {
				lower := int64(f.HP)*int64(target.MaxHP) < int64(target.HP)*int64(f.MaxHP)
				if s.RestoresSP() {
					lower = int64(f.SP)*int64(target.MaxSP) < int64(target.SP)*int64(f.MaxSP)
				}
				if lower {
					target = f
				}
			}
		}
		a.TX, a.TY = target.X, target.Y
		c := choice{action: a}
		if target.Side != actor.Side {
			c.hostileTargets = targets
		}
		choices = append(choices, c)
	}
	if len(choices) == 0 {
		return Action{}, false
	}
	c := choices[r.Next(0, len(choices))]
	if len(c.hostileTargets) > 0 {
		target := r.monsterTarget(actor, c.hostileTargets)
		c.action.TX, c.action.TY = target.X, target.Y
	}
	return c.action, true
}
