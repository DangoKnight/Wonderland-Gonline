package battle

import (
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

func TestPhysicalReductionAggregatesOnceAndBothShieldsCoexist(t *testing.T) {
	r := lowest
	r.Skills = map[uint16]assets.Skill{50000: {ID: 50000, EffectLayer: 2}, 50001: {ID: 50001, EffectLayer: 1}}
	f := &Fighter{HP: 100}
	f.ApplyEffect(50002, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, StackGroup: "protection", Modifiers: []assets.StatModifier{{Stat: assets.EffectPhysicalDamageTaken, Percent: -50}}})
	f.ApplyEffect(50003, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, StackGroup: "protection", Modifiers: []assets.StatModifier{{Stat: assets.EffectMagicalDamageTaken, Percent: -50}}})
	for _, skill := range []uint16{10001, 50000, 50001} {
		if got := r.incomingDamage(f, skill, 100); got != 50 {
			t.Fatal("shields did not coexist", skill, got)
		}
	}
	f.ApplyEffect(50004, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, StackGroup: "protection", Modifiers: []assets.StatModifier{{Stat: assets.EffectDamageTaken, Percent: -30}}})
	if got := r.incomingDamage(f, 50001, 100); got != 50 {
		t.Fatal("named group compounded", got)
	}
	f.ApplyEffect(50005, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, Modifiers: []assets.StatModifier{{Stat: assets.EffectDamageTaken, Percent: -10}}})
	if got := r.incomingDamage(f, 50001, 101); got != 40 {
		t.Fatal("physical reduction rounded twice", got)
	}
}

func TestPhysicalShieldProtectsAgainstMonsterAndRedirectedMonster(t *testing.T) {
	b, f, r := goddessFixture()
	f.HP, f.MaxHP, f.Def = 1000, 1000, 20
	monster := b.Defenders[0]
	monster.Atk = 100
	f.ApplyEffect(50000, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, Modifiers: []assets.StatModifier{{Stat: assets.EffectPhysicalDamageTaken, Percent: -50}}})
	r.monster(b, monster, map[int]bool{})
	if f.HP != 950 {
		t.Fatal("physical shield did not reduce monster strike", f.HP)
	}
	ally := &Fighter{Side: Defender, HP: 1000, MaxHP: 1000, Def: 20, X: 2, Y: 3}
	ally.ApplyEffect(50000, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, Modifiers: []assets.StatModifier{{Stat: assets.EffectPhysicalDamageTaken, Percent: -50}}})
	b.Defenders = append(b.Defenders, ally)
	monster.ApplyEffect(50001, 0, assets.SkillEffect{Target: assets.EffectSelf, Rounds: 2, AllyAttackChancePercent: 100})
	r.monster(b, monster, map[int]bool{})
	if ally.HP != 950 || f.HP != 950 {
		t.Fatal("redirected monster ignored physical shield", ally.HP, f.HP)
	}
}

func TestPhysicalShieldReducesOnlyPhysicalRecordInPlayerPetCombo(t *testing.T) {
	b, r, actions := comboFixture(100, 70)
	r.Skills = map[uint16]assets.Skill{50000: {ID: 50000, EffectLayer: 1, StatMultiplier: 2}, 50001: {ID: 50001, EffectLayer: 2, StatMultiplier: 2}}
	actions[0].Skill = 50000
	actions[0].Actor.Char.Skills = []game.LearnedSkill{{ID: 50000, Grade: 1}}
	actions[1].Skill = 50001
	pet := actions[1].Actor
	pet.Char = nil
	pet.Kind, pet.X = Pet, 3
	pet.Pet = &game.Pet{Skills: []game.PetSkill{{ID: 50001, Grade: 1}}}
	for _, a := range actions {
		a.Actor.Atk, a.Actor.Matk = 100, 100
	}
	target := b.Defenders[0]
	target.Def, target.Mdef = 20, 20
	target.ApplyEffect(50002, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, Modifiers: []assets.StatModifier{{Stat: assets.EffectPhysicalDamageTaken, Percent: -50}}})
	steps, _ := r.attack(b, b.order(r, actions)[0], map[int]bool{})
	if len(steps) != 1 || len(steps[0].SkillUses) != 2 || target.HP != 9630 {
		t.Fatal("mixed combo damage", target.HP, steps)
	}
	animation := steps[0].Packets[len(steps[0].Packets)-1]
	if len(animation) != 40 || animation[16] != 123 || animation[35] != 247 {
		t.Fatal("physical/magical native combo records", animation)
	}
}
