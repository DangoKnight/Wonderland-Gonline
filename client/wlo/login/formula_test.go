package login

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFormulaMaximums: Formula.Dat's HP and SP constants give the same
// maxima as the server's formulas (internal/game: level^0.35 × CON × 2 +
// level + CON × 2 + 180, and level^0.3 × WIS × 3.2 + level + WIS × 2 + 94).
func TestFormulaMaximums(t *testing.T) {
	a := NewAssets(filepath.Join("..", "..", "..", "data"))
	if _, err := os.Stat(a.DataPath(formulaExport)); err != nil {
		t.Skip("formula export not present")
	}
	f, err := LoadFormula(a)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("HP %+v SP %+v", f.HP, f.SP)
	for _, tc := range []struct {
		level, attr, hp, sp int
	}{{1, 0, 181, 95}, {1, 5, 201, 121}, {37, 22, 417, 383}} {
		if got := f.HP.Max(tc.level, uint16(tc.attr), 0); got != tc.hp {
			t.Errorf("HP level %d attr %d: %d, want %d", tc.level, tc.attr, got, tc.hp)
		}
		if got := f.SP.Max(tc.level, uint16(tc.attr), 0); got != tc.sp {
			t.Errorf("SP level %d attr %d: %d, want %d", tc.level, tc.attr, got, tc.sp)
		}
	}
}
