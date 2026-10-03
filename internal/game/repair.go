package game

// EquipmentRepairManager's native payment and effect references.
const (
	RepairWrenchItemID   = 48050
	RepairGoldFee        = 500
	RepairHammerEffectID = 60015
	RepairToolsPerItem   = 1
)

// RepairBagItem plans payment and restores the exact bag stack's damage. A
// healthy item is a no-op. A wrench takes priority over gold, but payment must
// leave the selected item present. Metadata and the other items stay intact.
// Call on a clone and commit the character before reporting success.
func (c *Character) RepairBagItem(slot byte) (toolSlot byte, repaired bool) {
	if slot < 1 || slot > BagSize {
		return 0, false
	}
	item := c.Bag[slot-1]
	if item.Empty() || item.Damage == 0 {
		return 0, false
	}
	for i, tool := range c.Bag {
		if tool.Empty() || tool.ID != RepairWrenchItemID {
			continue
		}
		if byte(i+1) == slot && tool.Count <= RepairToolsPerItem {
			continue
		}
		toolSlot = byte(i + 1)
		break
	}
	if toolSlot == 0 {
		if c.Gold < RepairGoldFee {
			return 0, false
		}
		c.Gold -= RepairGoldFee
	} else {
		if c.Bag.Remove(toolSlot, RepairToolsPerItem) != nil {
			return 0, false
		}
	}
	c.Bag[slot-1].Damage = 0
	return toolSlot, true
}
