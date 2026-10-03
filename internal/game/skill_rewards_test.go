package game

import (
	"bytes"
	"testing"
)

func TestSkillRewardPacketsAndReplay(t *testing.T) {
	c := Character{Body: 1, Head: 1}
	got := c.LearnSkill(11001)
	want := [][]byte{{8, 1, 110, 1, 1, 0, 0, 0, 0xf9, 0x2a, 0, 0}, {5, 16, 0, 0xf9, 0x2a, 1}, {5, 12, 0xf9, 0x2a, 1}, {5, 11, 0xf9, 0x2a, 0, 0, 0, 0}}
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatal(i, got[i], want[i])
		}
	}
	c.Skills[0].Grade, c.Skills[0].EXP = 7, 99
	if len(c.LearnSkill(11001)) != 0 || len(c.Skills) != 1 || c.Skills[0].EXP != 99 || c.Skills[0].Grade != 7 {
		t.Fatal("replay reset progress")
	}
	if got := c.LearnSkill(StarterStunt(c.Body, c.Head)); !bytes.Equal(got[1], []byte{5, 16, 0, 0x9b, 0x3a, 1}) {
		t.Fatal("stunt alias", got)
	}
}
