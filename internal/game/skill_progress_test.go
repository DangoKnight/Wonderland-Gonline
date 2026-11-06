package game

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/protocol"
)

func TestSkillProgressAliasAndCap(t *testing.T) {
	c := Character{Body: 1, Head: 1}
	id := StarterStunt(c.Body, c.Head)
	if id == 0 {
		t.Fatal("missing stunt")
	}
	c.Skills = []LearnedSkill{{ID: id, Grade: 1, EXP: 99}}
	packets := c.AddSkillEXP(15003, 1)
	if c.Skills[0].Grade != 2 || c.Skills[0].EXP != 0 || len(packets) != 3 || !bytes.Equal(packets[1], protocol.Builder{8, 1, 110, 1}.U32(2).U32(15003)) {
		t.Fatal(c.Skills, packets)
	}
	c.Skills[0].Grade, c.Skills[0].EXP = 9, 899
	c.AddSkillEXP(id, 2)
	if c.Skills[0].Grade != 10 || c.Skills[0].EXP != 1 {
		t.Fatal(c.Skills)
	}
	if len(c.AddSkillEXP(id, 1)) != 0 || c.Skills[0].EXP != 1 {
		t.Fatal("max grade changed")
	}
	if len(c.AddSkillEXP(65535, 1)) != 0 {
		t.Fatal("unknown skill")
	}
}

func TestPetSkillProgressPacketsAndWideEXP(t *testing.T) {
	p := Pet{Skills: []PetSkill{{ID: 11001, Grade: 1, Exp: 99}}}
	packets := p.AddSkillEXP(11001, 1, 3)
	want := protocol.Builder{8, 2, 4}.U16(3).U8(111).U8(1).U32(0).U32(11001)
	if p.Skills[0].Grade != 2 || len(packets) != 2 || !bytes.Equal(packets[1], want) {
		t.Fatal(p.Skills, packets)
	}
	p.Skills[0].Exp = ^uint32(0)
	if len(p.AddSkillEXP(11001, 1, 0)) != 0 || p.Skills[0].Grade != 10 || p.Skills[0].Exp != 0 {
		t.Fatal(p.Skills)
	}
}

func TestSkillProgressUsesProvidedNativeLimit(t *testing.T) {
	const skillID = 50000
	for _, limit := range []byte{5, 15, 30, 255} {
		c := Character{Skills: []LearnedSkill{{ID: skillID, Grade: limit - 1, EXP: uint32(limit-1)*100 - 1}}}
		p := Pet{Skills: []PetSkill{{ID: skillID, Grade: limit - 1, Exp: uint32(limit-1)*100 - 1}}}
		c.AddSkillEXP(skillID, 1, limit)
		p.AddSkillEXP(skillID, 1, 0, limit)
		if c.Skills[0].Grade != limit || p.Skills[0].Grade != limit || p.Skills[0].Exp != 0 {
			t.Fatal("progress stopped at the wrong cap", limit, c.Skills, p.Skills)
		}
		if len(c.AddSkillEXP(skillID, 10000, limit)) != 0 || len(p.AddSkillEXP(skillID, 10000, 1, limit)) != 0 {
			t.Fatal("capped skill gained progress", limit)
		}
		if len(c.SetSkillGrade(skillID, limit, limit)) == 0 {
			t.Fatal("native maximum rejected", limit)
		}
		if limit < 255 && len(c.SetSkillGrade(skillID, limit+1, limit)) != 0 {
			t.Fatal("above-native maximum accepted", limit)
		}
	}
	if SkillGradeLimit() != 10 || SkillGradeLimit(0) != 0 {
		t.Fatal("missing grade metadata must default to ten")
	}
	c := Character{Skills: []LearnedSkill{{ID: skillID, Grade: 1}}}
	p := Pet{Skills: []PetSkill{{ID: skillID, Grade: 1}}}
	if len(c.AddSkillEXP(skillID, 1000, 0)) != 0 || len(p.AddSkillEXP(skillID, 1000, 1, 0)) != 0 || len(c.LearnSkill(50001, 0)) != 0 || len(c.SetSkillGrade(skillID, 1, 0)) != 0 {
		t.Fatal("explicit zero limit allowed learning or progression")
	}
}
