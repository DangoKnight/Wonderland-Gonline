package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBattleGoldConfiguration(t *testing.T) {
	for _, tc := range []struct {
		raw              string
		enabled, invalid bool
	}{
		{`{}`, true, false},
		{`{"battle_gold_enabled":true}`, true, false},
		{`{"battle_gold_enabled":false}`, false, false},
		{`{"battle_gold_enabled":"false"}`, false, true},
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(tc.raw), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if (err != nil) != tc.invalid || err == nil && got.BattleGoldEnabled != tc.enabled {
			t.Fatal(tc, got, err)
		}
	}
}
