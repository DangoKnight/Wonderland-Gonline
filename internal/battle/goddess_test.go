package battle

import (
	"bytes"
	"fmt"
	"testing"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func goddessFixture() (*Battle, *Fighter, Rules) {
	b, f := setup(1000)
	f.Char.Skills = append(f.Char.Skills, game.LearnedSkill{ID: 15188, Grade: 1}, game.LearnedSkill{ID: 15189, Grade: 1})
	f.SP = 300
	r := lowest
	r.Skills = map[uint16]assets.Skill{
		15188: {ID: 15188, Name: "Translated water skill", SP: 52, EffectLayer: 15, NativeEffectCode52: 172, NativeRounds51: 3},
		15189: {ID: 15189, Name: "Translated fire skill", SP: 82, EffectLayer: 19, NativeEffectCode52: 60, NativeRounds51: 2},
	}
	for id, skill := range r.Skills {
		assets.PopulateSkillEffects(&skill)
		r.Skills[id] = skill
	}
	return b, f, r
}

func TestShrinkIsNonDamagingDebuff(t *testing.T) {
	b, f, r := goddessFixture()
	b.Submit(r, 7, 1, []byte{4, 2, 2, 2, 84, 59})
	if b.Pending[f.key()].Kind != "effect" {
		t.Fatal("Shrink was classified as damage")
	}
	m := b.Defenders[0]
	original := [4]int{m.Atk, m.Matk, m.Def, m.Mdef}
	steps, outcome := b.Round(r)
	if outcome != Continue || m.HP != 1000 || !m.HasEffect(15188) || f.SP != 248 {
		t.Fatal("Shrink damaged target or did not apply", outcome, m.HP, f.SP)
	}
	if [4]int{m.Atk, m.Matk, m.Def, m.Mdef} != original {
		t.Fatal("base battle snapshot was mutated")
	}
	if m.attack() != 6 || m.magicAttack() != 6 || m.defense() != 4 || m.magicDefense() != 4 {
		t.Fatal("combat reductions incorrect")
	}
	want := []byte{50, 1, 17, 0, 4, 2, 84, 59, 0, 1, 2, 2, 1, 0, 1, 0, 0, 0, 0, 0, 1}
	if len(steps[0].SkillUses) != 1 || !bytes.Equal(steps[0].Packets[2], want) {
		t.Fatal("native non-HP effect or proficiency missing", steps[0])
	}
	// Enemy acts later in the same round with reduced attack: damage floor five.
	if f.HP != 95 {
		t.Fatal("enemy used unreduced attack", f.HP)
	}
	for i := 0; i < 3; i++ {
		b.Timeout()
		b.Round(r)
	}
	if m.HasEffect(15188) || m.attack() != original[0] || m.defense() != original[2] {
		t.Fatal("expiry did not restore effective stats")
	}
}

func TestShrinkRefreshAndDamagePaths(t *testing.T) {
	b, f, r := goddessFixture()
	m := b.Defenders[0]
	m.Atk, m.Matk, m.Def, m.Mdef = 101, 81, 61, 41
	r.abilityEffect(b, Action{Actor: f, Skill: 15188, TX: 2, TY: 2})
	r.abilityEffect(b, Action{Actor: f, Skill: 15188, TX: 2, TY: 2})
	if m.attack() != 50 || m.magicAttack() != 40 || m.defense() != 30 || m.magicDefense() != 20 {
		t.Fatal("refresh compounded reductions")
	}
	f.Atk, f.Matk = 100, 90
	if got := r.baseDamage(f, m, 10001, 1); got != 171 {
		t.Fatal("physical defense ignored", got)
	}
	f.Weapon = 5
	if got := r.baseDamage(f, m, 10001, 1); got != 161 {
		t.Fatal("staff magic defense ignored", got)
	}
	f.Weapon = 0
	r.Skills[20001] = assets.Skill{ID: 20001, EffectLayer: 2, StatMultiplier: 2}
	if got := r.baseDamage(f, m, 20001, 1); got != 170 {
		t.Fatal("magic skill defense ignored", got)
	}
	m.RemoveEffects(15188)
	if m.attack() != 101 || m.magicDefense() != 41 {
		t.Fatal("odd base stats lost precision")
	}
}

func TestHotFireTargetsAllyAndDoesNotStackWithFiery(t *testing.T) {
	b, f, r := goddessFixture()
	b.Submit(r, 7, 1, []byte{4, 2, 4, 2, 85, 59})
	if b.Pending[f.key()].Kind != "effect" {
		t.Fatal("Hot Fire was classified as damage")
	}
	steps, _ := b.Round(r)
	if !f.HasEffect(15189) || f.SP != 218 || b.Defenders[0].HP != 1000 || len(steps[0].SkillUses) != 1 {
		t.Fatal("Hot Fire failed to buff ally")
	}
	// The base physical damage is 16; Goddess multiplier is three even when
	// the weaker generic Fiery status is present.
	f.ApplyEffect(11072, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 3, StackGroup: "attack_damage_buff", Modifiers: []assets.StatModifier{{Stat: assets.EffectDamageDealt, Percent: 100}}})
	b.Submit(r, 7, 1, []byte{4, 2, 2, 2})
	b.Round(r)
	if b.Defenders[0].HP != 952 {
		t.Fatal("Hot Fire damage or buff precedence", b.Defenders[0].HP)
	}
	b.Timeout()
	b.Round(r)
	if f.HasEffect(15189) {
		t.Fatal("Hot Fire did not expire after three rounds including cast")
	}
}

func TestGoddessInvalidTargetsAndUnableCasters(t *testing.T) {
	for _, skill := range []uint16{15188, 15189} {
		for _, bad := range []string{"opposite side", "missing", "dead target", "no SP", "unlearned", "sealed"} {
			t.Run(fmt.Sprintf("%s/%d", bad, skill), func(t *testing.T) {
				b, f, r := goddessFixture()
				tx, ty := byte(2), byte(2)
				if skill == 15189 {
					tx, ty = 4, 2
				}
				switch bad {
				case "opposite side":
					if skill == 15188 {
						tx = 4
					} else {
						tx = 2
					}
				case "missing":
					tx, ty = 9, 9
				case "dead target":
					if skill == 15188 {
						b.Defenders[0].HP = 0
					} else {
						b.Attackers = append(b.Attackers, &Fighter{X: 4, Y: 3, HP: 0})
						ty = 3
					}
				case "no SP":
					f.SP = 0
				case "unlearned":
					f.Char.Skills = nil
				case "sealed":
					f.ApplyEffect(50010, 0, assets.SkillEffect{Target: assets.EffectSelf, Rounds: 2, BlockActions: true})
				}
				before := f.SP
				step := r.abilityEffect(b, Action{Actor: f, Skill: skill, TX: tx, TY: ty})
				if len(step.Packets) != 0 || len(step.SkillUses) != 0 || f.SP != before || f.HasEffect(15189) || b.Defenders[0].HasEffect(15188) {
					t.Fatal("invalid effect consumed cost or changed state")
				}
			})
		}
	}
}

func TestGoddessDoesNotJoinAttackCombo(t *testing.T) {
	b, f, r := goddessFixture()
	ally := *f
	ally.X, ally.Y, ally.ID = 4, 3, 8
	ally.Spd = f.Spd
	b.Attackers = append(b.Attackers, &ally)
	steps := b.order(r, []Action{{Actor: f, Kind: "effect", Skill: 15188, TX: 2, TY: 2}, {Actor: &ally, Kind: "attack", Skill: 10001, TX: 2, TY: 2}})
	if len(steps[0]) != 1 || len(steps[1]) != 1 {
		t.Fatal("debuff joined offensive combo", steps)
	}
}

func TestShrunkMonsterDamageUsesEffectiveAttack(t *testing.T) {
	b, f, r := goddessFixture()
	m := b.Defenders[0]
	f.HP, f.Def = 500, 10
	m.Atk = 100
	m.ApplyEffect(15188, 0, r.Skills[15188].Effects[0])
	r.monster(b, m, map[int]bool{})
	if f.HP != 450 {
		t.Fatal("normal monster damage ignored Shrink", f.HP)
	}
	// Confused monsters use the same effective combat stats against an ally.
	ally := &Fighter{Side: Defender, HP: 500, Def: 10, X: 2, Y: 3}
	b.Defenders = append(b.Defenders, ally)
	m.ApplyEffect(50010, 0, assets.SkillEffect{Target: assets.EffectSelf, Rounds: 2, AllyAttackChancePercent: 50})
	r.monster(b, m, map[int]bool{})
	if ally.HP != 450 {
		t.Fatal("confused monster damage ignored Shrink", ally.HP)
	}
}

func TestHotFireBuffsPetAttacks(t *testing.T) {
	b, f, r := goddessFixture()
	pet := &Fighter{Side: Attacker, Kind: Pet, ID: 99, Owner: 7, X: 3, Y: 2, HP: 1000, MaxHP: 1000, Atk: 100, Spd: 10, Pet: &game.Pet{}}
	b.Attackers = append(b.Attackers, pet)
	step := r.abilityEffect(b, Action{Actor: f, Skill: 15189, TX: 3, TY: 2})
	if !pet.HasEffect(15189) || len(step.SkillUses) != 1 {
		t.Fatal("ally pet did not receive Hot Fire")
	}
	b.Submit(r, 7, 4, []byte{4, 2, 0, 0})
	b.Submit(r, 7, 1, []byte{3, 2, 2, 2})
	b.Round(r)
	// Pet's basic attack: (100*2 - 9 + 1) * 3 = 576.
	if b.Defenders[0].HP != 424 {
		t.Fatal("pet damage ignored Hot Fire", b.Defenders[0].HP)
	}
}
