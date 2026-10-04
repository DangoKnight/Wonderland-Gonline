package game

// Compound consumes one unit from each distinct slot and creates one fresh item.
// Prefer the lower ingredient slot, matching native Recv14. If it still holds
// ingredients, use another empty slot; never discard leftovers or their metadata.
// A separate result slot also preserves AC23:8's fresh one-item record semantics.
func (b *Inventory) Compound(first, second byte, result uint16, definitions ...map[uint16]ItemDefinition) (byte, error) {
	if first < 1 || first > BagSize || second < 1 || second > BagSize || first == second || result == 0 {
		return 0, ErrInvalidItem
	}
	next := *b
	if err := next.Remove(first, 1); err != nil {
		return 0, err
	}
	if err := next.Remove(second, 1); err != nil {
		return 0, err
	}
	target := min(first, second)
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
