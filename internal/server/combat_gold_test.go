package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/battle"
)

func TestBattleGoldSwitchPersistsVictoryBalance(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		s, c, _ := battleFixture(t, 1000)
		s.battleGoldEnabled = enabled
		// Mutating the exported config after startup must not change policy.
		s.Config.BattleGoldEnabled = !enabled
		run := c.battle
		run.timer.Stop()
		goldBefore, expBefore := c.character.Gold, c.character.EXP
		exp, gold := run.b.Rewards()
		if gold == 0 || exp == 0 {
			t.Fatal("fixture has no rewards")
		}
		s.endBattle(run, battle.Victory)
		settle(t, s, c)
		want := goldBefore
		if enabled {
			want += uint32(gold)
		}
		stored, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil || stored[0].Gold != want || c.character.Gold != want {
			t.Fatal("battle gold policy", enabled, want, stored, err)
		}
		if stored[0].EXP <= expBefore {
			t.Fatal("gold switch disabled EXP")
		}
	}
}
