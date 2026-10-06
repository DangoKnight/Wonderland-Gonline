package game

// Compound consumes one unit from each distinct slot and creates one fresh item.
// Prefer the lower ingredient slot, matching native Recv14. If it still holds
// ingredients, use another empty slot; never discard leftovers or their metadata.
// A separate result slot also preserves AC23:8's fresh one-item record semantics.
func (b *Inventory) Compound(first, second byte, result uint16, definitions ...map[uint16]ItemDefinition) (byte, error) {
	return b.CompoundSlots([]byte{first, second}, result, definitions...)
}

// CompoundSlots plans every debit and rectangular output placement together.
func (b *Inventory) CompoundSlots(slots []byte, result uint16, definitions ...map[uint16]ItemDefinition) (byte, error) {
	if len(slots) < AlchemyMinimumMaterials || len(slots) > AlchemyMaximumIngredients || result == 0 {
		return 0, ErrInvalidItem
	}
	next := *b
	target := byte(BagSize)
	seen := map[byte]bool{}
	for _, slot := range slots {
		if slot < 1 || slot > BagSize || seen[slot] {
			return 0, ErrInvalidItem
		}
		seen[slot] = true
		target = min(target, slot)
		if err := next.Remove(slot, 1); err != nil {
			return 0, err
		}
	}
	if !next.CanPlace(target, Item{ID: result, Count: 1}, inventoryDefinitions(definitions)) {
		target = 0
		for index, item := range next {
			if item.Empty() && next.CanPlace(byte(index+1), Item{ID: result, Count: 1}, inventoryDefinitions(definitions)) {
				target = byte(index + 1)
				break
			}
		}
	}
	if target == 0 {
		return 0, ErrInventoryFull
	}
	next[target-1] = Item{ID: result, Count: 1}
	*b = next
	return target, nil
}
