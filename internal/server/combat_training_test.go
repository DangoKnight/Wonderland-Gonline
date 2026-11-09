package server

import (
	"testing"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
)

func TestTrainingVictoryBonusBypassesMultipliers(t *testing.T) {
	for _, formula := range []game.BattleEXPFormula{game.BattleEXPSimple, game.BattleEXPParticipation} {
		for _, outcome := range []battle.Outcome{battle.Victory, battle.Fled, battle.Defeat} {
			s, c, _, _ := participationVictoryFixture(t)
			run := c.battle
			s.battleEXPFormula = formula
			s.expRateMultiplier = 3
			run.backdropLocation.Map = 13056
			s.Assets.Maps[13056] = assets.Map{BattleVictoryEXP: 200, BattleRoundRecoveryHP: 100, BattleRoundRecoverySP: 50}
			if rules := s.combatRules(run); rules.RoundRecoveryHP != 100 || rules.RoundRecoverySP != 50 {
				t.Fatal("battle did not resolve its original map recovery")
			}
			regular, _ := run.b.Rewards()
			m := run.member(c)
			beforePlayer, beforePet := uint64(c.character.EXP), uint64(c.character.Pets[0].ClientTotalExp())
			wantPlayer, wantPet := beforePlayer, beforePet
			if outcome == battle.Victory {
				if formula == game.BattleEXPSimple {
					wantPlayer += regular*3 + 200
					wantPet += regular*3 + 200
				} else {
					// Levels 10 and 20 split the unscaled 400 bonus as 267/133.
					wantPlayer += run.b.ParticipantEXP(m.self)*3 + 267
					wantPet += run.b.ParticipantEXP(m.pet)*3 + 133
				}
			}
			s.endBattle(run, outcome)
			settle(t, s, c)
			if uint64(c.character.EXP) != wantPlayer || uint64(c.character.Pets[0].ClientTotalExp()) != wantPet {
				t.Fatal(formula, outcome, "flat EXP was multiplied, omitted or granted without victory", c.character.EXP, c.character.Pets[0].ClientTotalExp(), wantPlayer, wantPet)
			}
		}
	}
}

func TestTrainingSimpleBonusIncludesNonContributingPlayerAndPet(t *testing.T) {
	s, c, _ := battlePetFixture(t)
	run := c.battle
	s.battleEXPFormula = game.BattleEXPSimple
	s.expRateMultiplier = 3
	run.backdropLocation.Map = 13056
	s.Assets.Maps[13056] = assets.Map{BattleVictoryEXP: 200}
	m := run.member(c)
	// The fixture's other player performs the only attack; this session's
	// player and active pet neither attack nor assist.
	killer := run.b.Attackers[len(run.b.Attackers)-1]
	killer.Side = battle.Attacker
	killer.Char = &game.Character{ID: killer.ID}
	killer.Atk, killer.Spd = 10000, 1000
	target := run.b.Defenders[0]
	target.HP = 1
	rules := s.combatRules(run)
	rules.Next = func(lo, hi int) int { return lo }
	rules.Float = func() float64 { return 0 }
	if ack := run.b.Submit(rules, killer.ID, 1, []byte{killer.X, killer.Y, target.X, target.Y, 17, 39}); ack == nil {
		t.Fatal("killer's action was rejected")
	}
	_, outcome := run.b.Round(rules)
	if outcome != battle.Victory || run.b.ParticipantEXP(m.self) != 0 || run.b.ParticipantEXP(m.pet) != 0 {
		t.Fatal("fixture must win without this player's or pet's contribution", outcome)
	}
	regular, _ := run.b.Rewards()
	playerBefore, petBefore := uint64(c.character.EXP), uint64(c.character.Pets[0].ClientTotalExp())
	benchBefore := c.character.Pets[1].ClientTotalExp()
	s.endBattle(run, outcome)
	settle(t, s, c)
	if uint64(c.character.EXP) != playerBefore+regular*3+200 || uint64(c.character.Pets[0].ClientTotalExp()) != petBefore+regular*3+200 {
		t.Fatal("non-contributors did not receive the full unscaled bonus")
	}
	if c.character.Pets[1].ClientTotalExp() != benchBefore {
		t.Fatal("a pet outside battle received the bonus")
	}
}
