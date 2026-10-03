package game

const (
	StrongScrollItemID     = 30101
	PointForgeCost         = 3
	MaxForgeProgress       = 200
	ForgeMetadataOffset    = 18
	PointForgeMaxEquipSlot = 5
	ForgeBaseStatOffset    = 100
	// Native status 250 is excluded from forging; its complete meaning is unresolved.
	ForgeExcludedStatusCode250 = 250
)

func (i *Item) SetForge(progress byte) { i.Metadata[ForgeMetadataOffset] = progress }

// CanPointForge follows MallForgingManager's equipment/stat eligibility checks.
func (d ItemDefinition) CanPointForge() bool {
	if d.EquipSlot < 1 || d.EquipSlot > PointForgeMaxEquipSlot {
		return false
	}
	for index, status := range d.Status {
		if status != 0 && status != ForgeExcludedStatusCode250 && d.Values[index] >= ForgeBaseStatOffset {
			return true
		}
	}
	return false
}
