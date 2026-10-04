package game

import (
	"bytes"
	"testing"
)

func TestCharacterMetadataNativePackets(t *testing.T) {
	c := Character{ID: 10001, Slot: 1, Name: "C", Nickname: "N", Job: JobKnight, Reborn: true, Potential: 300, Level: 1, HP: 1, MaxHP: 1}
	base, err := c.BaseStatsPacket(func(uint16) (uint16, bool) { return 0, true })
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(base[len(base)-8:], []byte{0, 0, 0, 0, 0, 1, 44, 3}) {
		t.Fatal(base[len(base)-8:])
	}
	peer, err := c.AppearancePacket(true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(peer, []byte{0, 1, 3, 1, 'C', 1, 'N', 255}) {
		t.Fatal(peer)
	}
	found := false
	for _, packet := range c.StatPackets(nil) {
		if bytes.Equal(packet, []byte{8, 1, 37, 1, 44, 1, 0, 0, 0, 0, 0, 0}) {
			found = true
		}
	}
	if !found {
		t.Fatal("wide potential missing from stat 37")
	}
	roster, err := c.SelectionRecord()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(roster[len(roster)-14:len(roster)-12], []byte{1, 3}) {
		t.Fatal(roster)
	}
	p := Pet{ID: 1, Level: 1, Potential: 77}
	found = false
	for _, packet := range p.ProgressionPackets(1, nil) {
		if bytes.Equal(packet, []byte{8, 2, 4, 1, 0, 37, 1, 77, 0, 0, 0, 0, 0, 0, 0}) {
			found = true
		}
	}
	if !found {
		t.Fatal("pet potential stat missing")
	}
}
func TestRebornClassInnateStatModifiers(t *testing.T) {
	c := Character{Element: Fire, Level: 100, Base: Attributes{10, 20, 30, 40, 50}, Reborn: true}
	for _, tt := range []struct {
		job  byte
		want [5]int32
	}{{JobKiller, [5]int32{242, 235, 220, 288, 275}}, {JobWarrior, [5]int32{242, 258, 220, 288, 250}}, {JobKnight, [5]int32{220, 258, 220, 288, 275}}, {JobWit, [5]int32{220, 235, 242, 288, 250}}, {JobPriest, [5]int32{220, 235, 220, 317, 250}}, {JobSeer, [5]int32{220, 235, 220, 288, 275}}} {
		c.Job = tt.job
		stats := c.Combat(nil)
		got := [5]int32{stats.ATK, stats.DEF, stats.MAT, stats.MDF, stats.SPD}
		if got != tt.want {
			t.Fatal(tt.job, got, tt.want)
		}
	}
	c.Reborn = false
	c.Job = JobKiller
	if c.Combat(nil).ATK != 220 {
		t.Fatal("metadata alone applied class boost")
	}
}
func TestRebornProgressionStartsAtOne(t *testing.T) {
	c := Character{Level: 1, Reborn: true}
	if c.LevelRequirement(1) != 59 || c.LevelFromEXP(0) != 1 {
		t.Fatal("invalid normalized reborn starting level")
	}
	c.AddExp(58)
	if c.Level != 1 {
		t.Fatal(c.Level)
	}
	c.AddExp(1)
	if c.Level != 2 || c.StatPoints != 3 {
		t.Fatal(c)
	}
	c.SetLevel(100)
	if c.Level != 100 {
		t.Fatal(c.Level)
	}
}
