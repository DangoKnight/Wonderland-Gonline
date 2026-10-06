package battle

import "wonderland-gonline/internal/assets"

// monsterSkill selects uniformly among distinct usable native slots. No skill ID
// or name decides a buff/debuff: cast targets come from the effect definitions.
// Healing retains the same compatibility classifier used for player abilities.
func (r Rules) monsterSkill(b *Battle, actor *Fighter) (Action, bool) {
	if actor.Dead() || !actor.CanAct() {
		return Action{}, false
	}
	allies, enemies := b.Teams(actor.Side)
	var choices []Action
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
					if f.Dead() || f.HasEffect(id) || (e.Target == assets.EffectAlly && f.Side != actor.Side) || (e.Target == assets.EffectEnemy && f.Side == actor.Side) {
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
		// Lowest HP ratio for support, stable first live enemy for offensive casts;
		// random choice between skills does not consume rolls for rejected slots.
		target := targets[0]
		if a.Kind == "heal" {
			for _, f := range targets[1:] {
				if int64(f.HP)*int64(target.MaxHP) < int64(target.HP)*int64(f.MaxHP) {
					target = f
				}
			}
		}
		a.TX, a.TY = target.X, target.Y
		choices = append(choices, a)
	}
	if len(choices) == 0 {
		return Action{}, false
	}
	return choices[r.Next(0, len(choices))], true
}
