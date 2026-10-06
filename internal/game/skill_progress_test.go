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
