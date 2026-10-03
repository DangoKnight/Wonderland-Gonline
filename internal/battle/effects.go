package battle

import (
	"math"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/protocol"
)

const (
	effectPercentBase        = 100
	effectNormalHitMode byte = 1
)

type ActiveEffect struct {
	Source     uint16
	Index      int
	Definition assets.SkillEffect
	Turns      int
}

// ApplyEffect refreshes the same ability effect without compounding it. Copies
// keep the immutable asset catalog separate from mutable battle ownership.
func (f *Fighter) ApplyEffect(source uint16, index int, e assets.SkillEffect) {
	if !validEffect(e) {
		return
	}
	e.Modifiers = append([]assets.StatModifier(nil), e.Modifiers...)
	if e.PeriodicDamage != nil {
		p := *e.PeriodicDamage
		e.PeriodicDamage = &p
	}
	for i := range f.Effects {
		if f.Effects[i].Source == source && f.Effects[i].Index == index {
			f.Effects[i].Definition = e
			f.Effects[i].Turns = max(f.Effects[i].Turns, e.Rounds)
			return
		}
	}
	f.Effects = append(f.Effects, ActiveEffect{Source: source, Index: index, Definition: e, Turns: e.Rounds})
}
func validEffect(e assets.SkillEffect) bool {
	return assets.ValidateSkillEffects([]assets.SkillEffect{e}) == nil
}

func (f *Fighter) HasEffect(source uint16) bool {
	for _, e := range f.Effects {
		if e.Source == source && e.Turns > 0 {
			return true
		}
	}
	return false
}
func (f *Fighter) RemoveEffects(source uint16) {
	out := f.Effects[:0]
	for _, e := range f.Effects {
		if e.Source != source {
			out = append(out, e)
		}
	}
	f.Effects = out
}
func (f *Fighter) tickEffects() {
	out := f.Effects[:0]
	for _, e := range f.Effects {
		e.Turns--
		if e.Turns > 0 {
			out = append(out, e)
		}
	}
	f.Effects = out
}

// modified aggregates all active effects, independent of ability names and IDs.
// Percent changes apply to the base; flat changes follow. Round only once and
// clamp the result before conversion to native signed damage/stat values.
func (f *Fighter) modified(stat assets.EffectStat, base int, additional ...assets.EffectStat) int {
	type extremes struct{ flatUp, flatDown, percentUp, percentDown int64 }
	groups := map[string]extremes{}
	var flat, percent int64
	for _, e := range f.Effects {
		if e.Turns <= 0 {
			continue
		}
		var effectFlat, effectPercent int64
		for _, m := range e.Definition.Modifiers {
			matches := m.Stat == stat
			for _, other := range additional {
				matches = matches || m.Stat == other
			}
			if matches {
				effectFlat += int64(m.Flat)
				effectPercent += int64(m.Percent)
			}
		}
		if e.Definition.StackGroup == "" {
			flat += effectFlat
			percent += effectPercent
			continue
		}
		g := groups[e.Definition.StackGroup]
		g.flatUp = max(g.flatUp, effectFlat)
		g.flatDown = min(g.flatDown, effectFlat)
		g.percentUp = max(g.percentUp, effectPercent)
		g.percentDown = min(g.percentDown, effectPercent)
		groups[e.Definition.StackGroup] = g
	}
	for _, g := range groups {
		flat += g.flatUp + g.flatDown
		percent += g.percentUp + g.percentDown
	}
	result := float64(base)*(1+float64(percent)/effectPercentBase) + float64(flat)
	return int(max(0, min(result, math.MaxInt32)))
}
func (f *Fighter) attack() int       { return f.modified(assets.EffectATK, f.Atk) }
func (f *Fighter) magicAttack() int  { return f.modified(assets.EffectMATK, f.Matk) }
func (f *Fighter) defense() int      { return f.modified(assets.EffectDEF, f.Def) }
func (f *Fighter) magicDefense() int { return f.modified(assets.EffectMDEF, f.Mdef) }
func (f *Fighter) speed() int        { return f.modified(assets.EffectSPD, f.Spd) }
func (r Rules) abilityEffect(b *Battle, a Action) Step {
	if !r.CanUse(a.Actor, a.Skill) {
		return Step{}
	}
	definitions := []assets.SkillEffect{}
	for _, e := range r.Skills[a.Skill].Effects {
		if !e.OnHit {
			definitions = append(definitions, e)
		}
	}
	if len(definitions) == 0 || assets.ValidateSkillEffects(definitions) != nil {
		return Step{}
	}
	allies, enemies := b.Attackers, b.Defenders
	if a.Actor.Side == Defender {
		allies, enemies = enemies, allies
	}
	indices := []int{}
	for i, e := range r.Skills[a.Skill].Effects {
		if !e.OnHit {
			indices = append(indices, i)
		}
	}
	targets := make([]*Fighter, len(definitions))
	for i, e := range definitions {
		switch e.Target {
		case assets.EffectAlly:
			targets[i] = at(allies, a.TX, a.TY, true)
		case assets.EffectEnemy:
			targets[i] = at(enemies, a.TX, a.TY, true)
		case assets.EffectSelf:
			targets[i] = a.Actor
		}
		if targets[i] == nil {
			return Step{}
		}
	}
	packets := [][]byte{r.spend(a.Actor, a.Skill), turnPacket(a.Actor)}
	emitted := map[*Fighter]bool{}
	for i, e := range definitions {
		target := targets[i]
		target.ApplyEffect(a.Skill, indices[i], e)
		if emitted[target] {
			continue
		}
		emitted[target] = true
		packets = append(packets, hitRecord(protocol.Builder{protocol.CommandBattleAction, protocol.BattleActionAnimation}, a.Actor, target, a.Skill, false, 0, 0, effectNormalHitMode))
	}
	return Step{Packets: packets, Delay: r.Delay(a.Skill, a.Kind), SkillUses: []Action{a}}
}

func hasCastEffects(defs []assets.SkillEffect) bool {
	for _, e := range defs {
		if !e.OnHit {
			return true
		}
	}
	return false
}
func (f *Fighter) breakDamageEffects() {
	out := f.Effects[:0]
	for _, e := range f.Effects {
		if !e.Definition.BreakOnDamage {
			out = append(out, e)
		}
	}
	f.Effects = out
}
func (f *Fighter) allyAttackChance() int {
	groups := map[string]int{}
	total := 0
	for _, e := range f.Effects {
		if e.Turns <= 0 {
			continue
		}
		c := e.Definition.AllyAttackChancePercent
		if e.Definition.StackGroup == "" {
			total += c
		} else {
			groups[e.Definition.StackGroup] = max(groups[e.Definition.StackGroup], c)
		}
	}
	for _, c := range groups {
		total += c
	}
	return min(total, effectPercentBase)
}
func (f *Fighter) periodicDamage() int {
	groups := map[string]int{}
	total := 0
	for _, e := range f.Effects {
		if e.Turns <= 0 || e.Definition.PeriodicDamage == nil {
			continue
		}
		p := e.Definition.PeriodicDamage
		damage := int(min(float64(math.MaxInt32), max(float64(p.Minimum), float64(f.MaxHP)*float64(p.MaxHPPercent)/effectPercentBase+float64(p.Flat))))
		if e.Definition.StackGroup == "" {
			total += damage
		} else {
			groups[e.Definition.StackGroup] = max(groups[e.Definition.StackGroup], damage)
		}
	}
	for _, d := range groups {
		total += d
	}
	return min(total, math.MaxInt32)
}
func (r Rules) applyHitEffects(actor, target *Fighter, skill uint16) {
	for i, e := range r.Skills[skill].Effects {
		if !e.OnHit {
			continue
		}
		recipient := target
		if e.Target == assets.EffectSelf {
			recipient = actor
		} else if e.Target == assets.EffectEnemy && target.Side == actor.Side || e.Target == assets.EffectAlly && target.Side != actor.Side {
			continue
		}
		if !recipient.Dead() {
			recipient.ApplyEffect(skill, i, e)
		}
	}
}

func (r Rules) redirectedTarget(b *Battle, actor *Fighter) *Fighter {
	chance := actor.allyAttackChance()
	if chance == 0 || r.Next(0, effectPercentBase) >= chance {
		return nil
	}
	allies := b.Attackers
	if actor.Side == Defender {
		allies = b.Defenders
	}
	var targets []*Fighter
	for _, f := range allies {
		if f != actor && !f.Dead() {
			targets = append(targets, f)
		}
	}
	if len(targets) == 0 {
		return nil
	}
	return targets[r.Next(0, len(targets))]
}

// attackMisses aggregates protection properties across every active effect.
// Basic strikes and unresolved offensive layers follow the physical damage
// fallback. A staff's basic strike remains a physical action despite using MATK.
func (r Rules) attackMisses(actor, target *Fighter, skill uint16) bool {
	s, known := r.Skills[skill]
	physical := Basic(skill) || !known || s.EffectLayer != assets.SkillLayerMagical
	grade := 1
	if actor.Pet != nil {
		if learned, ok := petLearned(actor.Pet, skill); ok {
			grade = int(learned.Grade)
		}
	} else if actor.Char != nil {
		if learned, ok := learned(actor.Char, skill); ok {
			grade = int(learned.Grade)
		}
	}
	area := known && !Basic(skill) && s.IsAreaAttack(grade)
	for _, e := range target.Effects {
		if e.Turns > 0 && (physical && e.Definition.MissPhysicalAttacks || area && e.Definition.MissAreaAttacks) {
			return true
		}
	}
	return false
}

// incomingDamage aggregates generic and applicable attack-type modifiers in one
// pass, preserving named-group stacking and rounding only once. Basic strikes,
// including staff attacks, retain their physical classification.
func (r Rules) incomingDamage(target *Fighter, skill uint16, damage int) int {
	if s, ok := r.Skills[skill]; ok && !Basic(skill) && s.EffectLayer == assets.SkillLayerMagical {
		return target.modified(assets.EffectDamageTaken, damage, assets.EffectMagicalDamageTaken)
	}
	return target.modified(assets.EffectDamageTaken, damage, assets.EffectPhysicalDamageTaken)
}
