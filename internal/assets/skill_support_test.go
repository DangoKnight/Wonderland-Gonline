package assets

import "testing"

func TestNativeSupportClassificationAndGradeFallback(t *testing.T) {
	if !(Skill{EffectLayer: 6, Name: "translated"}).RestoresSP() || !(Skill{Name: "Magic Recovery"}).RestoresSP() || !(Skill{EffectLayer: 5}).CleansesStatus() {
		t.Fatal("native support classification")
	}
	if (Skill{Name: "Cure Spell"}).CleansesStatus() || (Skill{Name: "Fire Ball"}).RestoresSP() {
		t.Fatal("support misclassification")
	}
	s := Skill{NativePattern109: 2, NativePattern111: 7}
	for _, tc := range []struct {
		grade int
		want  byte
	}{{1, 2}, {3, 2}, {4, 2}, {6, 2}, {7, 7}, {9, 7}, {10, 7}} {
		if s.NativePatternAt(tc.grade) != tc.want || !s.IsAreaAttack(tc.grade) {
			t.Fatal("native grade fallback", tc)
		}
	}
}
