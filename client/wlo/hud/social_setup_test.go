package hud

import (
	"testing"
	"time"
)

// TestSunSign: FUN_00279c88's month table, its cut days and the empty
// date.
func TestSunSign(t *testing.T) {
	for _, c := range []struct {
		m, d int
		want byte
	}{{3, 20, 12}, {3, 21, 1}, {6, 21, 3}, {6, 22, 4}, {2, 19, 11}, {2, 20, 12}, {12, 22, 9}, {12, 23, 10}, {1, 21, 11}} {
		if got := SunSign(2000, c.m, c.d); got != c.want {
			t.Errorf("%d/%d: sign %d, want %d", c.m, c.d, got, c.want)
		}
	}
	if SunSign(2000, 0, 5) != 0 || SunSign(0, 3, 5) != 0 {
		t.Error("an incomplete date has a sign")
	}
}

// TestClampDate: FUN_0027977c's limits.
func TestClampDate(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct{ y, m, d, wy, wm, wd int }{
		{3000, 1, 1, 2026, 1, 1}, {1500, 1, 1, 1900, 1, 1}, {999, 1, 1, 999, 1, 1},
		{2000, 13, 40, 2000, 12, 31}, {2001, 4, 31, 2001, 4, 30}, {2000, 2, 30, 2000, 2, 29},
		{2001, 2, 30, 2001, 2, 28}, {2026, 12, 25, 2026, 10, 12},
	} {
		y, m, d := ClampDate(c.y, c.m, c.d, now)
		if y != c.wy || m != c.wm || d != c.wd {
			t.Errorf("%d-%d-%d → %d-%d-%d, want %d-%d-%d", c.y, c.m, c.d, y, m, d, c.wy, c.wm, c.wd)
		}
	}
}
