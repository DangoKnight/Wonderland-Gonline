package assets

import "wonderland-gonline/internal/game"

const (
	nativeAlchemyRankOffset        = 45
	nativeAlchemyRestrictionOffset = 112
	nativeAlchemyFlagsOffset       = 123
	nativeAlchemyForbiddenFlag     = 1 << 2
	nativeAlchemyBasesOffset       = 136
	nativeAlchemyBaseBytes         = 2
)

// Native item info FUN_00287f58 passes the five words at offsets 136..144
// to FUN_00485d44. FUN_003cfd70 rejects restricted/non-1x1 material records.
func (it NativeItem) AlchemyItem() game.AlchemyItem {
	d := game.AlchemyItem{ID: it.Definition.ID, Rank: int(it.Record[nativeAlchemyRankOffset])}
	for i := range d.Bases {
		d.Bases[i] = le.Uint16(it.Record[nativeAlchemyBasesOffset+i*nativeAlchemyBaseBytes:])
	}
	d.Eligible = it.Definition.Type != game.ItemTypeExotic && it.Record[nativeAlchemyRestrictionOffset] == 0 && le.Uint16(it.Record[nativeAlchemyFlagsOffset:])&nativeAlchemyForbiddenFlag == 0 && it.Record[nativeItemCellWidthOffset] == 1 && it.Record[nativeItemCellHeightOffset] == 1 && (d.Bases[0] != 0 || game.AlchemyBookBonus(d.ID) > 0)
	return d
}
func (c *Catalog) CompoundingCatalog() *game.AlchemyCatalog {
	items := make(map[uint16]game.AlchemyItem, len(c.NativeItems))
	for id, it := range c.NativeItems {
		if _, ok := c.Items[id]; ok {
			items[id] = it.AlchemyItem()
		}
	}
	return game.NewAlchemyCatalog(items)
}
