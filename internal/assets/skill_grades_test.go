package assets

import (
	"encoding/json"
	"testing"
)

func TestSkillMaximumGradeNativeAndFallback(t *testing.T) {
	// Independent encoded Skill.dat golden: ID 50000; maximum grade 15.
	record := make([]byte, 148)
	record[20] = 'X'
	record[22], record[23] = 0xf4, 0xad
	record[105] = 0xee
	skills, err := ParseSkills(record)
	if err != nil || skills[50000].MaximumGrade == nil || skills[50000].GradeLimit() != 15 {
		t.Fatal("native grade byte not decoded", skills, err)
	}
	if (Skill{}).GradeLimit() != 10 {
		t.Fatal("absent maximum must default to ten")
	}
	for _, tc := range []struct {
		raw  string
		want byte
	}{{`{}`, 10}, {`{"maximum_grade":null}`, 10}, {`{"maximum_grade":0}`, 0}, {`{"maximum_grade":15}`, 15}} {
		var skill Skill
		if err := json.Unmarshal([]byte(tc.raw), &skill); err != nil || skill.GradeLimit() != tc.want {
			t.Fatal("absent/zero grade distinction was lost", tc, skill, err)
		}
	}
	record[105] = 0xf9 // Independent encoded zero.
	zero, err := ParseSkills(record)
	if err != nil || zero[50000].MaximumGrade == nil || zero[50000].GradeLimit() != 0 {
		t.Fatal("native zero maximum was lost", zero, err)
	}
	for _, value := range []struct{ stored, want byte }{{0, 0}, {5, 5}, {15, 15}, {30, 30}, {255, 255}} {
		if got := (Skill{MaximumGrade: GradeLimit(value.stored)}).GradeLimit(); got != value.want {
			t.Fatal(value, got)
		}
	}
	shape := SkillTargeting{MinGrade: 11, MaxGrade: 15, All: true}
	skill := Skill{MaximumGrade: GradeLimit(15), Targeting: []SkillTargeting{shape}}
	if err := ValidateSkillTargeting(skill.Targeting, skill.GradeLimit()); err != nil {
		t.Fatal(err)
	}
	if got, ok := skill.TargetingAt(100); !ok || got.MaxGrade != 15 {
		t.Fatal("high-grade targeting was clamped to ten", got, ok)
	}
}
