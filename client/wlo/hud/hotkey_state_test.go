package hud

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHotbarPreferencesRoundTripAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hotbar.json")
	var b Bindings
	b[1][1] = Binding{Kind: 2, ID: 10001}
	b[3][8] = Binding{Kind: 2, ID: 11001, Target: 3, Pet: 14156}
	if err := b.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadBindings(path)
	if err != nil || loaded != b {
		t.Fatal("round trip", err)
	}
	for _, raw := range []string{`[[{"kind":2,"id":10001}]]`, `[[],[{"kind":7,"id":10001}]]`, `[[],[{}, {"kind":2,"id":11001,"target":5,"pet":14156}]]`} {
		if err = os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if got, err := LoadBindings(path); err == nil || got != (Bindings{}) {
			t.Fatal("bad preferences accepted", raw)
		}
	}
}
