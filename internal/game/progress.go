package game

import (
	"math"
	"wonderland-go/internal/protocol"
)

// MaxLevel is the highest level before rebirth (EquipManager.GetLevelProgress).
const MaxLevel = 199

// LevelExp is CalcMaxExp without rebirth: the EXP needed to leave a level.
func LevelExp(level int) uint64 {
	return uint64(math.RoundToEven(math.Pow(float64(level+1), 3.1) + 5))
}

// LevelForExp derives the level from total EXP, as C# does from TotalExp.
func LevelForExp(total uint64) byte {
	level := 1
	for level < MaxLevel {
		need := LevelExp(level)
		if total < need {
			break
		}
		total -= need
		level++
	}
	return byte(level)
}

// AddExp is EquipManager.AddExp: total EXP grows, levels follow it, and each level
// gained grants three stat points. Reborn characters use their own EXP curve.
// It returns the levels gained.
func (c *Character) AddExp(amount uint64) int {
	if amount == 0 {
		return 0
	}
	before := c.Level
	c.EXP = uint32(min(uint64(c.EXP)+amount, math.MaxUint32))
	c.Level = c.LevelFromEXP(uint64(c.EXP))
	gained := max(0, int(c.Level)-int(before))
	c.StatPoints = uint16(min(int(c.StatPoints)+gained*StatPointsPerLevel, math.MaxUint16))
	return gained
}

// ExpPacket is SendExp: AC8:1 stat 36 carries total EXP as 64 bits.
func (c Character) ExpPacket() []byte {
	return protocol.Builder{protocol.CommandStats, protocol.StatsStatUpdate, StatTotalEXP, protocol.StatsValueAbsolute}.U64(uint64(c.EXP))
}

// Refill sets HP and SP to the equipped maxima, as Send8_1(levelup) does.
func (c *Character) Refill(items map[uint16]ItemDefinition, growth ...ElementalGrowth) {
	full := c.Combat(items, growth...)
	c.MaxHP, c.MaxSP = uint32(max(full.MaxHP, 1)), uint32(max(full.MaxSP, 0))
	c.HP, c.SP = c.MaxHP, c.MaxSP
}

func statID(b byte) bool {
	return b == StatINT || b == StatSTR || b == StatCON || b == StatAGI || b == StatWIS
}

// StatAllocation is one requested increase.
type StatAllocation struct {
	Stat   byte
	Amount uint32
}

// amount reads the widest little-endian integer available, as AC08 does; it reports
// how many bytes it used. A missing amount means one point.
func amount(b []byte) (uint32, int) {
	switch {
	case len(b) >= 4:
		return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24, 4
	case len(b) >= 2:
		return uint32(b[0]) | uint32(b[1])<<8, 2
	case len(b) == 1:
		return uint32(b[0]), 1
	}
	return 1, 0
}

// ParseStatAllocation follows AC08.HandleStatAllocation's accepted layouts: the
// subcode itself is a stat, or subcode 1/2 carries a stat, or a target (0 = character,
// 1..4 = pet) followed by a stat or a counted list of stat/amount pairs.
func ParseStatAllocation(sub byte, data []byte) (target byte, out []StatAllocation) {
	add := func(stat byte, v uint32) { out = append(out, StatAllocation{stat, max(1, v)}) }
	if statID(sub) {
		v, _ := amount(data)
		add(sub, v)
		return 0, out
	}
	if (sub != 1 && sub != 2) || len(data) == 0 {
		return 0, nil
	}
	if statID(data[0]) {
		v, _ := amount(data[1:])
		add(data[0], v)
		return 0, out
	}
	target, data = data[0], data[1:]
	if len(data) == 0 {
		return target, nil
	}
	if statID(data[0]) {
		v, _ := amount(data[1:])
		add(data[0], v)
		return target, out
	}
	count := max(1, int(data[0]))
	data = data[1:]
	for i := 0; i < count && len(data) > 0; i++ {
		stat := data[0]
		v, n := amount(data[1:])
		data = data[1+n:]
		if statID(stat) {
			add(stat, v)
		}
	}
	return target, out
}

// Allocate spends stat points on base attributes; each request applies only when
// enough points remain. It reports whether anything changed.
func (c *Character) Allocate(requests []StatAllocation) bool {
	changed := false
	for _, r := range requests {
		if r.Amount == 0 || uint32(c.StatPoints) < r.Amount {
			continue
		}
		c.StatPoints -= uint16(r.Amount)
		n := uint16(r.Amount)
		switch r.Stat {
		case StatSTR:
			c.Base.Strength += n
		case StatCON:
			c.Base.Constitution += n
		case StatINT:
			c.Base.Intelligence += n
		case StatWIS:
			c.Base.Wisdom += n
		case StatAGI:
			c.Base.Agility += n
		}
		changed = true
	}
	return changed
}

// RecalculateVitals applies startup growth changes without healing existing characters.
func (c *Character) RecalculateVitals(items map[uint16]ItemDefinition, growth ...ElementalGrowth) bool {
	before := [4]uint32{c.MaxHP, c.MaxSP, c.HP, c.SP}
	full := c.Combat(items, growth...)
	c.MaxHP, c.MaxSP = uint32(max(full.MaxHP, 1)), uint32(max(full.MaxSP, 0))
	c.HP, c.SP = min(c.HP, c.MaxHP), min(c.SP, c.MaxSP)
	return before != [4]uint32{c.MaxHP, c.MaxSP, c.HP, c.SP}
}
