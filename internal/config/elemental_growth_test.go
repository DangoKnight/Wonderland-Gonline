package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestElementalGrowthIsNotRuntimeConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"elemental_growth":{"fire":{"ATK":{"level":2}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatal("accepted compiled tuning in startup config", err)
	}
	data, err := json.Marshal(Default())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "elemental_growth") {
		t.Fatal("growth exposed in admin startup configuration")
	}
	if _, err := Load("../../config.example.json"); err != nil {
		t.Fatal(err)
	}
}
