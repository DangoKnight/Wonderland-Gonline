package game

import (
	"bytes"
	"testing"
	"wonderland-go/internal/protocol"
)

func knows(c Character, id uint16) bool {
	for _, s := range c.Skills {
		if s.ID == id {
			return true
		}
	}
	return false
}

func TestSkillTreeRequirementsAndReplay(t *testing.T) {
	for _, tc := range []struct {
		element byte
		attrs   Attributes
		id      uint16
	}{
		{Fire, Attributes{Strength: 65, Wisdom: 2}, 15102},
		{Earth, Attributes{Strength: 60, Constitution: 5}, 15049},
		{2, Attributes{Intelligence: 1, Wisdom: 30}, 11051},
		{Wind, Attributes{Strength: 5, Agility: 10}, 15079},
		{5, Attributes{Wisdom: 38}, 25470},
		{7, Attributes{Intelligence: 38}, 25275},
	} {
		c := Character{Element: tc.element, Base: tc.attrs}
		c.UnlockQualifiedSkills(true, func(id uint16) bool { return id == tc.id })
		if !knows(c, tc.id) {
			t.Fatal("qualified skill missing", tc)
		}
		c.Skills[0].Grade, c.Skills[0].EXP = 7, 123
		c.UnlockQualifiedSkills(true, nil)
		if c.Skills[0].Grade != 7 || c.Skills[0].EXP != 123 {
			t.Fatal("progress overwritten")
		}
		if len(c.UnlockQualifiedSkills(true, nil)) != 0 {
			t.Fatal("replayed unlock")
		}
	}
	c := Character{Element: Fire, Base: Attributes{Strength: 65, Wisdom: 1}}
	if len(c.UnlockQualifiedSkills(true, func(id uint16) bool { return id == 15102 })) != 0 {
		t.Fatal("secondary requirement ignored")
	}
	c.Element = 2
	c.Base.Wisdom = 2
	if len(c.UnlockQualifiedSkills(true, func(id uint16) bool { return id == 15102 })) != 0 {
		t.Fatal("element requirement ignored")
	}
}

func TestSkillEvolutionIsOneStepAndPreservesExisting(t *testing.T) {
	c := Character{Skills: []LearnedSkill{{ID: 11166, Grade: 10, EXP: 9}}}
	if len(c.UnlockQualifiedSkills(false, nil)) != 0 {
		t.Fatal("login evolved skill")
	}
	packets := c.UnlockQualifiedSkills(true, nil)
	if !knows(c, 15104) || knows(c, 15105) || len(packets) != 4 || !bytes.Equal(packets[0], protocol.Builder{8, 1, 110, 1}.U32(1).U32(15104)) || !bytes.Equal(packets[3], []byte{5, 4}) {
		t.Fatal(c.Skills, packets)
	}
	c.Skills[1].Grade = 10
	c.UnlockQualifiedSkills(true, nil)
	if !knows(c, 15105) || c.Skills[0].EXP != 9 {
		t.Fatal(c.Skills)
	}
}

func TestSkillTreeAvatarBonusAndMissingCatalog(t *testing.T) {
	c := Character{Element: Fire, Body: 1, Head: 1}
	bonus := c.Attributes().Strength
	c.Base.Strength = 16 - bonus
	if len(c.UnlockQualifiedSkills(true, func(id uint16) bool { return id == 15101 })) != 4 {
		t.Fatal("avatar bonus not used")
	}
	c.Skills = nil
	if len(c.UnlockQualifiedSkills(true, func(uint16) bool { return false })) != 0 || len(c.Skills) != 0 {
		t.Fatal("missing skill catalog entry learned")
	}
}
