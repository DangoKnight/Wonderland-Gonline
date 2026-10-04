package world

import (
	"encoding/binary"
	"errors"

	"wonderland-go/client/wlo/login"
)

// Stats are the player's values on its role object, as the status panel
// reads them. 5/3 (FUN_004381c4) fills them; 8/1 (FUN_00416ebc) updates
// one at a time; 26/4 sets the gold (+0x3b44).
type Stats struct {
	Element            byte   // +0x1f82
	HP                 uint32 // +0x1f84
	SP                 uint16 // +0x1f88
	INT, STR, CON, AGI uint16 // +0x1f8a, +0x1f8c, +0x1f8e, +0x1f90
	WIS                uint16 // +0x1f92
	Level              byte   // +0x1f98
	Rebirth            byte   // +0x1f99
	Job                byte   // +0x1f9c
	EXP                uint32 // +0x1fa0
	MaxHP              uint32 // +0x1fa8
	MaxSP              uint16 // +0x1fac
	HPBonus, SPBonus   int32  // +0x1fd8, +0x1fdc (stats 0xcf, 0xd0)
	HPExtra, SPExtra   uint16 // +0x1f94, +0x1f96 (5/3's tail); max HP also adds +0x1ff4, not ported
	Gold               uint32 // +0x3b44
	Potential          byte
	Combat             [5]uint16 // +0x1ffc..0x2004: inventory display values
	Points             uint16    // +0x1fa6
	Formula            *login.Formula
}

// Stat IDs of 8/1 (FUN_00416ebc) and of the role accessor FUN_004166e4.
const (
	StatElement   = 0x18
	StatHP        = 0x19
	StatSP        = 0x1a
	StatINT       = 0x1b
	StatSTR       = 0x1c
	StatCON       = 0x1d
	StatAGI       = 0x1e
	StatJob       = 0x1f
	StatWIS       = 0x21
	StatLevel     = 0x23
	StatEXP       = 0x24
	StatPotential = 0x25
	StatATK       = 0x29
	StatDEF       = 0x2a
	StatMAT       = 0x2b
	StatMDF       = 0x2c
	StatSPD       = 0x2d
	StatPoints    = 0x26
	StatHPBonus   = 0xcf
	StatSPBonus   = 0xd0
)

// rebirthLevels is FUN_00485490: a rebirth counts as 100 levels.
const rebirthLevels = 100

// 5/3's layout after its command byte (FUN_004381c4, 1-based copies
// converted to offsets): element, HP, SP, five attributes, level, EXP,
// two words, the stored maximum HP and SP, …, the skill list, then two
// words, rebirth and job.
//
// The attributes are read as STR, CON, INT, WIS, AGI (+0x1f8c, +0x1f8e,
// +0x1f8a, +0x1f92, +0x1f90). This repository's server writes them as CON,
// INT, STR, AGI, WIS; the 8/1 updates that follow set each by ID.
const (
	baseElement   = 1
	baseHP        = 2
	baseSP        = 6
	baseSTR       = 8
	baseCON       = 10
	baseINT       = 12
	baseWIS       = 14
	baseAGI       = 16
	baseLevel     = 0x12
	baseEXP       = 0x13
	baseMaxHP     = 0x1b
	baseMaxSP     = 0x1f
	baseSkillsLen = 0x3d
	skillBytes    = 7
)

var errBaseStats = errors.New("5/3: packet too short")

// ParseBaseStats reads 5/3 after its command byte.
func (s *Stats) ParseBaseStats(p []byte) error {
	if len(p) < baseSkillsLen+2 {
		return errBaseStats
	}
	u16 := func(o int) uint16 { return binary.LittleEndian.Uint16(p[o:]) }
	u32 := func(o int) uint32 { return binary.LittleEndian.Uint32(p[o:]) }
	s.Element = p[baseElement]
	s.HP = u32(baseHP)
	s.SP = u16(baseSP)
	s.STR, s.CON, s.INT, s.WIS, s.AGI = u16(baseSTR), u16(baseCON), u16(baseINT), u16(baseWIS), u16(baseAGI)
	s.Level = p[baseLevel]
	s.EXP = u32(baseEXP)
	s.MaxHP = u32(baseMaxHP)
	s.MaxSP = u16(baseMaxSP)
	tail := baseSkillsLen + 2 + int(u16(baseSkillsLen))*skillBytes
	if len(p) >= tail+6 {
		s.HPExtra, s.SPExtra = u16(tail), u16(tail+2)
		s.Rebirth, s.Job = p[tail+4], p[tail+5]
	}
	return nil
}

// EffectiveLevel is stat 0x28: level plus 100 per rebirth.
func (s *Stats) EffectiveLevel() int { return int(s.Level) + int(s.Rebirth)*rebirthLevels }

// Apply is 8/1 for the player: the stat takes the value, and the bonus
// stats recompute the maxima with the formula.
func (s *Stats) Apply(id byte, v uint32) {
	switch id {
	case StatElement:
		s.Element = byte(v)
	case StatHP:
		s.HP = v
	case StatSP:
		s.SP = uint16(v)
	case StatINT:
		s.INT = uint16(v)
	case StatSTR:
		s.STR = uint16(v)
	case StatCON:
		s.CON = uint16(v)
	case StatAGI:
		s.AGI = uint16(v)
	case StatJob:
		s.Job = byte(v)
	case StatWIS:
		s.WIS = uint16(v)
		s.recomputeSP()
	case StatLevel:
		s.Level = byte(v)
	case StatEXP:
		s.EXP = v
	case StatPotential:
		s.Potential = byte(v)
	case StatATK, StatDEF, StatMAT, StatMDF, StatSPD:
		s.Combat[id-StatATK] = uint16(v)
	case StatPoints:
		s.Points = uint16(v)
	case StatHPBonus:
		s.HPBonus = int32(v)
		s.recomputeHP()
	case StatSPBonus:
		s.SPBonus = int32(v)
		s.recomputeSP()
	}
}

func (s *Stats) recomputeHP() {
	if s.Formula != nil {
		s.MaxHP = uint32(s.Formula.HP.Max(s.EffectiveLevel(), s.CON, int(s.HPBonus)+int(s.HPExtra)))
	}
}

func (s *Stats) recomputeSP() {
	if s.Formula != nil {
		s.MaxSP = uint16(s.Formula.SP.Max(s.EffectiveLevel(), s.WIS, int(s.SPBonus)+int(s.SPExtra)))
	}
}

// CombatValues are the cached words read by FUN_0035172c, not the derived
// battle getter FUN_004166e4. AC8 supplies their displayed values directly.
func (s *Stats) CombatValues() [5]uint16 { return s.Combat }
