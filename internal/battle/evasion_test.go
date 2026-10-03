package battle

import (
	"bytes"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

func TestProtectionEffectsForceMissesByAttackCategory(t *testing.T) {
	for _, tc := range []struct {
		name       string
		layer      byte
		area, miss bool
	}{
		{"physical", 1, false, true}, {"physical area", 1, true, true},
		{"single magic", 2, false, false}, {"area magic", 2, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, f, r := goddessFixture()
			target := b.Defenders[0]
			target.ApplyEffect(50002, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, MissPhysicalAttacks: true})
			target.ApplyEffect(50003, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, MissAreaAttacks: true})
			target.ApplyEffect(50004, 0, blocking(2, true))
			onHit := blocking(2, false)
			onHit.OnHit = true
			r.Skills[50000] = assets.Skill{ID: 50000, SP: 7, EffectLayer: tc.layer, StatMultiplier: 2, AreaAttack: &tc.area, Effects: []assets.SkillEffect{onHit}}
			f.Char.Skills = append(f.Char.Skills, game.LearnedSkill{ID: 50000, Grade: 1})
			initialSP := f.SP
			steps, _ := r.attack(b, []Action{{Actor: f, Kind: "attack", Skill: 50000, TX: 2, TY: 2}}, map[int]bool{})
			if f.SP != initialSP-7 || len(steps[0].SkillUses) != 1 {
				t.Fatal("miss must consume SP and retain proficiency")
			}
			if tc.miss {
				if target.HP != 1000 || target.HasEffect(50000) || !target.HasEffect(50004) {
					t.Fatal("miss inflicted damage/effect or woke target")
				}
				want := []byte{50, 1, 17, 0, 4, 2, 80, 195, 0, 1, 2, 2, 0, 0, 1, 25, 0, 0, 0, 0, 1}
				if !bytes.Equal(steps[0].Packets[2], want) {
					t.Fatalf("miss record: %v", steps[0].Packets[2])
				}
			} else if target.HP >= 1000 || !target.HasEffect(50000) || target.HasEffect(50004) {
				t.Fatal("single magic should hit and apply its effect")
			}
			target.RemoveEffects(50002)
			target.tickEffects()
			target.tickEffects()
			if r.attackMisses(f, target, 50000) {
				t.Fatal("expired protection remained")
			}
		})
	}
}

func TestBasicAndMonsterAttacksMissProtectedTargets(t *testing.T) {
	b, f, r := goddessFixture()
	protection := assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, MissPhysicalAttacks: true}
	b.Defenders[0].ApplyEffect(50000, 0, protection)
	f.Weapon = 5 // A basic staff strike uses MATK but is still a physical action.
	r.attack(b, []Action{{Actor: f, Kind: "attack", Skill: 10001, TX: 2, TY: 2}}, map[int]bool{})
	if b.Defenders[0].HP != 1000 {
		t.Fatal("basic strike bypassed protection")
	}
	f.ApplyEffect(50001, 0, protection)
	hp := f.HP
	steps := r.monster(b, b.Defenders[0], map[int]bool{})
	if f.HP != hp || len(steps) != 1 || steps[0].Packets[1][12] != 0 {
		t.Fatal("monster strike did not miss")
	}
	ally := &Fighter{Side: Defender, X: 2, Y: 3, HP: 100}
	ally.ApplyEffect(50002, 0, protection)
	b.Defenders = append(b.Defenders, ally)
	b.Defenders[0].ApplyEffect(50003, 0, assets.SkillEffect{Target: assets.EffectEnemy, Rounds: 2, AllyAttackChancePercent: 100})
	r.monster(b, b.Defenders[0], map[int]bool{})
	if ally.HP != 100 {
		t.Fatal("redirected monster strike bypassed protection")
	}
}

func TestMixedComboAppliesEffectsOnlyForLandedHits(t *testing.T) {
	b, f, r := goddessFixture()
	target := b.Defenders[0]
	target.ApplyEffect(50002, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, MissPhysicalAttacks: true})
	for i, layer := range []byte{1, 2} {
		id := uint16(50000 + i)
		onHit := blocking(2, false)
		onHit.OnHit = true
		r.Skills[id] = assets.Skill{ID: id, EffectLayer: layer, StatMultiplier: 2, Effects: []assets.SkillEffect{onHit}}
		f.Char.Skills = append(f.Char.Skills, game.LearnedSkill{ID: id, Grade: 1})
	}
	r.attack(b, []Action{{Actor: f, Kind: "attack", Skill: 50000, TX: 2, TY: 2}, {Actor: f, Kind: "attack", Skill: 50001, TX: 2, TY: 2}}, map[int]bool{})
	if target.HP >= 1000 || target.HasEffect(50000) || !target.HasEffect(50001) {
		t.Fatal("mixed combo applied missed attack effects")
	}
}

func TestAreaClassificationUsesCurrentPlayerAndPetGrade(t *testing.T) {
	b, f, r := goddessFixture()
	target := b.Defenders[0]
	target.ApplyEffect(50000, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, MissAreaAttacks: true})
	r.Skills[50001] = assets.Skill{ID: 50001, EffectLayer: 2, NativePattern109: 1, NativePattern110: 1, NativePattern111: 2, NativePattern112: 2}
	f.Char.Skills = append(f.Char.Skills, game.LearnedSkill{ID: 50001, Grade: 1})
	if r.attackMisses(f, target, 50001) {
		t.Fatal("low-grade single attack classified as area")
	}
	f.Char.Skills[len(f.Char.Skills)-1].Grade = 7
	if !r.attackMisses(f, target, 50001) {
		t.Fatal("high-grade area attack bypassed protection")
	}
	pet := &Fighter{Pet: &game.Pet{Skills: []game.PetSkill{{ID: 50001, Grade: 7}}}}
	if !r.attackMisses(pet, target, 50001) {
		t.Fatal("pet grade ignored")
	}
}

func TestStoneWallCastsOnAllyAndAllowsMagic(t *testing.T) {
	b, f, r := goddessFixture()
	wall := assets.Skill{ID: 50000, Name: "unrelated", SP: 110, EffectLayer: 4, NativeEffectCode52: 101, NativeRounds51: 3}
	assets.PopulateSkillEffects(&wall)
	r.Skills[wall.ID] = wall
	f.Char.Skills = append(f.Char.Skills, game.LearnedSkill{ID: wall.ID, Grade: 1})
	ally := &Fighter{Side: Attacker, X: 4, Y: 3, HP: 1000, MaxHP: 1000}
	b.Attackers = append(b.Attackers, ally)
	initialSP := f.SP
	b.Submit(r, 7, 1, []byte{4, 2, 4, 3, 80, 195})
	action := b.Pending[f.key()]
	if action.Kind != "effect" {
		t.Fatal("Stone Wall classified as attack", action)
	}
	step := r.abilityEffect(b, action)
	if ally.HP != 1000 || b.Defenders[0].HP != 1000 || !ally.HasEffect(wall.ID) || f.HasEffect(wall.ID) || f.SP != initialSP-110 || len(step.SkillUses) != 1 {
		t.Fatal("Stone Wall failed to protect selected ally without damage")
	}
	want := []byte{50, 1, 17, 0, 4, 2, 80, 195, 0, 1, 4, 3, 1, 0, 1, 0, 0, 0, 0, 0, 1}
	if !bytes.Equal(step.Packets[2], want) {
		t.Fatalf("Stone Wall animation: %v", step.Packets[2])
	}
	enemy := b.Defenders[0]
	// The monster's physical strike must miss.
	hit, _ := r.attack(b, []Action{{Actor: enemy, Kind: "attack", Skill: 10001, TX: 4, TY: 3}}, map[int]bool{})
	if ally.HP != 1000 || hit[0].Packets[1][12] != 0 {
		t.Fatal("physical strike bypassed Stone Wall")
	}
	for _, area := range []bool{false, true} {
		r.Skills[50001] = assets.Skill{ID: 50001, EffectLayer: 2, StatMultiplier: 2, AreaAttack: &area}
		hp := ally.HP
		r.attack(b, []Action{{Actor: enemy, Kind: "attack", Skill: 50001, TX: 4, TY: 3}}, map[int]bool{})
		if ally.HP >= hp {
			t.Fatal("Stone Wall blocked magical attack", area)
		}
	}
	for i := 0; i < wall.Effects[0].Rounds; i++ {
		ally.tickEffects()
	}
	if r.attackMisses(enemy, ally, 10001) {
		t.Fatal("Stone Wall protection outlived duration")
	}
	// Invalid opposing-side targets do not consume SP or proficiency.
	before := f.SP
	rejected := r.abilityEffect(b, Action{Actor: f, Skill: wall.ID, TX: 2, TY: 2})
	if f.SP != before || len(rejected.SkillUses) != 0 || enemy.HasEffect(wall.ID) {
		t.Fatal("Stone Wall accepted an enemy target")
	}
}

func TestWaterShieldReducesOnlyMagicalAttackDamage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		skill uint16
		layer byte
		area  bool
	}{
		{"basic", 10001, 1, false}, {"staff", 10001, 1, false}, {"physical", 50001, 1, false}, {"physical area", 50001, 1, true},
		{"single magic", 50001, 2, false}, {"area magic", 50001, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, f, r := goddessFixture()
			f.HP, f.MaxHP = 1000, 1000
			enemy := b.Defenders[0]
			enemy.Atk, enemy.Matk = 100, 100
			if tc.name == "staff" {
				enemy.Weapon = 5
			}
			r.Skills[tc.skill] = assets.Skill{ID: tc.skill, EffectLayer: tc.layer, StatMultiplier: 2, AreaAttack: &tc.area}
			a := Action{Actor: enemy, Kind: "attack", Skill: tc.skill, TX: 4, TY: 2}
			r.attack(b, []Action{a}, map[int]bool{})
			damage := 1000 - f.HP
			f.HP = 1000
			shield := assets.Skill{ID: 50000, Name: "unrelated", SP: 35, EffectLayer: 4, NativeEffectCode52: 105, NativeRounds51: 4}
			assets.PopulateSkillEffects(&shield)
			r.Skills[shield.ID] = shield
			f.Char.Skills = append(f.Char.Skills, game.LearnedSkill{ID: shield.ID, Grade: 1})
			step := r.abilityEffect(b, Action{Actor: f, Kind: "effect", Skill: shield.ID, TX: 4, TY: 2})
			if f.SP != 265 || f.HP != 1000 || len(step.SkillUses) != 1 {
				t.Fatal("Water Shield cast changed cost, HP or proficiency")
			}
			r.attack(b, []Action{a}, map[int]bool{})
			want := damage
			if tc.layer == 2 {
				want = max(1, damage/2)
			}
			if got := 1000 - f.HP; got != want {
				t.Fatal("wrong attack-type reduction", got, want)
			}
			// Periodic damage has no attack category and remains unchanged.
			f.ApplyEffect(50003, 0, assets.SkillEffect{Target: assets.EffectEnemy, Rounds: 2, PeriodicDamage: &assets.PeriodicDamage{Flat: 20}})
			if f.periodicDamage() != 20 {
				t.Fatal("Water Shield reduced periodic damage")
			}
			for i := 0; i < 5; i++ {
				f.tickEffects()
			}
			if got := r.incomingDamage(f, tc.skill, 100); got != 100 {
				t.Fatal("expired Water Shield still applied", got)
			}
		})
	}
}

func TestMagicalReductionAggregatesWithGenericProtection(t *testing.T) {
	r := lowest
	r.Skills = map[uint16]assets.Skill{50000: {ID: 50000, EffectLayer: 2}, 50001: {ID: 50001, EffectLayer: 1}}
	f := &Fighter{HP: 100}
	f.ApplyEffect(50002, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, StackGroup: "protection", Modifiers: []assets.StatModifier{{Stat: assets.EffectMagicalDamageTaken, Percent: -50}}})
	f.ApplyEffect(50003, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, StackGroup: "protection", Modifiers: []assets.StatModifier{{Stat: assets.EffectDamageTaken, Percent: -30}}})
	if got := r.incomingDamage(f, 50000, 100); got != 50 {
		t.Fatal("same-group protections compounded", got)
	}
	if got := r.incomingDamage(f, 50001, 100); got != 70 {
		t.Fatal("generic physical protection lost", got)
	}
	f.ApplyEffect(50004, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, Modifiers: []assets.StatModifier{{Stat: assets.EffectDamageTaken, Percent: -10}}})
	if got := r.incomingDamage(f, 50000, 101); got != 40 {
		t.Fatal("independent modifiers failed to aggregate before rounding", got)
	}
	f.tickEffects()
	f.tickEffects()
	if got := r.incomingDamage(f, 50000, 100); got != 100 {
		t.Fatal("expired modifiers remained", got)
	}
}

func TestShieldDefenseBuffsDefenseAgainstAllAttackCategories(t *testing.T) {
	for _, tc := range []struct {
		name  string
		skill uint16
		layer byte
		area  bool
	}{
		{"basic", 10001, 1, false}, {"staff", 10001, 1, false}, {"physical", 50001, 1, false}, {"physical area", 50001, 1, true},
		{"single magic", 50001, 2, false}, {"area magic", 50001, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, f, r := goddessFixture()
			f.HP, f.MaxHP, f.Def, f.Mdef = 1000, 1000, 100, 100
			enemy := b.Defenders[0]
			enemy.Atk, enemy.Matk = 100, 100
			if tc.name == "staff" {
				enemy.Weapon = 5
			}
			r.Skills[tc.skill] = assets.Skill{ID: tc.skill, EffectLayer: tc.layer, StatMultiplier: 2, AreaAttack: &tc.area}
			a := Action{Actor: enemy, Kind: "attack", Skill: tc.skill, TX: 4, TY: 2}
			r.attack(b, []Action{a}, map[int]bool{})
			damage := 1000 - f.HP
			f.HP = 1000
			shield := assets.Skill{ID: 50000, Name: "unrelated", SP: 15, EffectLayer: 19, NativeEffectCode52: 52, NativeRounds51: 3}
			assets.PopulateSkillEffects(&shield)
			r.Skills[shield.ID] = shield
			f.Char.Skills = append(f.Char.Skills, game.LearnedSkill{ID: shield.ID, Grade: 1})
			step := r.abilityEffect(b, Action{Actor: f, Kind: "effect", Skill: shield.ID, TX: 4, TY: 2})
			if f.SP != 285 || f.HP != 1000 || len(step.SkillUses) != 1 {
				t.Fatal("Shield Defense cast changed cost, HP or proficiency")
			}
			r.attack(b, []Action{a}, map[int]bool{})
			want := damage - 5
			if Basic(tc.skill) {
				want = damage - 10
			}
			if f.Def != 100 || f.Mdef != 100 || f.defense() != 110 || f.magicDefense() != 110 {
				t.Fatal("stat buff overwrote base or failed")
			}
			if got := 1000 - f.HP; got != want {
				t.Fatal("wrong attack-type reduction", got, want)
			}
			// Periodic damage has no attack category and remains unchanged.
			f.ApplyEffect(50003, 0, assets.SkillEffect{Target: assets.EffectEnemy, Rounds: 2, PeriodicDamage: &assets.PeriodicDamage{Flat: 20}})
			if f.periodicDamage() != 20 {
				t.Fatal("Shield Defense reduced periodic damage")
			}
			for i := 0; i < 4; i++ {
				f.tickEffects()
			}
			if f.defense() != 100 || f.magicDefense() != 100 {
				t.Fatal("defense buff did not expire")
			}
			if got := r.incomingDamage(f, tc.skill, 100); got != 100 {
				t.Fatal("expired Shield Defense still applied", got)
			}
		})
	}
}
