package game

import "fmt"

// Native EVE marks and appearance for Breillat's permanent character conversion.
const (
	BreillatTalkMark       = 50030
	BreillatConversionMark = 50031
	BreillatRequiredTalks  = 10
	BreillatBody           = 4
	BreillatHead           = 3
	BreillatModel          = 13
)

// ReplaceBreillatOutfit removes the original character's starter items from the
// bag and worn slots, returns other worn items intact, and equips the new outfit.
// Plan on copies so reservations or insufficient bag space leave everything intact.
func (c *Character) ReplaceBreillatOutfit(items map[uint16]ItemDefinition) error {
	standard := make(map[uint16]bool)
	for _, id := range StarterOutfit(c.Body, c.Head) {
		standard[id] = true
	}
	bag := c.Bag
	for i, item := range bag {
		if !item.Empty() && standard[item.ID] {
			if item.Locked {
				return ErrItemLocked
			}
			bag[i] = Item{}
		}
	}
	var worn Equipment
	for _, id := range StarterOutfit(BreillatBody, BreillatHead) {
		definition, ok := items[id]
		if !ok || definition.EquipSlot < 1 || int(definition.EquipSlot) > len(worn) || !worn[definition.EquipSlot-1].Empty() {
			return fmt.Errorf("Breillat equipment %d missing or invalid", id)
		}
		worn[definition.EquipSlot-1] = Item{ID: id, Count: 1}
	}
	for _, item := range c.Equipment {
		if item.Empty() {
			continue
		}
		if item.Locked {
			return ErrItemLocked
		}
		if standard[item.ID] {
			continue
		}
		definition, ok := items[item.ID]
		if !ok {
			return fmt.Errorf("worn item %d missing", item.ID)
		}
		if err := bag.Add(item, int(item.Count), definition.StackLimit(), items); err != nil {
			return err
		}
	}
	c.Bag, c.Equipment = bag, worn
	return nil
}
