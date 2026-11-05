package battle

import (
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func experienceParty() (*Fighter, *Fighter, *Fighter) {
	return &Fighter{Kind: Player, ID: 1, Side: Attacker, Level: 10, HP: 100, MaxHP: 100, X: 4, Y: 1},
		&Fighter{Kind: Player, ID: 2, Side: Attacker, Level: 20, HP: 100, MaxHP: 100, X: 4, Y: 2},
		&Fighter{Kind: Pet, ID: 10501, Owner: 1, Side: Attacker, Level: 30, HP: 100, MaxHP: 100, X: 3, Y: 1}
}

func TestEarlierDamageHalfParticipationAndUpgrade(t *testing.T) {
	for _, full := range []bool{false, true} {
		a, killer, _ := experienceParty()
		a.Level, killer.Level = 10, 10
		b := &Battle{}
		target := &Fighter{Kind: Monster, Side: Defender, Level: 25, HP: 100}
		b.recordExperienceDamage(target, a, 5)
		b.recordExperienceDamage(target, a, 5) // Repeated damage never stacks credits.
		target.HP = 0
		actors := []Action{{Actor: killer}}
		if full {
			actors = append(actors, Action{Actor: a})
		}
		b.recordKillExperience(target, actors)
		wantA, wantKiller := uint64(100), uint64(200) // 200 * 1.5 pool, equal levels weighted 1:2.
		if full {
			wantA, wantKiller = 280, 280
		} // Two full comboers, +40%.
		if b.ParticipantEXP(a) != wantA || b.ParticipantEXP(killer) != wantKiller {
			t.Fatal(full, b.experience)
		}
		other := &Fighter{Kind: Monster, Side: Defender, Level: 25}
		b.recordKillExperience(other, []Action{{Actor: killer}})
		if b.ParticipantEXP(a) != wantA {
			t.Fatal("damage credit leaked to another monster")
		}
	}
}

func TestEarlierDamageEligibilityAndSupportUpgrade(t *testing.T) {
	a, killer, pet := experienceParty()
	a.Level, killer.Level, pet.Level = 10, 10, 10
	b := &Battle{}
	target := &Fighter{Kind: Monster, Side: Defender, Level: 25, HP: 100}
	b.recordExperienceDamage(target, a, 0)
	if len(b.experienceDamage) != 0 {
		t.Fatal("zero damage credited")
	}
	b.recordExperienceDamage(target, a, 1)
	b.recordExperienceDamage(target, pet, 1)
	pet.Withdrawn = true
	target.HP = 0
	b.recordKillExperience(target, []Action{{Actor: killer, supporters: []*Fighter{a}}})
	if b.ParticipantEXP(a) != 200 || b.ParticipantEXP(killer) != 200 || b.ParticipantEXP(pet) != 0 {
		t.Fatal(b.experience)
	}
}

func TestEarlierDamageRecordedByNonlethalAttack(t *testing.T) {
	b, r, actions := comboFixture(100, 100)
	r.ExperienceFormula = game.BattleEXPParticipation
	target := b.Defenders[0]
	target.HP, target.MaxHP = 100000, 100000
	r.attack(b, actions[:1], nil)
	if len(b.experienceDamage[target]) != 1 || len(b.experience) != 0 {
		t.Fatal("nonlethal attack not tracked")
	}
	target.HP = 1
	r.attack(b, actions[1:], nil)
	if b.ParticipantEXP(actions[0].Actor) == 0 || b.ParticipantEXP(actions[1].Actor) == 0 {
		t.Fatal("earlier attacker lost participation", b.experience)
	}
}

func TestParticipationExperienceBaseRateIsSeparateFromSimple(t *testing.T) {
	for _, tc := range []struct {
		level                 byte
		participation, simple uint64
	}{
		{0, 8, 10}, {1, 8, 15}, {10, 80, 150}, {25, 200, 375}, {255, 2040, 3825},
	} {
		if got := participationMonsterExperience(tc.level); got != tc.participation {
			t.Fatal("participation base", tc, got)
		}
		if got := MonsterExperience(tc.level); got != tc.simple {
			t.Fatal("simple base changed", tc, got)
		}
	}
}

func TestParticipationExperienceWeightedPoolAndRemainders(t *testing.T) {
	assistant, player, pet := experienceParty()
	shares := divideKillExperience(200, []*Fighter{player, pet, assistant, assistant, player}, 2)
	for _, tc := range []struct {
		f    *Fighter
		want uint64
	}{{assistant, 350}, {player, 280}, {pet, 210}} {
		if got := shares[tc.f.experienceID()]; got != tc.want {
			t.Fatal("weighted contribution", got, tc.want)
		}
	}
	assistant.Level, player.Level, pet.Level = 1, 1, 5
	shares = divideKillExperience(2, []*Fighter{pet, assistant, player}, 1)
	total := uint64(0)
	for _, share := range shares {
		total += share
	}
	if total != 6 || shares[pet.experienceID()] != 1 || shares[assistant.experienceID()] != 3 {
		t.Fatal("remainder lost or unstable", shares)
	}
	if solo := divideKillExperience(200, []*Fighter{assistant}, 1); solo[assistant.experienceID()] != 200 {
		t.Fatal("solo lost EXP", solo)
	}
}

func TestParticipationExperienceComboPoolBonus(t *testing.T) {
	for _, tc := range []struct {
		participants, combo int
		want                uint64
	}{
		{1, 1, 200}, {3, 1, 600}, {2, 2, 560}, {3, 2, 840}, {3, 3, 900}, {4, 4, 1280}, {8, 8, 3200},
	} {
		if got := killExperiencePool(200, tc.participants, tc.combo); got != tc.want {
			t.Fatal("combo pool", tc, got)
		}
	}
	if got := killExperiencePool(15, 3, 3); got != 68 {
		t.Fatal("fractional pool rounding", got)
	}
	assistant, player, pet := experienceParty()
	b := &Battle{}
	target := &Fighter{Kind: Monster, Side: Defender, Level: 20}
	b.recordKillExperience(target, []Action{{Actor: player, supporters: []*Fighter{assistant}}, {Actor: player}, {Actor: pet, supporters: []*Fighter{assistant}}})
	if total := b.ParticipantEXP(assistant) + b.ParticipantEXP(player) + b.ParticipantEXP(pet); total != 672 {
		t.Fatal("duplicate actor or assistant increased combo bonus", total)
	}
}

func TestParticipationExperienceBuffAndHealLifetimes(t *testing.T) {
	assistant, player, pet := experienceParty()
	buff := assets.SkillEffect{Target: assets.EffectAlly, Rounds: 2, Modifiers: []assets.StatModifier{{Stat: assets.EffectATK, Flat: 10}}}
	pet.applyEffectFrom(assistant, 50001, 0, buff)
	pet.tickEffects()
	player.recordImmediateSupport(assistant)
	actors := []Action{{Actor: pet, supporters: pet.consumeSupport()}, {Actor: player, supporters: player.consumeSupport()}}
	b := &Battle{}
	target := &Fighter{Kind: Monster, Side: Defender, Level: 20, HP: 0}
	b.recordKillExperience(target, actors)
	if b.ParticipantEXP(assistant) != 280 || b.ParticipantEXP(player) != 224 || b.ParticipantEXP(pet) != 168 {
		t.Fatal("assistant not deduplicated across comboers", b.experience)
	}
	if len(player.consumeSupport()) != 0 || len(pet.consumeSupport()) != 1 {
		t.Fatal("heal consumed or active buff lost")
	}
	pet.tickEffects()
	if len(pet.consumeSupport()) != 0 {
		t.Fatal("expired buff still grants assistance")
	}
	b.recordKillExperience(target, actors)
	if b.ParticipantEXP(assistant) != 280 {
		t.Fatal("kill awarded twice")
	}
}

func TestParticipationExperienceRemovedBuffAndSpentHeal(t *testing.T) {
	assistant, player, _ := experienceParty()
	player.applyEffectFrom(assistant, 50001, 0, assets.SkillEffect{Target: assets.EffectAlly, Rounds: 3, Modifiers: []assets.StatModifier{{Stat: assets.EffectATK, Flat: 10}}})
	player.RemoveEffects(50001)
	if len(player.consumeSupport()) != 0 {
		t.Fatal("removed buff credited")
	}
	player.recordImmediateSupport(assistant)
	if len(player.consumeSupport()) != 1 || len(player.consumeSupport()) != 0 {
		t.Fatal("heal did not attach to next attack only")
	}
	assistant.Withdrawn = true
	player.recordImmediateSupport(assistant)
	if len(player.consumeSupport()) != 0 {
		t.Fatal("withdrawn assister credited")
	}
}

func TestParticipationExperienceSupportExecutionAndNoOp(t *testing.T) {
	assistant, player, _ := experienceParty()
	b := New([]*Fighter{assistant, player}, []Enemy{{Template: 10500, HP: 1000}})
	r := lowest
	r.Skills = map[uint16]assets.Skill{50002: {ID: 50002, Name: "Heal", EffectLayer: 3}}
	player.HP = 50
	r.support(b, Action{Actor: assistant, Skill: 50002, TX: player.X, TY: player.Y})
	if len(player.consumeSupport()) != 1 {
		t.Fatal("successful heal not credited")
	}
	r.support(b, Action{Actor: assistant, Skill: 50002, TX: player.X, TY: player.Y})
	if len(player.consumeSupport()) != 0 {
		t.Fatal("full-HP no-op heal credited")
	}
}

func TestParticipationExperienceRecordsKillingStrikeOnly(t *testing.T) {
	b, r, actions := comboFixture(100, 100)
	r.ExperienceFormula = game.BattleEXPParticipation
	b.Defenders[0].HP, b.Defenders[0].MaxHP = 1, 1000
	r.attack(b, actions, nil)
	if b.ParticipantEXP(actions[0].Actor) == 0 || b.ParticipantEXP(actions[1].Actor) == 0 {
		t.Fatal("killing combo absent", b.experience)
	}
	b2, r2, actions2 := comboFixture(100)
	r2.ExperienceFormula = game.BattleEXPParticipation
	r2.attack(b2, actions2, nil)
	if len(b2.experience) != 0 {
		t.Fatal("nonlethal attack credited")
	}
}

func TestParticipationExperienceBuffThenHealAndKillingCombo(t *testing.T) {
	assistant, player, pet := experienceParty()
	player.HP = 50
	player.Spd, pet.Spd = 100, 100
	b := New([]*Fighter{assistant, player, pet}, []Enemy{{Template: 10500, Level: 20, HP: 1000}})
	r := lowest
	r.ExperienceFormula = game.BattleEXPParticipation
	r.Skills = map[uint16]assets.Skill{
		50002: {ID: 50002, Name: "Heal", EffectLayer: 3},
		50003: {ID: 50003, Effects: []assets.SkillEffect{{Target: assets.EffectAlly, Rounds: 2, Modifiers: []assets.StatModifier{{Stat: assets.EffectATK, Flat: 10}}}}},
	}
	r.abilityEffect(b, Action{Actor: assistant, Skill: 50003, TX: pet.X, TY: pet.Y})
	pet.tickEffects()
	r.support(b, Action{Actor: assistant, Skill: 50002, TX: player.X, TY: player.Y})
	target := b.Defenders[0]
	target.HP = 1
	r.attack(b, []Action{{Actor: player, Kind: "attack", Skill: 10001, TX: target.X, TY: target.Y}, {Actor: pet, Kind: "attack", Skill: 10001, TX: target.X, TY: target.Y}}, nil)
	if b.ParticipantEXP(assistant) != 280 || b.ParticipantEXP(player) != 224 || b.ParticipantEXP(pet) != 168 {
		t.Fatal("cast/strike assistance integration", b.experience)
	}
}

func TestParticipationExperienceReviveLastsThroughBattle(t *testing.T) {
	assistant, player, _ := experienceParty()
	b := New([]*Fighter{assistant, player}, []Enemy{{Template: 10500, HP: 1000}})
	r := lowest
	r.Skills = map[uint16]assets.Skill{50004: {ID: 50004, Name: "Revive", EffectLayer: assets.SkillLayerRevival}}
	player.HP = 0
	r.support(b, Action{Actor: assistant, Skill: 50004, TX: player.X, TY: player.Y})
	if player.HP <= 0 {
		t.Fatal("revive failed")
	}
	for kill := 0; kill < 3; kill++ {
		player.tickEffects()
		supporters := player.consumeSupport()
		if len(supporters) != 1 || supporters[0] != assistant {
			t.Fatal("revive assistance consumed", kill, supporters)
		}
		b.recordKillExperience(&Fighter{Kind: Monster, Side: Defender, Level: 10}, []Action{{Actor: player, supporters: supporters}})
	}
	if b.ParticipantEXP(assistant) != 321 || b.ParticipantEXP(player) != 159 {
		t.Fatal("reviver did not earn each later kill", b.experience)
	}
	player.HP = 0
	r.support(b, Action{Actor: assistant, Skill: 50004, TX: player.X, TY: player.Y})
	if len(player.consumeSupport()) != 1 {
		t.Fatal("repeat revive counted caster twice")
	}
}

func TestParticipationExperienceReviveNoOpAndPetIdentity(t *testing.T) {
	assistant, player, pet := experienceParty()
	b := New([]*Fighter{assistant, player, pet}, []Enemy{{Template: 10500, HP: 1000}})
	r := lowest
	r.Skills = map[uint16]assets.Skill{50004: {ID: 50004, Name: "Revive", EffectLayer: assets.SkillLayerRevival}}
	r.support(b, Action{Actor: assistant, Skill: 50004, TX: player.X, TY: player.Y})
	if len(player.consumeSupport()) != 0 {
		t.Fatal("living-target revive granted permanent assistance")
	}
	pet.HP = 0
	r.support(b, Action{Actor: assistant, Skill: 50004, TX: pet.X, TY: pet.Y})
	other := &Fighter{Kind: Pet, Owner: pet.Owner, ID: 10502, Side: Attacker, HP: 100}
	b.ReplacePet(assistant, pet, other)
	if len(other.consumeSupport()) != 0 {
		t.Fatal("replacement inherited revive credit")
	}
	returning := &Fighter{Kind: Pet, Owner: pet.Owner, ID: pet.ID, Side: Attacker, HP: 100}
	b.ReplacePet(assistant, other, returning)
	if len(returning.consumeSupport()) != 1 {
		t.Fatal("returning pet lost revive credit")
	}
	New([]*Fighter{returning}, nil)
	if len(returning.consumeSupport()) != 0 {
		t.Fatal("reused fighter retained revive credit in new battle")
	}
	fresh := PetFighter(game.Pet{ID: pet.ID, Level: 1, HP: 100}, pet.Owner, nil, 0)
	New([]*Fighter{fresh}, nil)
	if len(fresh.consumeSupport()) != 0 {
		t.Fatal("revive credit carried into new battle")
	}
}
