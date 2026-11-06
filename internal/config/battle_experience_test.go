package config

import (
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestBattleExperienceConfiguration(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want game.BattleEXPFormula
		bad  bool
	}{
		{`{}`, game.BattleEXPSimple, false},
		{`{"battle_exp_formula":"simple"}`, game.BattleEXPSimple, false},
		{`{"battle_exp_formula":"participation"}`, game.BattleEXPParticipation, false},
		{`{"battle_exp_formula":"unknown"}`, "", true},
		{`{"battle_exp_formula":true}`, "", true},
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(tc.raw), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if (err != nil) != tc.bad || err == nil && got.BattleEXPFormula != tc.want {
			t.Fatal(tc, got, err)
		}
	}
}
