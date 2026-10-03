package game

import (
	"bytes"
	"math"
	"testing"
)

func TestSetLevelThresholdsAndRequestedCap(t *testing.T) {
	c := Character{Level: 1}
	c.SetLevel(3)
	// Independent values: level-one threshold 14, level-two threshold 35.
	if c.Level != 3 || c.EXP != 49 || c.StatPoints != 6 {
		t.Fatal(c.Level, c.EXP, c.StatPoints)
	}
	c.SetLevel(1)
	if c.Level != 1 || c.EXP != 0 || c.StatPoints != 6 {
		t.Fatal("lowering a level reclaimed points")
	}
	c.SetLevel(200)
	if c.Level != 199 || c.StatPoints != 603 {
		t.Fatal("legacy requested-200 cap/point behavior changed", c.Level, c.StatPoints)
	}
	c.StatPoints = math.MaxUint16 - 1
	c.Level = 1
	c.SetLevel(2)
	if c.StatPoints != math.MaxUint16 {
		t.Fatal("points overflowed")
	}
	c.SetLevel(0)
	if c.Level != 1 || c.EXP != 0 {
		t.Fatal("zero target was not normalized")
	}
}

func TestExplicitSkillGradePacketsAndReset(t *testing.T) {
	c := Character{Body: 1, Head: 0, Skills: []LearnedSkill{{ID: 11001, Grade: 2, EXP: 89}}}
	got := c.SetSkillGrade(11001, 7)
	want := [][]byte{{8, 1, 110, 1, 7, 0, 0, 0, 249, 42, 0, 0}, {5, 16, 0, 249, 42, 7}, {5, 12, 249, 42, 7}, {5, 11, 249, 42, 0, 0, 0, 0}}
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatal(i, got[i], want[i])
		}
	}
	if c.Skills[0].Grade != 7 || c.Skills[0].EXP != 0 || len(c.Skills) != 1 {
		t.Fatal("grade reset duplicated a skill")
	}
	for _, grade := range []byte{7, 1} {
		c.Skills[0].EXP = 90
		got = c.SetSkillGrade(11001, grade)
		if len(got) != 3 || got[0][0] != 5 || c.Skills[0].EXP != 0 {
			t.Fatal("equal/lower grade notification mismatch", got)
		}
	}
	for _, grade := range []byte{0, 11, 255} {
		before := c.Clone()
		if len(c.SetSkillGrade(11001, grade)) != 0 || c.Skills[0] != before.Skills[0] {
			t.Fatal("invalid grade mutated a skill")
		}
	}
	alias := c.SetSkillGrade(11075, 3)
	if !bytes.Equal(alias[1], []byte{5, 16, 0, 155, 58, 3}) {
		t.Fatal("native stunt alias missing", alias)
	}
}
