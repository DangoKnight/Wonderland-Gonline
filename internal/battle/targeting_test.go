package battle

import (
	"bytes"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

func targetingRules() Rules {
	r := lowest
	s := r.Skills[11001]
	s.Targeting = []assets.SkillTargeting{{MinGrade: 1, MaxGrade: 1, Offsets: []assets.FormationOffset{{}}}, {MinGrade: 2, MaxGrade: 10, All: true}}
	r.Skills = map[uint16]assets.Skill{11001: s}
	return r
}
func TestAreaAttackCostProtectionEffectsAndNativeRecord(t *testing.T) {
	r := targetingRules()
	b, actor := setup(1000)
	first := b.Defenders[0]
	guarded := *first
	guarded.X = 1
	guarded.Y = 1
	vanished := *first
	vanished.X = 2
	vanished.Y = 3
	vanished.Effects = []ActiveEffect{{Turns: 3, Definition: assets.SkillEffect{MissAreaAttacks: true}}}
	dead := *first
	dead.X = 1
	dead.Y = 4
	dead.HP = 0
	b.Defenders = append(b.Defenders, &guarded, &vanished, &dead)
	s := r.Skills[11001]
	s.Effects = []assets.SkillEffect{{Target: assets.EffectEnemy, Rounds: 3, OnHit: true, Modifiers: []assets.StatModifier{{Stat: assets.EffectDEF, Percent: -10}}}}
	r.Skills[11001] = s
	a := Action{Actor: actor, Skill: 11001, Kind: "attack", TX: first.X, TY: first.Y}
	if r.comboEligible(a) {
		t.Fatal("area joined combo")
	}
	steps, _ := r.attack(b, []Action{a}, map[int]bool{guarded.key(): true})
	if len(steps) != 1 || actor.SP != 40 || len(steps[0].SkillUses) != 1 || first.HP >= 1000 || guarded.HP >= 1000 || vanished.HP != 1000 || dead.HP != 0 {
		t.Fatal("area execution", steps, actor.SP)
	}
	if len(first.Effects) != 1 || len(guarded.Effects) != 1 || len(vanished.Effects) != 1 {
		t.Fatal("on-hit effects")
	}
	packet := steps[0].Packets[2]
	// Independent actor record: 6-byte header + three 11-byte target results.
	if packet[2] != 39 || packet[3] != 0 || packet[9] != 3 || len(packet) != 43 || packet[10] != 2 || packet[11] != 2 || packet[23] != 1 || packet[34] != 0 {
		t.Fatal("wire", packet)
	}
	if int(1000-guarded.HP)*2 > 1000-first.HP+1 {
		t.Fatal("guard did not reduce damage")
	}
	// Grade one preserves the original single-target result exactly.
	actor.Char.Skills[0].Grade = 1
	if !r.comboEligible(a) {
		t.Fatal("single grade blocked combo")
	}
	result := targetResult{target: first, result: protocol.BattleHitLanded, stat: game.StatCurrentHP, amount: 42, mode: 1}
	if !bytes.Equal(areaRecord(protocol.Builder{50, 1}, actor, 11001, []targetResult{result}), hitRecord(protocol.Builder{50, 1}, actor, first, 11001, false, 25, 42, 1)) {
		t.Fatal("single wire changed")
	}
}
func TestAreaSupportSideAndDeathRules(t *testing.T) {
	r := targetingRules()
	s := r.Skills[11001]
	s.Name = "Cure"
	r.Skills[11001] = s
	b, actor := setup(1000)
	ally := *actor
	ally.X = 3
	ally.Y = 1
	ally.HP = 1
	dead := *actor
	dead.X = 3
	dead.Y = 3
	dead.HP = 0
	b.Attackers = append(b.Attackers, &ally, &dead)
	a := Action{Actor: actor, Skill: 11001, Kind: "heal", TX: actor.X, TY: actor.Y}
	step := r.support(b, a)
	if actor.SP != 40 || ally.HP <= 1 || dead.HP != 0 || b.Defenders[0].HP != 1000 || len(step.SkillUses) != 1 {
		t.Fatal("heal recipients/cost")
	}
	// An area heal aimed at a dead ally heals only its living neighbors.
	a.TX, a.TY = dead.X, dead.Y
	before := actor.SP
	if step := r.support(b, a); len(step.Packets) == 0 || actor.SP != before-10 || dead.HP != 0 {
		t.Fatal("heal revived corpse")
	}
	before = actor.SP
	s.Name = "Revival"
	r.Skills[11001] = s
	if step := r.support(b, a); dead.HP == 0 || actor.SP != before-10 || len(step.SkillUses) != 1 {
		t.Fatal("revival failed")
	}
	// Reversed PvP formations keep selection on the actor's own side.
	shape := assets.SkillTargeting{All: true}
	if got := formationTargets(b.Defenders, b.Defenders[0], shape, false); len(got) != 1 || got[0].Side != Defender {
		t.Fatal("side leak")
	}
}
func TestAreaDebuffsRespectVanishAndApplySelfOnce(t *testing.T) {
	r := targetingRules()
	b, actor := setup(1000)
	first := b.Defenders[0]
	other := *first
	other.X = 1
	other.Y = 1
	other.Effects = []ActiveEffect{{Turns: 2, Definition: assets.SkillEffect{MissAreaAttacks: true}}}
	b.Defenders = append(b.Defenders, &other)
	s := r.Skills[11001]
	s.Effects = []assets.SkillEffect{{Target: assets.EffectEnemy, Rounds: 2, BlockActions: true}, {Target: assets.EffectSelf, Rounds: 2, Modifiers: []assets.StatModifier{{Stat: assets.EffectATK, Flat: 10}}}}
	r.Skills[11001] = s
	step := r.abilityEffect(b, Action{Actor: actor, Skill: 11001, Kind: "effect", TX: first.X, TY: first.Y})
	if first.CanAct() || !other.CanAct() || len(actor.Effects) != 1 || actor.SP != 40 || len(step.SkillUses) != 1 {
		t.Fatal("area effect recipients")
	}
}

func TestUnconfiguredMixedEffectsPreserveSingleRecipientAnimations(t *testing.T) {
	r := targetingRules()
	b, actor := setup(1000)
	target := b.Defenders[0]
	skill := r.Skills[11001]
	skill.Targeting = nil
	skill.Effects = []assets.SkillEffect{{Target: assets.EffectEnemy, Rounds: 2, BlockActions: true}, {Target: assets.EffectSelf, Rounds: 2, Modifiers: []assets.StatModifier{{Stat: assets.EffectATK, Flat: 10}}}}
	r.Skills[11001] = skill
	step := r.abilityEffect(b, Action{Actor: actor, Skill: 11001, TX: target.X, TY: target.Y})
	var animations [][]byte
	for _, p := range step.Packets {
		if len(p) > 2 && p[0] == 50 && p[1] == 1 {
			animations = append(animations, p)
		}
	}
	if len(animations) != 2 {
		t.Fatal("legacy animation count", animations)
	}
	for _, p := range animations {
		if len(p) != 21 || p[2] != 17 || p[9] != 1 {
			t.Fatal("legacy single result layout changed", p)
		}
	}
}
