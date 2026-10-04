package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComboDamageConfiguration(t *testing.T) {
	for _, tc := range []struct {
		raw           string
		want, invalid bool
	}{
		{`{}`, false, false}, {`{"combo_damage_per_participant":false}`, false, false},
		{`{"combo_damage_per_participant":true}`, true, false},
		{`{"combo_damage_per_participant":"true"}`, false, true},
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(tc.raw), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if (err != nil) != tc.invalid || (!tc.invalid && got.ComboDamagePerParticipant != tc.want) {
			t.Fatal(tc, got, err)
		}
	}
}
