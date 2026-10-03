package battle

import (
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

func blocking(rounds int, wake bool) assets.SkillEffect {
	return assets.SkillEffect{Target: assets.EffectEnemy, Rounds: rounds, BlockActions: true, BreakOnDamage: wake}
}
func TestControlEffectsAggregateExpireAndBreakOnDamage(t *testing.T) {
	f := &Fighter{HP: 100}
	f.ApplyEffect(50000, 0, blocking(2, true))
	f.ApplyEffect(50001, 0, blocking(3, false))
	if f.CanAct() {
		t.Fatal("blocking effects ignored")
	}
	f.hurt(0)
	if !f.HasEffect(50000) {
		t.Fatal("zero damage woke target")
	}
	f.hurt(5)
	if f.HasEffect(50000) || f.CanAct() {
		t.Fatal("damage removed unrelated restriction")
	}
	for i := 0; i < 3; i++ {
		f.tickEffects()
	}
	if !f.CanAct() {
		t.Fatal("restriction failed to expire")
	}
}
func TestPeriodicEffectsStackByPolicyAndKill(t *testing.T) {
	f := &Fighter{HP: 100, MaxHP: 100}
	p := assets.SkillEffect{Target: assets.EffectEnemy, Rounds: 2, StackGroup: "poison", PeriodicDamage: &assets.PeriodicDamage{MaxHPPercent: 8, Minimum: 5}}
	f.ApplyEffect(50000, 0, p)
	f.ApplyEffect(50001, 0, p)
	p.PeriodicDamage.Flat = 1000 // Active records own a copy, not the asset pointer.
	if f.periodicDamage() != 8 {
		t.Fatal("same-group ticks compounded or shared catalog pointer")
	}
	f.ApplyEffect(50002, 0, assets.SkillEffect{Target: assets.EffectEnemy, Rounds: 2, PeriodicDamage: &assets.PeriodicDamage{Flat: 7}})
	if f.periodicDamage() != 15 {
		t.Fatal("independent periodic effects did not add")
	}
	f.ApplyEffect(50003, 0, blocking(2, true))
	f.hurt(f.periodicDamage())
	if f.HP != 85 || f.HasEffect(50003) {
		t.Fatal("periodic damage did not break damage-sensitive effects")
	}
	f.tickEffects()
	f.tickEffects()
	if f.periodicDamage() != 0 {
		t.Fatal("periodic damage did not expire")
	}
	b, self, r := goddessFixture()
	self.HP = 3
	self.ApplyEffect(50000, 0, assets.SkillEffect{Target: assets.EffectEnemy, Rounds: 2, PeriodicDamage: &assets.PeriodicDamage{Minimum: 5}})
	b.Timeout()
	_, outcome := b.Round(r)
	if outcome != Defeat || self.Deaths != 1 {
		t.Fatal("periodic knockout not counted", outcome, self.Deaths)
	}
}
func TestArbitrarySleepLikeAbilityAndOnHitEffects(t *testing.T) {
	b, f, r := goddessFixture()
	f.Char.Skills = append(f.Char.Skills, game.LearnedSkill{ID: 50000, Grade: 1})
	r.Skills[50000] = assets.Skill{ID: 50000, SP: 3, Effects: []assets.SkillEffect{blocking(2, true)}}
	b.Submit(r, 7, 1, []byte{4, 2, 2, 2, 80, 195})
	steps, _ := b.Round(r)
	m := b.Defenders[0]
	if m.HP != 1000 || m.CanAct() || len(steps[0].SkillUses) != 1 {
		t.Fatal("pure control ability damaged or failed to block")
	}
	e := blocking(2, true)
	e.OnHit = true
	r.Skills[50001] = assets.Skill{ID: 50001, EffectLayer: 1, StatMultiplier: 2, Effects: []assets.SkillEffect{e}}
	f.Char.Skills = append(f.Char.Skills, game.LearnedSkill{ID: 50001, Grade: 1})
	b.Submit(r, 7, 1, []byte{4, 2, 2, 2, 81, 195})
	if b.Pending[f.key()].Kind != "attack" {
		t.Fatal("on-hit ability became non-damaging")
	}
	b.Round(r)
	if m.HP >= 1000 || !m.HasEffect(50001) || m.HasEffect(50000) {
		t.Fatal("hit effect application/removal order")
	}
}
func TestAttackRedirectionWorksForAnyActor(t *testing.T) {
	b, f, r := goddessFixture()
	ally := &Fighter{Side: Attacker, X: 4, Y: 3, HP: 500, Def: 0}
	b.Attackers = append(b.Attackers, ally)
	f.ApplyEffect(50000, 0, assets.SkillEffect{Target: assets.EffectEnemy, Rounds: 2, AllyAttackChancePercent: 100})
	_, _ = r.attack(b, []Action{{Actor: f, Kind: "attack", Skill: 10001, TX: 2, TY: 2}}, map[int]bool{})
	if ally.HP >= 500 || b.Defenders[0].HP != 1000 {
		t.Fatal("player redirection did not select an ally")
	}
	f.RemoveEffects(50000)
	if r.redirectedTarget(b, f) != nil {
		t.Fatal("redirection persisted after removal")
	}
}
func TestRedirectedAttackStopsAfterAllyKnockout(t *testing.T) {
	b, f, r := goddessFixture()
	ally := &Fighter{Side: Attacker, X: 4, Y: 3, HP: 1}
	b.Attackers = append(b.Attackers, ally)
	f.ApplyEffect(50000, 0, assets.SkillEffect{Target: assets.EffectEnemy, Rounds: 2, AllyAttackChancePercent: 100})
	originalHP := f.HP
	a := Action{Actor: f, Kind: "attack", Skill: 10001, TX: 2, TY: 2}
	_, won := r.attack(b, []Action{a, a, a}, map[int]bool{})
	if !ally.Dead() || f.HP != originalHP || b.Defenders[0].HP != 1000 || won {
		t.Fatal("redirected attack retargeted after ally knockout")
	}
}

func TestVisualEffectsAreTimedAndDoNotRestrictActions(t *testing.T) {
	f := &Fighter{HP: 100}
	f.ApplyEffect(50000, 0, assets.SkillEffect{Target: assets.EffectSelf, Rounds: 1, Presentation: "vanish"})
	if !f.HasEffect(50000) || !f.CanAct() {
		t.Fatal("visual effect missing or changed action policy")
	}
	f.tickEffects()
	if f.HasEffect(50000) {
		t.Fatal("visual effect did not expire")
	}
}
