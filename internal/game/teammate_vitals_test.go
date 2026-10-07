package game

import "testing"

func TestNativeTeammateVitalBonusesRebirth(t *testing.T) {
	c := Character{Level: 1, Element: Water, Reborn: true}
	hp, sp := c.NativeVitalBonuses(nil)
	// With no CON/WIS or equipment, native rebirth adds 100 level points
	// to both maxima. The projection removes those points to match Combat.
	if hp != -100 || sp != -100 {
		t.Fatal(hp, sp)
	}
	c.Reborn = false
	hp, sp = c.NativeVitalBonuses(nil)
	if hp != 0 || sp != 0 {
		t.Fatal(hp, sp)
	}
}
