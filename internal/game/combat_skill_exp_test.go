package game

import "testing"

func TestCombatSkillEXP(t *testing.T) {
	for _, tc := range []struct {
		damage, before, max int
		want                uint32
	}{
		{0, 100, 100, 10}, {1, 100, 100, 11}, {1, 1000, 1000, 11}, {31, 40, 100, 41}, {999, 5, 100, 15}, {999, 100, 100, 110},
	} {
		if got := CombatSkillEXP(tc.damage, tc.before, tc.max); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	for _, tc := range []struct {
		restored, before, max int
		want                  uint32
	}{
		{0, 100, 100, 15}, {50, 50, 100, 90}, {1000, 95, 100, 22}, {100, 0, 100, 165},
	} {
		if got := HealingSkillEXP(tc.restored, tc.before, tc.max); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestRebirthLevelDeductionAndLevel199(t *testing.T) {
	for _, tc := range []struct{ before, want byte }{{100, 1}, {120, 21}, {199, 100}} {
		c := Character{Level: tc.before, StatPoints: 77}
		if !c.RebornAs(JobKiller, false) || c.Level != tc.want || c.StatPoints != 77 || c.RebornJob != 1 {
			t.Fatal(tc, c)
		}
		if c.RebornAs(JobKnight, false) {
			t.Fatal("replayed rebirth")
		}
	}
	c := Character{Reborn: true, Job: JobWarrior, Level: 1}
	c.SetLevel(199)
	if c.Level != 199 || c.EXP != 3491147433 {
		t.Fatal(c.Level, c.EXP)
	}
	if got := c.ExpPacket(); len(got) != 12 || got[4] != 0x17 || got[5] != 0xd0 || got[6] != 0xf1 || got[7] != 0x76 {
		t.Fatal(got)
	}
	var level []byte
	for _, p := range c.StatPackets(nil) {
		if p[2] == 35 {
			level = p
		}
	}
	if len(level) != 12 || level[4] != 199 {
		t.Fatal("explicit level missing", level)
	}
}
