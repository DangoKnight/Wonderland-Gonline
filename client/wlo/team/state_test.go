package team

import (
	"reflect"
	"testing"
	"wonderland-gonline/client/wlo/world"
)

func TestTeamRosterValidationAndOtherTeams(t *testing.T) {
	s := State{}
	s.Reset(1)
	if !s.ApplyRoster([]byte{13, 6, 1, 0, 0, 0, 1, 2, 0, 0, 0}) || !s.InParty() || s.Leader != 1 || !reflect.DeepEqual(s.Others(), []uint32{2}) {
		t.Fatal(s)
	}
	for _, p := range [][]byte{{13, 6, 1, 0, 0, 0, 1, 1, 0, 0, 0}, {13, 6, 1, 0, 0, 0, 3}, {13, 6, 0, 0, 0, 0, 0}} {
		if s.ApplyRoster(p) || s.Leader != 1 || len(s.Members) != 2 {
			t.Fatal("invalid roster adopted")
		}
	}
	if !s.ApplyRoster([]byte{13, 6, 3, 0, 0, 0, 1, 4, 0, 0, 0}) || s.Leader != 1 {
		t.Fatal("adopted unrelated team")
	}
	if !s.ApplyRoster([]byte{13, 6, 1, 0, 0, 0, 0}) || s.InParty() {
		t.Fatal("did not leave")
	}
}
func TestTeamStatsKeepLevelAndHPSeparate(t *testing.T) {
	s := State{}
	s.Reset(1)
	// Captured native layout: stat byte 0x23 is level; byte 0x19 is HP.
	level := []byte{8, 3, 2, 0, 0, 0, 35, 1, 1, 0, 0, 0, 0, 0, 0, 0}
	hp := []byte{8, 3, 2, 0, 0, 0, 25, 1, 181, 0, 0, 0, 0, 0, 0, 0}
	if !s.ApplyStat(level) || !s.ApplyStat(hp) {
		t.Fatal("native stats rejected")
	}
	if m := s.Vitals[2]; m.Stats.Level != 1 || m.Stats.HP != 181 {
		t.Fatal(m)
	}
	con := []byte{8, 3, 2, 0, 0, 0, 29, 1, 7, 0, 0, 0, 0, 0, 0, 0}
	if !s.ApplyStat(con) || s.Vitals[2].Stats.CON != 7 || s.Vitals[2].Stats.Level != 1 {
		t.Fatal("CON overwritten level")
	}
	neg := []byte{8, 3, 2, 0, 0, 0, 207, 2, 5, 0, 0, 0, 0, 0, 0, 0}
	if !s.ApplyStat(neg) || s.Vitals[2].Stats.HPBonus != -5 {
		t.Fatal("negative bonus")
	}
	bad := append([]byte(nil), hp...)
	bad[7] = 3
	before := s.Vitals[2].Stats
	if s.ApplyStat(bad) || s.Vitals[2].Stats != before {
		t.Fatal("malformed stats changed state")
	}
	s.Vitals[2].Stats.Apply(world.StatHP, 180)
}
