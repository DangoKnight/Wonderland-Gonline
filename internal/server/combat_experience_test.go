package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
)

func participationVictoryFixture(t *testing.T) (*Server, *Session, uint64, uint64) {
	t.Helper()
	s, c, _ := battlePetFixture(t)
	run := c.battle
	m := run.member(c)
	s.battleEXPFormula = game.BattleEXPParticipation
	s.Config.BattleEXPFormula = game.BattleEXPSimple // Captured startup policy wins.
	s.expRateMultiplier = 1
	m.self.Level, m.pet.Level = 10, 20
	m.self.Spd, m.pet.Spd = 100, 100
	m.self.Atk, m.pet.Atk = 1000, 1000
	target := run.b.Defenders[0]
	target.HP, target.MaxHP, target.Level = 1, 1000, 10
	r := s.combatRules(run)
	r.Next = func(lo, hi int) int { return lo }
	r.Float = func() float64 { return 0 }
	for _, actor := range []*battle.Fighter{m.self, m.pet} {
		run.b.Submit(r, c.character.ID, 1, []byte{actor.X, actor.Y, target.X, target.Y, 17, 39})
	}
	beforeEXP := c.character.EXP
	beforePetEXP := c.character.Pets[0].Exp
	run.b.Round(r)
	if c.character.EXP != beforeEXP || c.character.Pets[0].Exp != beforePetEXP {
		t.Fatal("EXP awarded before victory")
	}
	playerEXP, petEXP := run.b.ParticipantEXP(m.self), run.b.ParticipantEXP(m.pet)
	if playerEXP != 149 || petEXP != 75 {
		t.Fatal("inverse-level killing shares", playerEXP, petEXP)
	}
	return s, c, playerEXP, petEXP
}

func TestParticipationEXPVictoryOnlyAndSeparatePetShares(t *testing.T) {
	for _, outcome := range []battle.Outcome{battle.Victory, battle.Fled, battle.Defeat} {
		s, c, playerEXP, petEXP := participationVictoryFixture(t)
		before := c.character.Clone()
		run := c.battle
		if c.character.EXP != before.EXP {
			t.Fatal("EXP awarded before victory")
		}
		// Bench the earning pet before settlement; its reward belongs to that
		// pet identity, not to the newly active companion.
		if err := s.battlePetCommand(context.Background(), c, []byte{19, 1, 6, 41, 0, 0}); err != nil {
			t.Fatal(err)
		}
		s.endBattle(run, outcome)
		settle(t, s, c)
		stored, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil {
			t.Fatal(err)
		}
		wantPlayer, wantPet := uint64(before.EXP), before.Pets[0].ClientTotalExp()
		if outcome == battle.Victory {
			wantPlayer += playerEXP
			wantPet += uint32(petEXP)
		}
		if uint64(stored[0].EXP) != wantPlayer || stored[0].Pets[0].ClientTotalExp() != wantPet || stored[0].Pets[1].ClientTotalExp() != before.Pets[1].ClientTotalExp() {
			t.Fatal("victory-only EXP or pet identity", outcome, stored[0].EXP, stored[0].Pets)
		}
	}
}

func TestParticipationEXPDisconnectDoesNotGrant(t *testing.T) {
	s, c, _, _ := participationVictoryFixture(t)
	before := c.character.Clone()
	s.stageBattleDisconnect(c.battle, c.battle.member(c))
	if err := s.saveBattleDisconnect(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || stored[0].EXP != before.EXP || stored[0].Pets[0].ClientTotalExp() != before.Pets[0].ClientTotalExp() {
		t.Fatal("disconnect granted unearned victory EXP", stored, err)
	}
}
