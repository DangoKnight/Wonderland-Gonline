package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
)

func TestPetGrowthStartupFormulaReachesEXPGrants(t *testing.T) {
	s, _, _ := worldFixture(t)
	s.Assets.Items[42] = game.ItemDefinition{ID: 42, Status: [2]uint16{214, 0}, Values: [2]int32{140, 0}}
	for _, tc := range []struct {
		formula game.PetGrowthFormula
		total   int
	}{{game.PetGrowthBaseStats, 25}, {game.PetGrowthCombatStats, 120}} {
		cfg := config.Default()
		cfg.PetGrowthFormula = tc.formula
		selected := New(cfg, s.Store, s.Assets, s.Log)
		p := game.Pet{ID: 17100, Level: 1, Base: game.Attributes{Strength: 5, Constitution: 5, Intelligence: 5, Wisdom: 5, Agility: 5}}
		p.Equipment[0] = game.Item{ID: 42, Count: 1}
		selected.worldMu.Lock()
		levels := selected.gainPetExp(&p, 14, func(total int) int {
			if total != tc.total {
				t.Errorf("%s weights=%d, want %d", tc.formula, total, tc.total)
			}
			return total - 1
		})
		selected.worldMu.Unlock()
		if levels != 1 || p.Base.Agility != 6 {
			t.Fatal(p, levels)
		}
	}
}

func TestPetGrowthConfigurationSaveRequiresRestart(t *testing.T) {
	s, _, _ := worldFixture(t)
	cfg := s.Config
	path := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s.SetConfigurationPath(path)
	snapshot, err := s.StartupConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	next := snapshot.Configuration
	next.PetGrowthFormula = game.PetGrowthCombatStats
	if err = s.SaveStartupConfiguration(snapshot.Version, next); err != nil {
		t.Fatal(err)
	}
	if s.Config.PetGrowthFormula != game.PetGrowthBaseStats || s.petGrowthFormula != game.PetGrowthBaseStats {
		t.Fatal("configuration save changed running formula")
	}
	loaded, err := config.Load(path)
	if err != nil || loaded.PetGrowthFormula != game.PetGrowthCombatStats {
		t.Fatal("formula not saved", err)
	}
	settings := s.RuntimeSettings()
	settings.ExpRate = 2
	if err = s.UpdateRuntimeSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	if err = s.LoadRuntimeSettings(context.Background()); err != nil || s.Config.PetGrowthFormula != game.PetGrowthBaseStats {
		t.Fatal("live settings changed formula", err)
	}
	raw, err := json.Marshal(s.RuntimeSettings())
	if err != nil || strings.Contains(string(raw), "pet_growth_formula") {
		t.Fatal("formula exposed as live setting", err)
	}
	if err = s.Store.SaveSettings(context.Background(), map[string]string{"pet_growth_formula": "combat_stats"}); err == nil {
		t.Fatal("formula accepted as live database setting")
	}
}
