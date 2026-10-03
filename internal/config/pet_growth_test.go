package config

import (
	"os"
	"path/filepath"
	"testing"
	"wonderland-go/internal/game"
)

func TestPetGrowthStartupConfiguration(t *testing.T) {
	for _, tc := range []struct {
		json  string
		want  game.PetGrowthFormula
		valid bool
	}{
		{`{}`, game.PetGrowthBaseStats, true},
		{`{"pet_growth_formula":"base_stats"}`, game.PetGrowthBaseStats, true},
		{`{"pet_growth_formula":"combat_stats"}`, game.PetGrowthCombatStats, true},
		{`{"pet_growth_formula":"unknown"}`, "", false},
		{`{"pet_growth_formula":""}`, "", false},
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(tc.json), 0600); err != nil {
			t.Fatal(err)
		}
		c, err := Load(path)
		if (err == nil) != tc.valid {
			t.Fatal(tc.json, err)
		}
		if tc.valid && c.PetGrowthFormula != tc.want {
			t.Fatal(tc.json, c.PetGrowthFormula)
		}
	}
	if _, err := Load("../../config.example.json"); err != nil {
		t.Fatal(err)
	}
}
