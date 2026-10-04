package battle

import (
	"math"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

const (
	animationActorHeaderBytes = 6
	animationTargetBytes      = 11
)

func skillGrade(actor *Fighter, id uint16) int {
	if actor == nil {
		return 1
	}
	if actor.Pet != nil {
		if s, ok := petLearned(actor.Pet, id); ok {
			return int(s.Grade)
		}
	}
	if actor.Char != nil {
		if s, ok := learned(actor.Char, id); ok {
			return int(s.Grade)
		}
	}
	return 1
}

// Selection is always restricted to one side; mirrored formations need no
// translation because offsets are expressed in actual wire grid coordinates.
func formationTargets(side []*Fighter, selected *Fighter, shape assets.SkillTargeting, includeDead bool) []*Fighter {
	var result []*Fighter
	seen := map[*Fighter]bool{}
	add := func(f *Fighter) {
		if f != nil && !f.Captured && !seen[f] && (includeDead || !f.Dead()) && len(result) < assets.MaxBattleSideTargets {
			seen[f] = true
			result = append(result, f)
		}
	}
	add(selected)
	if shape.All {
		for _, f := range side {
			add(f)
		}
		return result
	}
	for _, o := range shape.Offsets {
		x, y := int(selected.X)+int(o.X), int(selected.Y)+int(o.Y)
		if x < 0 || x > 255 || y < 0 || y > 255 {
			continue
		}
		add(at(side, byte(x), byte(y), !includeDead))
	}
	return result
}

type targetResult struct {
	target   *Fighter
	result   byte
	defended bool
	stat     byte
	amount   int
	mode     byte
}

// AC50:1 actor records are length-prefixed. The target count is followed by
// eleven bytes per result; the existing single-target record is its n=1 case.
func areaRecord(p protocol.Builder, actor *Fighter, skill uint16, results []targetResult) protocol.Builder {
	p = p.U16(uint16(animationActorHeaderBytes + animationTargetBytes*len(results))).U8(actor.X).U8(actor.Y).U16(skill).U8(0).U8(byte(len(results)))
	for _, r := range results {
		defended := byte(0)
		if r.defended {
			defended = 1
		}
		p = p.U8(r.target.X).U8(r.target.Y).U8(r.result).U8(defended).U8(1).U8(r.stat).U32(uint32(r.amount)).U8(r.mode)
	}
	return p
}
func (r Rules) areaAttack(b *Battle, a Action, shape assets.SkillTargeting, defending map[int]bool) []Step {
	if a.Actor == nil || a.Actor.Absent || !r.CanUse(a.Actor, a.Skill) {
		return nil
	}
	_, enemies := b.Teams(a.Actor.Side)
	selected := at(enemies, a.TX, a.TY, true)
	if redirected := r.redirectedTarget(b, a.Actor); redirected != nil {
		selected = redirected
		enemies, _ = b.Teams(a.Actor.Side)
	}
	if selected == nil {
		if alive := living(enemies); len(alive) > 0 {
			selected = alive[0]
		}
	}
	if selected == nil {
		return nil
	}
	targets := formationTargets(enemies, selected, shape, false)
	packets := [][]byte{turnPacket(a.Actor), r.spend(a.Actor, a.Skill)}
	var results []targetResult
	var defeated [][]byte
	for _, target := range targets {
		result := targetResult{target: target, result: protocol.BattleHitMiss, stat: game.StatCurrentHP, mode: effectNormalHitMode}
		if !r.attackMisses(a.Actor, target, a.Skill) {
			result.result = protocol.BattleHitLanded
			result.defended = defending[target.key()]
			result.amount, result.mode = r.strikeDamage(a, target, result.defended, false)
			target.hurt(result.amount)
			r.applyHitEffects(a.Actor, target, a.Skill)
			if target.Dead() {
				defeated = append(defeated, []byte{protocol.CommandBattleEffect, protocol.BattleEffectDefeated, target.X, target.Y})
			}
		}
		results = append(results, result)
	}
	packets = append(packets, areaRecord(protocol.Builder{protocol.CommandBattleAction, protocol.BattleActionAnimation}, a.Actor, a.Skill, results))
	steps := []Step{{Packets: packets, Delay: r.Delay(a.Skill, a.Kind), SkillUses: []Action{a}}}
	if len(defeated) > 0 {
		steps = append(steps, Step{Packets: defeated})
	}
	return steps
}

func (r Rules) strikeDamage(a Action, target *Fighter, defending, combo bool) (int, byte) {
	actor := a.Actor
	dmg := r.baseDamage(actor, target, a.Skill, r.Next(1, 6))
	dmg = max(1, int(min(float64(dmg)*Elemental(actor.Element, target.Element), math.MaxInt32)))
	dmg = max(1, actor.modified(assets.EffectDamageDealt, dmg))
	if combo {
		dmg = int(min(float64(dmg)*comboDamageMultiplier, math.MaxInt32))
	}
	// Players and pets use their own equipment's critical chance.
	crit := false
	var equipment *game.Equipment
	if actor.Pet != nil {
		equipment = &actor.Pet.Equipment
	} else if actor.Char != nil {
		equipment = &actor.Char.Equipment
	}
	if equipment != nil {
		var d int32
		d, crit = r.Critical.Damage(int32(dmg), equipment.IDs(), a.Kind, r.Next(0, 100))
		dmg = int(d)
	}
	guarded := defending
	dmg = max(1, r.incomingDamage(target, a.Skill, dmg))
	if guarded {
		dmg = max(1, dmg/2)
	}
	mode := byte(effectNormalHitMode)
	if crit {
		mode = effectCriticalHitMode // Enlarged critical digits.
	}
	return dmg, mode
}
