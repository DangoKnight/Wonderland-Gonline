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
		total += c.LevelRequirement(level)
	}
	gained := max(target-int(c.Level), 0)
	c.EXP = uint32(min(total, math.MaxUint32))
	c.Level = c.LevelFromEXP(uint64(c.EXP))
	c.StatPoints = uint16(min(int(c.StatPoints)+gained*StatPointsPerLevel, math.MaxUint16))
}

// GMResetAttributeBaseline is GmManager.RestatPlayer's base attribute target.
const GMResetAttributeBaseline = 10

// ResetAttributes refunds only base allocations above the reset budget. The
// reference reads bonus-inclusive getters but writes base setters, refunding
// avatar bonuses again on every reset. Permanent bonuses are not refundable.
// Returns the points actually granted after saturation; call on a clone.
func (c *Character) ResetAttributes() uint16 {
	base := c.Base
	total := int(base.Strength) + int(base.Constitution) + int(base.Intelligence) + int(base.Wisdom) + int(base.Agility)
	baseline := Attributes{GMResetAttributeBaseline, GMResetAttributeBaseline, GMResetAttributeBaseline, GMResetAttributeBaseline, GMResetAttributeBaseline}
	budget := int(baseline.Strength) + int(baseline.Constitution) + int(baseline.Intelligence) + int(baseline.Wisdom) + int(baseline.Agility)
	refund := min(max(total-budget, 0), math.MaxUint16-int(c.StatPoints))
	c.Base = baseline
	c.StatPoints += uint16(refund)
	return uint16(refund)
}
