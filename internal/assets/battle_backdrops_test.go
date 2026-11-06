package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBattleBackdropPositionAndOrder(t *testing.T) {
	b := BattleBackdrop{Default: 80, Regions: []BattleBackdropRegion{{50, 10, 20, 40, 60}, {90, 30, 40, 70, 80}}}
	for _, tc := range []struct {
		x, y uint16
		want byte
	}{{0, 0, 80}, {10, 20, 50}, {40, 60, 50}, {41, 60, 90}, {70, 80, 90}, {71, 80, 80}} {
		if got := b.At(tc.x, tc.y); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestInstalledNativeBattleBackdrops(t *testing.T) {
	root := os.Getenv("WONDERLAND_TEST_DATA")
	if root == "" {
		t.Skip("native fixture unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(root, "Ground.MMG"))
	if err != nil {
		t.Fatal(err)
	}
	values, err := ParseBattleBackdrops(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1147 {
		t.Fatalf("native maps=%d", len(values))
	}
	regions := 0
	for _, b := range values {
		regions += len(b.Regions)
	}
	if regions != 1398 {
		t.Fatalf("native regions=%d", regions)
	}
	for _, tc := range []struct {
		mapID, x, y uint16
		want        byte
	}{
		{60001, 2202, 1395, 91}, {60001, 2622, 755, 81},
		{60001, 2000, 400, 90}, {60001, 5000, 3000, 50},
		{60000, 1694, 963, 50}, {12000, 1800, 700, 130},
		{12002, 500, 455, 140}, {60001, 300, 300, 80},
	} {
		if got := values[tc.mapID].At(tc.x, tc.y); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}
