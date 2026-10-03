package game

import "math"

// MaxRequestedGMLevel is AC02's command cap. Equip.GetLevelProgress still caps
// non-reborn characters at MaxLevel (199), even when SetLevel requests 200.
const MaxRequestedGMLevel = 200

// SetLevel ports Equip.SetLevel. Points follow the requested level as in C#;
// the displayed level is derived from the threshold EXP. Lowering a level
// doesn't reclaim points. Saturation follows the existing Go AddExp policy.
func (c *Character) SetLevel(request byte) {
	target := min(max(int(request), 1), MaxRequestedGMLevel)
	var total uint64
	for level := 1; level < target; level++ {
		total += LevelExp(level)
	}
	gained := max(target-int(c.Level), 0)
	c.EXP = uint32(min(total, math.MaxUint32))
	c.Level = LevelForExp(uint64(c.EXP))
	c.StatPoints = uint16(min(int(c.StatPoints)+gained*StatPointsPerLevel, math.MaxUint16))
}
