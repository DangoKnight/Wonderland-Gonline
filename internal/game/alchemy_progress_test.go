package game

import (
	"bytes"
	"testing"
)

func TestAlchemyNativeExperienceCurveAndWire(t *testing.T) {
	// Formula.dat: round((grade+1)^3.1)+5. Cumulative bases: 0,14,49.
	if AlchemyGradeEXP(1) != 14 || AlchemyGradeEXP(2) != 35 || AlchemyCumulativeEXP(3, 0) != 49 {
		t.Fatal("native skill EXP curve")
	}
	c := Character{Skills: []LearnedSkill{{ID: AlchemyJuniorSkill, Grade: 1, EXP: 12}}}
	p := c.AddSkillEXP(AlchemyJuniorSkill, 1)
	if len(p) != 1 || !bytes.Equal(p[0], []byte{8, 1, 111, 1, 13, 0, 0, 0, 126, 62, 0, 0}) {
		t.Fatal("native EXP update", p)
	}
	p = c.AddSkillEXP(AlchemyJuniorSkill, 1)
	if c.Skills[0].Grade != 2 || c.Skills[0].EXP != 0 || len(p) != 2 || !bytes.Equal(p[1], []byte{8, 1, 111, 1, 14, 0, 0, 0, 126, 62, 0, 0}) {
		t.Fatal("level-up/cumulative EXP", c.Skills, p)
	}
	c.Skills[0] = LearnedSkill{ID: AlchemyJuniorSkill, Grade: 1, EXP: 22}
	c.AddSkillEXP(AlchemyJuniorSkill, 1)
	if c.Skills[0].Grade != 2 || c.Skills[0].EXP != 9 {
		t.Fatal("old overfilled progress lost", c.Skills)
	}
}
