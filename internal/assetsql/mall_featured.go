package assetsql

import "gorm.io/gorm"

// Refresh only the authored products' promotional metadata. Existing prices,
// currencies, quantities, categories and list order remain administrator-owned.
// Newly added containers retain their NEW flag, which native Hot also includes.
func migrateMallFeaturedContainers(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&MallRow{}) {
		return nil
	}
	defaults, err := defaultMall()
	if err != nil {
		return err
	}
	type identity struct {
		id           uint16
		bonus, order int
	}
	desired := map[identity]struct{ badge, hot, new, limited int }{}
	for _, e := range defaults {
		desired[identity{e.ID, e.Bonus, e.Order}] = struct{ badge, hot, new, limited int }{e.Badge, e.Hot, e.New, e.Limited}
	}
	var rows []MallRow
	if err := tx.Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		flags, ok := desired[identity{row.ValueID, row.ValueBonus, row.ValueOrder}]
		if !ok || (row.ValueBadge == flags.badge && row.ValueHot == flags.hot && row.ValueNew == flags.new && row.ValueLimited == flags.limited) {
			continue
		}
		if err := tx.Model(&MallRow{}).Where("mall_ordinal = ?", row.MallOrdinal).Updates(map[string]any{"value_badge": flags.badge, "value_hot": flags.hot, "value_new": flags.new, "value_limited": flags.limited}).Error; err != nil {
			return err
		}
	}
	return nil
}
