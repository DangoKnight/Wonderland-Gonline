package battle

import (
	"math"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func effect(stat assets.EffectStat, flat, percent int32, rounds int, group string) assets.SkillEffect {
	return assets.SkillEffect{Target: assets.EffectAlly, Rounds: rounds, StackGroup: group, Modifiers: []assets.StatModifier{{Stat: stat, Flat: flat, Percent: percent}}}
}
func TestEffectsAggregateAndRefreshWithoutChangingBase(t *testing.T) {
	f := &Fighter{Atk: 100, Spd: 50}
	a := effect(assets.EffectATK, 10, 20, 3, "")
	b := effect(assets.EffectATK, -5, -50, 2, "")
	f.ApplyEffect(50001, 0, a)
	f.ApplyEffect(50002, 0, b)
	if f.attack() != 75 || f.Atk != 100 {
		t.Fatal("independent buff/debuff aggregation", f.attack(), f.Atk)
	}
	f.ApplyEffect(50001, 0, a)
	if len(f.Effects) != 2 || f.attack() != 75 {
		t.Fatal("refresh duplicated modifiers")
	}
	f.tickEffects()
	f.tickEffects()
	if f.attack() != 130 || f.HasEffect(50002) {
		t.Fatal("expired reduction remained active", f.attack())
	}
	f.tickEffects()
	if f.attack() != 100 {
		t.Fatal("base stat was not restored")
	}
}
func TestEffectGroupsAndOrderIndependence(t *testing.T) {
	defs := []assets.SkillEffect{effect(assets.EffectDamageDealt, 0, 100, 3, "power"), effect(assets.EffectDamageDealt, 0, 200, 2, "power"), effect(assets.EffectDamageDealt, 0, -50, 3, ""), effect(assets.EffectDamageDealt, 0, 25, 3, "")}
	for _, order := range [][]int{{0, 1, 2, 3}, {3, 2, 1, 0}} {
		f := &Fighter{}
		for _, i := range order {
			f.ApplyEffect(uint16(50000+i), 0, defs[i])
		}
		if got := f.modified(assets.EffectDamageDealt, 100); got != 275 {
			t.Fatal("group/independent contributions", got)
		}
		f.tickEffects()
		f.tickEffects()
		if got := f.modified(assets.EffectDamageDealt, 100); got != 175 {
			t.Fatal("weaker group effect did not resume", got)
		}
	}
}
func TestEffectsClampAndSumModifiersWithinDefinition(t *testing.T) {
	f := &Fighter{Atk: 100}
	e := effect(assets.EffectATK, 5, 10, 2, "one")
	e.Modifiers = append(e.Modifiers, assets.StatModifier{Stat: assets.EffectATK, Flat: 7, Percent: 20})
	f.ApplyEffect(50000, 0, e)
	if f.attack() != 142 {
		t.Fatal("same-definition contributions lost", f.attack())
	}
	f.ApplyEffect(50001, 0, effect(assets.EffectATK, 0, -1000, 2, ""))
	if f.attack() != 0 {
		t.Fatal("negative stats not clamped")
	}
	f.RemoveEffects(50001)
	f.ApplyEffect(50002, 0, effect(assets.EffectATK, math.MaxInt32, math.MaxInt32, 2, ""))
	if f.attack() != math.MaxInt32 {
		t.Fatal("large modifiers overflowed")
	}
}
func TestSpeedEffectsExpireAndAffectRoundOrder(t *testing.T) {
	b, f, r := goddessFixture()
	m := b.Defenders[0]
	f.Spd = 5
	m.Spd = 20
	f.ApplyEffect(50000, 0, effect(assets.EffectSPD, 30, 0, 1, ""))
	steps := b.order(r, []Action{{Actor: f, Kind: "attack", Skill: 10001, TX: 2, TY: 2}})
	if steps[0][0].Actor != f || f.Spd != 5 {
		t.Fatal("temporary speed ordering")
	}
	f.tickEffects()
	if b.order(r, []Action{{Actor: f, Kind: "attack", Skill: 10001, TX: 2, TY: 2}})[0][0].Actor != m || f.speed() != 5 {
		t.Fatal("speed failed to expire")
	}
}
func TestArbitraryAbilityAppliesMultipleEffectsAtomically(t *testing.T) {
	b, f, r := goddessFixture()
	f.Char.Skills = append(f.Char.Skills, game.LearnedSkill{ID: 50001, Grade: 1})
	ally := effect(assets.EffectATK, 10, 0, 2, "")
	ally.Target = assets.EffectSelf
	enemy := effect(assets.EffectDEF, 0, -25, 2, "")
	enemy.Target = assets.EffectEnemy
	r.Skills[50001] = assets.Skill{ID: 50001, Name: "arbitrary ability", SP: 7, Effects: []assets.SkillEffect{ally, enemy}}
	b.Submit(r, 7, 1, []byte{4, 2, 2, 2, 81, 195})
	if b.Pending[f.key()].Kind != "effect" {
		t.Fatal("custom ability not dispatched generically")
	}
	before := f.SP
	step := r.abilityEffect(b, b.Pending[f.key()])
	if len(step.SkillUses) != 1 || f.SP != before-7 || f.attack() != f.Atk+10 || !b.Defenders[0].HasEffect(50001) {
		t.Fatal("multi-effect ability failed")
	}
	f.RemoveEffects(50001)
	b.Defenders[0].RemoveEffects(50001)
	step = r.abilityEffect(b, Action{Actor: f, Skill: 50001, TX: 9, TY: 9})
	if len(step.Packets) != 0 || f.SP != before-7 || f.HasEffect(50001) {
		t.Fatal("partial effect/cost applied with invalid second target")
	}
}
func TestEnemyAbilitiesResolveSidesAndProtection(t *testing.T) {
	b, f, r := goddessFixture()
	m := b.Defenders[0]
	r.Skills[50002] = assets.Skill{ID: 50002, SP: 1, Effects: []assets.SkillEffect{effect(assets.EffectDamageTaken, 0, -50, 2, "")}}
	step := r.abilityEffect(b, Action{Actor: m, Skill: 50002, TX: m.X, TY: m.Y})
	if len(step.SkillUses) != 1 || !m.HasEffect(50002) || f.HasEffect(50002) {
		t.Fatal("monster ally targeting chose player team")
	}
	m.Atk = 100
	f.HP = 500
	f.Def = 10
	f.ApplyEffect(50003, 0, effect(assets.EffectDamageTaken, 0, -50, 2, ""))
	m.ApplyEffect(50004, 0, effect(assets.EffectDamageDealt, 0, 100, 2, ""))
	r.monster(b, m, map[int]bool{})
	// Base 110 damage, +100% outgoing and -50% incoming = 110.
	if f.HP != 390 {
		t.Fatal("monster damage did not aggregate modifiers", f.HP)
	}
}
