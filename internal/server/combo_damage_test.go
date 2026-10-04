package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"wonderland-go/internal/config"
)

func TestComboDamageStartupSelectionAndRestart(t *testing.T) {
	s, _, _ := worldFixture(t)
	for _, enabled := range []bool{false, true} {
		cfg := config.Default()
		cfg.ComboDamagePerParticipant = enabled
		selected := New(cfg, s.Store, s.Assets, s.Log)
		if selected.rules().ComboDamagePerParticipant != enabled {
			t.Fatal("startup option did not reach battle rules")
		}
		path := filepath.Join(t.TempDir(), "config.json")
		raw, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		selected.SetConfigurationPath(path)
		snap, err := selected.StartupConfiguration()
		if err != nil {
			t.Fatal(err)
		}
		next := snap.Configuration
		next.ComboDamagePerParticipant = !enabled
		if err = selected.SaveStartupConfiguration(snap.Version, next); err != nil {
			t.Fatal(err)
		}
		if selected.rules().ComboDamagePerParticipant != enabled || selected.Config.ComboDamagePerParticipant != enabled {
			t.Fatal("configuration file edit changed running combo rules")
		}
		loaded, err := config.Load(path)
		if err != nil || loaded.ComboDamagePerParticipant == enabled {
			t.Fatal("new setting not saved", err)
		}
		restarted := New(loaded, s.Store, s.Assets, s.Log)
		if restarted.rules().ComboDamagePerParticipant == enabled {
			t.Fatal("restart did not apply setting")
		}
		if err = selected.LoadRuntimeSettings(context.Background()); err != nil {
			t.Fatal(err)
		}
		if selected.rules().ComboDamagePerParticipant != enabled {
			t.Fatal("runtime settings changed combo damage")
		}
	}
}
