package battle

import (
	"math"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func experienceParty() (*Fighter, *Fighter, *Fighter) {
	return &Fighter{Kind: Player, ID: 1, Side: Attacker, Level: 10, HP: 100, MaxHP: 100, X: 4, Y: 1},
		&Fighter{Kind: Player, ID: 2, Side: Attacker, Level: 20, HP: 100, MaxHP: 100, X: 4, Y: 2},
		&Fighter{Kind: Pet, ID: 10501, Owner: 1, Side: Attacker, Level: 30, HP: 100, MaxHP: 100, X: 3, Y: 1}
}

func TestParticipationExperienceWholeTeamContributors(t *testing.T) {
	for _, lowLevelContributes := range []bool{false, true} {
		t.Run(map[bool]string{false: "six_contributors_idle_teammate", true: "seven_contributors"}[lowLevelContributes], func(t *testing.T) {
			var team []*Fighter
			var actors []Action
			for owner := uint32(1); owner <= 3; owner++ {
				player := &Fighter{Kind: Player, ID: owner, Side: Attacker, Level: 100}
				// Identical pet templates still represent three independent owned pets.
				pet := &Fighter{Kind: Pet, ID: 10501, Owner: owner, Side: Attacker, Level: 100}
				team = append(team, player, pet)
				actors = append(actors, Action{Actor: player}, Action{Actor: pet})
			}
			lowLevel := &Fighter{Kind: Player, ID: 4, Side: Attacker, Level: 1}
			team = append(team, lowLevel)
			if lowLevelContributes {
				actors = append(actors, Action{Actor: lowLevel})
			}
			var contributors []*Fighter
			for _, action := range actors {
				contributors = append(contributors, action.Actor)
			}
			shares := divideWeightedKillExperience(150, contributors, nil, len(actors))
			var pool uint64
			for _, share := range shares {
				pool += share
			}
			wantPool, wantLowLevel := uint64(1620), uint64(0) // 6 * 150 * 1.80.
			if lowLevelContributes {
				wantPool, wantLowLevel = 1995, 332 // 7 * 150 * 1.90; inverse-level sharing.
			}
			if pool != wantPool || shares[lowLevel.experienceID()] != wantLowLevel || len(shares) != len(actors) {
				t.Fatalf("pool=%d low-level=%d recipients=%d", pool, shares[lowLevel.experienceID()], len(shares))
			}
			b := &Battle{Attackers: team}
			b.recordKillExperience(&Fighter{Kind: Monster, Side: Defender, Level: 100}, actors)
			for _, fighter := range team[:6] {
				want := uint64(540)
				if lowLevelContributes {
					want = 554
					if fighter == team[0] || fighter == team[2] {
						want = 555 // Stable identity breaks equal fractional remainders.
					}
				}
				if got := b.ParticipantEXP(fighter); got != want {
					t.Fatalf("fighter %v: EXP=%d want=%d", fighter.experienceID(), got, want)
				}
			}
			wantEarnedLowLevel := uint64(0)
			if lowLevelContributes {
				wantEarnedLowLevel = 1991 // Apply the individual level multiplier after sharing.
			}
			if got := b.ParticipantEXP(lowLevel); got != wantEarnedLowLevel {
				t.Fatalf("low-level EXP=%d want=%d", got, wantEarnedLowLevel)
			}
		})
	}
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
		wantA, wantKiller := uint64(75), uint64(152) // Half/full credits, then level-10 vs level-25 scaling.
		if full {
			wantA, wantKiller = 213, 213
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
	if b.ParticipantEXP(a) != 152 || b.ParticipantEXP(killer) != 152 || b.ParticipantEXP(pet) != 0 {
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
		level         byte
		participation float64
		simple        uint64
	}{
		{0, 3, 10}, {1, 3, 15}, {10, 30, 150}, {25, 75, 375}, {255, 765, 3825},
	} {
		if got := participationMonsterExperience(tc.level); got != tc.participation {
			t.Fatal("participation base", tc, got)
		}
		if got := MonsterExperience(tc.level); got != tc.simple {
			t.Fatal("simple base changed", tc, got)
		}
	}
}

func TestParticipationExperienceLevelCurve(t *testing.T) {
	for _, tc := range []struct {
		participant, monster byte
		want                 float64
	}{
		{10, 20, 1.75}, {20, 20, 1}, {30, 20, 0.75},
		{40, 20, 0.5 + 2.5/17}, {0, 0, 1},
	} {
		if got := participationLevelMultiplier(tc.participant, tc.monster); math.Abs(got-tc.want) > 1e-12 {
			t.Fatal(tc, got)
		}
	}
	if got := participationLevelMultiplier(1, 255); got < 2.999 || got >= 3 {
		t.Fatal("lower-level asymptote", got)
	}
	if got := participationLevelMultiplier(255, 1); got <= 0.5 || got > 0.501 {
		t.Fatal("higher-level asymptote", got)
	}
	previous := 3.0
	for level := 1; level <= 255; level++ {
		got := participationLevelMultiplier(byte(level), 25)
		if got >= previous || got <= 0.5 || got >= 3 {
			t.Fatal("curve must decrease smoothly within bounds", level, got, previous)
		}
		previous = got
	}
}

func TestParticipationExperienceSoloLevelScaling(t *testing.T) {
	for _, tc := range []struct {
		level byte
		want  uint64
	}{{10, 105}, {20, 60}, {30, 45}} {
		actor := &Fighter{Kind: Player, ID: 1, Level: tc.level}
		b := &Battle{}
		b.recordKillExperience(&Fighter{Kind: Monster, Side: Defender, Level: 20}, []Action{{Actor: actor}})
		if got := b.ParticipantEXP(actor); got != tc.want {
			t.Fatal(tc, got)
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
	if total := b.ParticipantEXP(assistant) + b.ParticipantEXP(player) + b.ParticipantEXP(pet); total != 315 {
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
	if b.ParticipantEXP(assistant) != 184 || b.ParticipantEXP(player) != 84 || b.ParticipantEXP(pet) != 47 {
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
	if b.ParticipantEXP(assistant) != 184 {
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
	if b.ParticipantEXP(assistant) != 184 || b.ParticipantEXP(player) != 84 || b.ParticipantEXP(pet) != 47 {
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
	if b.ParticipantEXP(assistant) != 120 || b.ParticipantEXP(player) != 39 {
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
