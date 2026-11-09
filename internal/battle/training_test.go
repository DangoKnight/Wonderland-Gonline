package battle

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/assets"
)

func TestTrainingRoundRecovery(t *testing.T) {
	for _, pvp := range []bool{false, true} {
		player, pet, dead := experienceParty()
		pet.Kind, pet.Owner = Pet, player.ID
		player.HP, player.MaxHP, player.SP, player.MaxSP = 10, 500, 0, 150
		pet.HP, pet.MaxHP, pet.SP, pet.MaxSP = 90, 100, 25, 30
		dead.HP = 0
		enemy := &Fighter{Kind: Monster, Side: Defender, HP: 10, MaxHP: 500, Effects: []ActiveEffect{{Turns: 2, Definition: assets.SkillEffect{BlockActions: true}}}}
		b := &Battle{Attackers: []*Fighter{player, pet, dead}, Defenders: []*Fighter{enemy}, PvP: pvp}
		steps, outcome := b.Round(Rules{RoundRecoveryHP: 100, RoundRecoverySP: 50})
		if outcome != Continue || dead.HP != 0 || enemy.HP != 10 {
			t.Fatal("recovery changed defeat state or enemy", outcome, dead, enemy)
		}
		if pvp {
			if player.HP != 10 || player.SP != 0 || len(steps) != 0 {
				t.Fatal("PvP received training recovery")
			}
			continue
		}
		if player.HP != 110 || player.SP != 50 || pet.HP != 100 || pet.SP != 30 {
			t.Fatal("uncapped/missing round recovery", player, pet)
		}
		if len(steps) != 1 || len(steps[0].Packets) != 4 || !bytes.Equal(steps[0].Packets[0], []byte{51, 1, 4, 1, 25, 110, 0, 0, 0}) {
			t.Fatal("missing native post-round HP/SP packets", steps)
		}
		player.HP, player.SP, enemy.HP = 10, 0, 0
		_, outcome = b.Round(Rules{RoundRecoveryHP: 100, RoundRecoverySP: 50})
		if outcome != Victory || player.HP != 110 || player.SP != 50 {
			t.Fatal("victorious final round missed recovery")
		}
	}
}

func TestTrainingParticipationBonusDeduplicatesContributors(t *testing.T) {
	a, killer, idle := experienceParty()
	b := &Battle{}
	dead := func(id uint32) *Fighter { return &Fighter{Kind: Monster, Side: Defender, ID: id, Level: 10} }
	// Prior damage earns a half-weight share, but still contributes one flat
	// 200-point award to the pool. An idle teammate contributes nothing.
	b.experienceDamage = map[*Fighter]map[experienceIdentity]*Fighter{}
	target := dead(100)
	b.experienceDamage[target] = map[experienceIdentity]*Fighter{a.experienceID(): a}
	b.recordKillExperience(target, []Action{{Actor: killer}})
	bonus := b.ParticipationBonus(200)
	if bonus.Participant(a) != 200 || bonus.Participant(killer) != 200 || bonus.Participant(idle) != 0 {
		t.Fatal("half-credit pool or idle member", bonus)
	}
	// Repeated kills do not create a second battle bonus. A later full
	// contribution replaces the earlier half weight.
	b.recordKillExperience(dead(101), []Action{{Actor: a}, {Actor: killer}})
	bonus = b.ParticipationBonus(200)
	if bonus.Participant(a) != 267 || bonus.Participant(killer) != 133 {
		t.Fatal("bonus was multiplied by kills/combo or lost inverse-level weights", bonus)
	}
	if b.ParticipationBonus(0).Participant(a) != 0 {
		t.Fatal("ordinary map got bonus")
	}
}

func TestTrainingExpiredContributorExcludedFromPoolAndShares(t *testing.T) {
	expired, eligible, _ := experienceParty()
	b := &Battle{EXPAllowed: func(owner uint32) bool { return owner == eligible.ID }}
	target := &Fighter{Kind: Monster, Side: Defender, Level: 10}
	b.recordKillExperience(target, []Action{{Actor: expired}, {Actor: eligible}})
	bonus := b.ParticipationBonus(200)
	if b.ParticipantEXP(expired) != 0 || b.ParticipantEXP(eligible) == 0 || bonus.Participant(expired) != 0 || bonus.Participant(eligible) != 200 {
		t.Fatal("ineligible contributor inflated or received rewards", bonus)
	}
}
