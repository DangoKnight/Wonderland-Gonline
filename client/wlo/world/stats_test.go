package world

import (
	"encoding/binary"
	"testing"

	"wonderland-go/client/wlo/login"
)

// TestBaseStats reads 5/3 with one skill (FUN_004381c4's offsets) and the
// tail's rebirth and job.
func TestBaseStats(t *testing.T) {
	p := []byte{3, 2} // subcommand, element
	p = binary.LittleEndian.AppendUint32(p, 150)
	p = binary.LittleEndian.AppendUint16(p, 40)
	for _, a := range []uint16{11, 12, 13, 14, 15} { // STR, CON, INT, WIS, AGI
		p = binary.LittleEndian.AppendUint16(p, a)
	}
	p = append(p, 9)
	p = binary.LittleEndian.AppendUint32(p, 1234)
	p = binary.LittleEndian.AppendUint16(p, 0)
	p = binary.LittleEndian.AppendUint16(p, 0)
	p = binary.LittleEndian.AppendUint32(p, 300) // stored max HP
	p = binary.LittleEndian.AppendUint16(p, 90)  // stored max SP
	p = append(p, make([]byte, baseSkillsLen-len(p))...)
	p = binary.LittleEndian.AppendUint16(p, 1)
	p = append(p, make([]byte, skillBytes)...)
	p = binary.LittleEndian.AppendUint16(p, 5)
	p = binary.LittleEndian.AppendUint16(p, 6)
	p = append(p, 1, 4, 0, 0)
	var s Stats
	if err := s.ParseBaseStats(p); err != nil {
		t.Fatal(err)
	}
	if s.Element != 2 || s.HP != 150 || s.SP != 40 || s.STR != 11 || s.CON != 12 || s.INT != 13 || s.WIS != 14 || s.AGI != 15 ||
		s.Level != 9 || s.EXP != 1234 || s.MaxHP != 300 || s.MaxSP != 90 || s.HPExtra != 5 || s.SPExtra != 6 || s.Rebirth != 1 || s.Job != 4 {
		t.Fatalf("%+v", s)
	}
	if s.EffectiveLevel() != 109 {
		t.Fatalf("effective level %d", s.EffectiveLevel())
	}
	if (&Stats{}).ParseBaseStats(p[:20]) == nil {
		t.Fatal("short packet accepted")
	}
}

// TestBonusRecomputesMaxima: 8/1's bonus stats recompute the maxima with
// Formula.Dat's constants (here the server's: 1, 2, 0.35, 2, 180).
func TestBonusRecomputesMaxima(t *testing.T) {
	f := &login.Formula{}
	f.HP.PerLevel, f.HP.PerAttrLevel, f.HP.Power, f.HP.PerAttr, f.HP.Base = 1, 2, 0.35, 2, 180
	s := Stats{Formula: f, Level: 1}
	s.Apply(StatCON, 5)
	s.Apply(StatHPBonus, 20)
	if s.MaxHP != 1*5*2+1+5*2+180+20 {
		t.Fatalf("max HP %d", s.MaxHP)
	}
	s.Apply(StatHP, 77)
	if s.HP != 77 {
		t.Fatal("HP update")
	}
}
