package game

// Native TRE_ItemGrid uses row-major anchor slots in a five-column bag.
const BagColumns = 5
const BagRows = BagSize / BagColumns

func (d ItemDefinition) Footprint() (int, int) {
	if d.CellWidth == 0 || d.CellHeight == 0 {
		return 1, 1
	}
	return int(d.CellWidth), int(d.CellHeight)
}

func inventoryDefinitions(definitions []map[uint16]ItemDefinition) map[uint16]ItemDefinition {
	if len(definitions) != 0 {
		return definitions[0]
	}
	return nil
}

// Occupancy derives covered cells from item anchors. Covered cells have no
// duplicate Item records in memory, SQL or AC23 packets. Locked empty cells are
// unavailable. Reject malformed/overlapping layouts rather than overwrite items.
func (b Inventory) Occupancy(items map[uint16]ItemDefinition) ([BagSize]byte, error) {
	var cells [BagSize]byte
	for index, item := range b {
		if item.Empty() && !item.Locked {
			continue
		}
		width, height := 1, 1
		if !item.Empty() {
			width, height = items[item.ID].Footprint()
		}
		x, y := index%BagColumns, index/BagColumns
		if x+width > BagColumns || y+height > BagRows {
			return cells, ErrInvalidItem
		}
		for dy := range height {
			for dx := range width {
				cell := index + dy*BagColumns + dx
				if cells[cell] != 0 {
					return cells, ErrInvalidItem
				}
				cells[cell] = byte(index + 1)
			}
		}
	}
	return cells, nil
}

// CanPlace checks a complete footprint at an empty anchor. Remove an item from
// a copy first when moving it, so its former footprint can be reused.
func (b Inventory) CanPlace(slot byte, item Item, items map[uint16]ItemDefinition) bool {
	if slot < 1 || slot > BagSize || item.Empty() {
		return false
	}
	cells, err := b.Occupancy(items)
	if err != nil {
		return false
	}
	index := int(slot) - 1
	width, height := items[item.ID].Footprint()
	if index%BagColumns+width > BagColumns || index/BagColumns+height > BagRows {
		return false
	}
	for dy := range height {
		for dx := range width {
			if cells[index+dy*BagColumns+dx] != 0 {
				return false
			}
		}
	}
	return true
}
