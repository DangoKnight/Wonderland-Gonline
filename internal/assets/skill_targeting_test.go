package assets

import "testing"

func TestSkillTargetingGradeValidation(t *testing.T) {
	shapes := []SkillTargeting{{MinGrade: 1, MaxGrade: 5, Offsets: []FormationOffset{{}}}, {MinGrade: 6, MaxGrade: 10, All: true}}
	if err := ValidateSkillTargeting(shapes); err != nil {
		t.Fatal(err)
	}
	s := Skill{Targeting: shapes}
	if s.IsAreaAttack(5) || !s.IsAreaAttack(6) || !s.IsAreaAttack(100) {
		t.Fatal("grade boundary")
	}
	for _, invalid := range [][]SkillTargeting{
		{{MinGrade: 0, MaxGrade: 10, All: true}}, {{MinGrade: 1, MaxGrade: 11, All: true}},
		{{MinGrade: 1, MaxGrade: 5, All: true}, {MinGrade: 5, MaxGrade: 10, All: true}},
		{{MinGrade: 1, MaxGrade: 10}}, {{MinGrade: 1, MaxGrade: 10, All: true, Offsets: []FormationOffset{{}}}},
		{{MinGrade: 1, MaxGrade: 10, Offsets: []FormationOffset{{X: 1}}}},
		{{MinGrade: 1, MaxGrade: 10, Offsets: []FormationOffset{{}, {}}}},
		{{MinGrade: 1, MaxGrade: 10, Offsets: []FormationOffset{{}, {X: 4}}}},
	} {
		if ValidateSkillTargeting(invalid) == nil {
			t.Fatal("accepted", invalid)
		}
	}
}
